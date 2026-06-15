package applier

import (
	"context"
	"fmt"

	"intelligent-cluster-optimizer/pkg/rollback"
	"intelligent-cluster-optimizer/pkg/scaler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

type Applier struct {
	kubeClient      kubernetes.Interface
	verticalScaler  *scaler.VerticalScaler
	rollbackManager *rollback.RollbackManager
}

func NewApplier(kubeClient kubernetes.Interface, eventRecorder record.EventRecorder) *Applier {
	return &Applier{
		kubeClient:      kubeClient,
		verticalScaler:  scaler.NewVerticalScaler(kubeClient, eventRecorder),
		rollbackManager: rollback.NewRollbackManager(kubeClient),
	}
}

func (a *Applier) Apply(ctx context.Context, recommendation *ResourceRecommendation, dryRun bool) (*ApplyResult, error) {
	if dryRun {
		return a.DryRunApply(ctx, recommendation)
	}
	return a.LiveApply(ctx, recommendation)
}

func (a *Applier) DryRunApply(ctx context.Context, recommendation *ResourceRecommendation) (*ApplyResult, error) {
	result := &ApplyResult{
		Applied:      false,
		DryRun:       true,
		WorkloadKind: recommendation.WorkloadKind,
		WorkloadName: recommendation.WorkloadName,
		Namespace:    recommendation.Namespace,
		Changes:      []string{},
	}

	if recommendation.CurrentCPU != recommendation.RecommendedCPU {
		change := fmt.Sprintf("CPU: %s -> %s", recommendation.CurrentCPU, recommendation.RecommendedCPU)
		result.Changes = append(result.Changes, change)
		klog.Infof("[DRY-RUN] Would change %s/%s/%s container=%s: %s",
			recommendation.Namespace, recommendation.WorkloadKind, recommendation.WorkloadName,
			recommendation.ContainerName, change)
	}

	if recommendation.CurrentMemory != recommendation.RecommendedMemory {
		change := fmt.Sprintf("Memory: %s -> %s", recommendation.CurrentMemory, recommendation.RecommendedMemory)
		result.Changes = append(result.Changes, change)
		klog.Infof("[DRY-RUN] Would change %s/%s/%s container=%s: %s",
			recommendation.Namespace, recommendation.WorkloadKind, recommendation.WorkloadName,
			recommendation.ContainerName, change)
	}

	klog.V(2).Infof("[DRY-RUN] Total changes for %s/%s: %d",
		recommendation.WorkloadKind, recommendation.WorkloadName, len(result.Changes))

	return result, nil
}

func (a *Applier) LiveApply(ctx context.Context, recommendation *ResourceRecommendation) (*ApplyResult, error) {
	result := &ApplyResult{
		Applied:      false,
		DryRun:       false,
		WorkloadKind: recommendation.WorkloadKind,
		WorkloadName: recommendation.WorkloadName,
		Namespace:    recommendation.Namespace,
		Changes:      []string{},
	}

	if recommendation.CurrentCPU == recommendation.RecommendedCPU &&
		recommendation.CurrentMemory == recommendation.RecommendedMemory {
		klog.V(3).Infof("[LIVE] No changes needed for %s/%s/%s",
			recommendation.Namespace, recommendation.WorkloadKind, recommendation.WorkloadName)
		return result, nil
	}

	klog.Infof("[LIVE] Applying changes to %s/%s/%s",
		recommendation.Namespace, recommendation.WorkloadKind, recommendation.WorkloadName)

	if err := a.rollbackManager.SavePreviousConfig(ctx, recommendation.Namespace, recommendation.WorkloadKind,
		recommendation.WorkloadName, recommendation.ContainerName); err != nil {
		klog.Warningf("Failed to save rollback config: %v", err)
	}

	scaleReq := &scaler.ScaleRequest{
		Namespace:     recommendation.Namespace,
		WorkloadKind:  recommendation.WorkloadKind,
		WorkloadName:  recommendation.WorkloadName,
		ContainerName: recommendation.ContainerName,
		NewCPU:        recommendation.RecommendedCPU,
		NewMemory:     recommendation.RecommendedMemory,
		Strategy:      scaler.StrategyRolling,
	}

	if err := a.verticalScaler.Scale(ctx, scaleReq); err != nil {
		result.Error = err
		return result, err
	}

	if recommendation.CurrentCPU != recommendation.RecommendedCPU {
		change := fmt.Sprintf("CPU: %s -> %s", recommendation.CurrentCPU, recommendation.RecommendedCPU)
		result.Changes = append(result.Changes, change)
	}
	if recommendation.CurrentMemory != recommendation.RecommendedMemory {
		change := fmt.Sprintf("Memory: %s -> %s", recommendation.CurrentMemory, recommendation.RecommendedMemory)
		result.Changes = append(result.Changes, change)
	}

	result.Applied = true
	klog.Infof("[LIVE] Successfully applied %d changes to %s/%s", len(result.Changes), recommendation.WorkloadKind, recommendation.WorkloadName)
	return result, nil
}

func (a *Applier) GetCurrentResources(ctx context.Context, namespace, kind, name, containerName string) (*ResourceRecommendation, error) {
	rec := &ResourceRecommendation{
		Namespace:     namespace,
		WorkloadKind:  kind,
		WorkloadName:  name,
		ContainerName: containerName,
	}

	var containers []corev1.Container
	var err error

	switch kind {
	case "Deployment":
		var deploy *appsv1.Deployment
		deploy, err = a.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			containers = deploy.Spec.Template.Spec.Containers
		}
	case "StatefulSet":
		var sts *appsv1.StatefulSet
		sts, err = a.kubeClient.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			containers = sts.Spec.Template.Spec.Containers
		}
	case "DaemonSet":
		var ds *appsv1.DaemonSet
		ds, err = a.kubeClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			containers = ds.Spec.Template.Spec.Containers
		}
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}

	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		if container.Name == containerName {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				rec.CurrentCPU = cpu.String()
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				rec.CurrentMemory = mem.String()
			}
			return rec, nil
		}
	}

	return rec, nil
}

func ParseResourceQuantity(value string) (resource.Quantity, error) {
	return resource.ParseQuantity(value)
}
