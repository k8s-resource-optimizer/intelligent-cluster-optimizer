package scaler

import (
	"context"
	"fmt"
	"time"

	"intelligent-cluster-optimizer/pkg/safety"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

type VerticalScaler struct {
	kubeClient    kubernetes.Interface
	pdbChecker    *safety.PDBChecker
	eventRecorder record.EventRecorder
}

type ScaleRequest struct {
	Namespace     string
	WorkloadKind  string
	WorkloadName  string
	ContainerName string
	NewCPU        string
	NewMemory     string
	Strategy      UpdateStrategy
}

type UpdateStrategy string

const (
	StrategyInPlace UpdateStrategy = "InPlace"
	StrategyRolling UpdateStrategy = "Rolling"
)

func NewVerticalScaler(kubeClient kubernetes.Interface, eventRecorder record.EventRecorder) *VerticalScaler {
	return &VerticalScaler{
		kubeClient:    kubeClient,
		pdbChecker:    safety.NewPDBChecker(kubeClient),
		eventRecorder: eventRecorder,
	}
}

func (v *VerticalScaler) DetectInPlaceSupport(ctx context.Context) (bool, error) {
	version, err := v.kubeClient.Discovery().ServerVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get server version: %v", err)
	}

	klog.V(3).Infof("Kubernetes server version: %s", version.GitVersion)
	return false, nil
}

func (v *VerticalScaler) Scale(ctx context.Context, req *ScaleRequest) error {
	klog.Infof("Starting vertical scaling for %s/%s/%s", req.Namespace, req.WorkloadKind, req.WorkloadName)

	if req.Strategy == StrategyInPlace {
		inPlaceSupported, err := v.DetectInPlaceSupport(ctx)
		if err != nil {
			klog.Warningf("Failed to detect in-place support: %v, falling back to rolling", err)
			return v.ApplyRollingUpdate(ctx, req)
		}
		if !inPlaceSupported {
			klog.V(3).Infof("In-place updates not supported, using rolling update")
			return v.ApplyRollingUpdate(ctx, req)
		}
		return v.ApplyInPlaceUpdate(ctx, req)
	}

	return v.ApplyRollingUpdate(ctx, req)
}

func (v *VerticalScaler) ApplyInPlaceUpdate(ctx context.Context, req *ScaleRequest) error {
	klog.Infof("Applying in-place update to %s/%s", req.WorkloadKind, req.WorkloadName)

	switch req.WorkloadKind {
	case "Deployment":
		return v.inPlaceUpdateDeployment(ctx, req)
	case "StatefulSet":
		return v.inPlaceUpdateStatefulSet(ctx, req)
	case "DaemonSet":
		return v.inPlaceUpdateDaemonSet(ctx, req)
	default:
		return fmt.Errorf("unsupported workload kind: %s", req.WorkloadKind)
	}
}

func (v *VerticalScaler) ApplyRollingUpdate(ctx context.Context, req *ScaleRequest) error {
	klog.Infof("Applying rolling update to %s/%s", req.WorkloadKind, req.WorkloadName)

	pdbResult, err := v.pdbChecker.CheckPDBSafety(ctx, req.Namespace, req.WorkloadKind, req.WorkloadName, 1)
	if err != nil {
		return fmt.Errorf("failed to check PDB: %v", err)
	}

	if pdbResult.HasPDB && !pdbResult.IsSafe {
		return fmt.Errorf("update would violate PDB: %s", pdbResult.Message)
	}

	switch req.WorkloadKind {
	case "Deployment":
		return v.rollingUpdateDeployment(ctx, req, pdbResult)
	case "StatefulSet":
		return v.rollingUpdateStatefulSet(ctx, req, pdbResult)
	case "DaemonSet":
		return v.rollingUpdateDaemonSet(ctx, req, pdbResult)
	default:
		return fmt.Errorf("unsupported workload kind: %s", req.WorkloadKind)
	}
}

func (v *VerticalScaler) inPlaceUpdateDeployment(ctx context.Context, req *ScaleRequest) error {
	deploy, err := v.kubeClient.AppsV1().Deployments(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	updated := false
	for i := range deploy.Spec.Template.Spec.Containers {
		if deploy.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&deploy.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("container %s not found", req.ContainerName)
	}

	_, err = v.kubeClient.AppsV1().Deployments(req.Namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %v", err)
	}

	v.recordEvent(deploy, corev1.EventTypeNormal, "VerticalScaleInPlace", "Applied in-place resource update")
	klog.Infof("Successfully applied in-place update to Deployment %s/%s", req.Namespace, req.WorkloadName)
	return nil
}

func (v *VerticalScaler) inPlaceUpdateStatefulSet(ctx context.Context, req *ScaleRequest) error {
	sts, err := v.kubeClient.AppsV1().StatefulSets(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	updated := false
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&sts.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("container %s not found", req.ContainerName)
	}

	_, err = v.kubeClient.AppsV1().StatefulSets(req.Namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update statefulset: %v", err)
	}

	v.recordEvent(sts, corev1.EventTypeNormal, "VerticalScaleInPlace", "Applied in-place resource update")
	klog.Infof("Successfully applied in-place update to StatefulSet %s/%s", req.Namespace, req.WorkloadName)
	return nil
}

func (v *VerticalScaler) inPlaceUpdateDaemonSet(ctx context.Context, req *ScaleRequest) error {
	ds, err := v.kubeClient.AppsV1().DaemonSets(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	updated := false
	for i := range ds.Spec.Template.Spec.Containers {
		if ds.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&ds.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("container %s not found", req.ContainerName)
	}

	_, err = v.kubeClient.AppsV1().DaemonSets(req.Namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update daemonset: %v", err)
	}

	v.recordEvent(ds, corev1.EventTypeNormal, "VerticalScaleInPlace", "Applied in-place resource update")
	klog.Infof("Successfully applied in-place update to DaemonSet %s/%s", req.Namespace, req.WorkloadName)
	return nil
}

func (v *VerticalScaler) rollingUpdateDeployment(ctx context.Context, req *ScaleRequest, pdbResult *safety.PDBCheckResult) error {
	deploy, err := v.kubeClient.AppsV1().Deployments(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	v.recordEvent(deploy, corev1.EventTypeNormal, "VerticalScaleStarted", "Starting rolling update for vertical scaling")

	for i := range deploy.Spec.Template.Spec.Containers {
		if deploy.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&deploy.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			break
		}
	}

	// Enforce RollingUpdate with maxUnavailable=0 so new pod becomes Ready
	// before the old one is terminated — guaranteed zero downtime.
	maxSurge := intstr.FromInt32(1)
	maxUnavailable := intstr.FromInt32(0)
	deploy.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxSurge:       &maxSurge,
			MaxUnavailable: &maxUnavailable,
		},
	}

	_, err = v.kubeClient.AppsV1().Deployments(req.Namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %v", err)
	}

	// Monitor rollout asynchronously so the approve handler can return immediately.
	// The deployment spec is already applied; Kubernetes handles the rollout.
	go func() {
		monCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := v.waitForRolloutComplete(monCtx, req.Namespace, req.WorkloadName, "Deployment"); err != nil {
			v.recordEvent(deploy, corev1.EventTypeWarning, "VerticalScaleFailed", fmt.Sprintf("Rollout failed: %v", err))
			klog.Warningf("Rollout monitoring for Deployment %s/%s failed: %v", req.Namespace, req.WorkloadName, err)
			return
		}
		v.recordEvent(deploy, corev1.EventTypeNormal, "VerticalScaleComplete", "Rolling update completed successfully")
		klog.Infof("Successfully completed rolling update for Deployment %s/%s", req.Namespace, req.WorkloadName)
	}()

	return nil
}

func (v *VerticalScaler) rollingUpdateStatefulSet(ctx context.Context, req *ScaleRequest, pdbResult *safety.PDBCheckResult) error {
	sts, err := v.kubeClient.AppsV1().StatefulSets(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	v.recordEvent(sts, corev1.EventTypeNormal, "VerticalScaleStarted", "Starting rolling update for vertical scaling")

	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&sts.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			break
		}
	}

	_, err = v.kubeClient.AppsV1().StatefulSets(req.Namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update statefulset: %v", err)
	}

	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	for i := int32(0); i < replicas; i++ {
		podName := fmt.Sprintf("%s-%d", req.WorkloadName, i)
		klog.V(3).Infof("Waiting for pod %s to be ready", podName)

		if err := v.waitForPodReady(ctx, req.Namespace, podName); err != nil {
			v.recordEvent(sts, corev1.EventTypeWarning, "VerticalScaleFailed", fmt.Sprintf("Pod %s failed to become ready: %v", podName, err))
			return err
		}

		if pdbResult.HasPDB {
			checkResult, err := v.pdbChecker.CheckPDBSafety(ctx, req.Namespace, req.WorkloadKind, req.WorkloadName, 1)
			if err != nil {
				return err
			}
			if !checkResult.IsSafe {
				klog.Warningf("PDB violation detected, pausing rollout")
				time.Sleep(30 * time.Second)
			}
		}
	}

	v.recordEvent(sts, corev1.EventTypeNormal, "VerticalScaleComplete", "Rolling update completed successfully")
	klog.Infof("Successfully completed rolling update for StatefulSet %s/%s", req.Namespace, req.WorkloadName)
	return nil
}

func (v *VerticalScaler) rollingUpdateDaemonSet(ctx context.Context, req *ScaleRequest, pdbResult *safety.PDBCheckResult) error {
	ds, err := v.kubeClient.AppsV1().DaemonSets(req.Namespace).Get(ctx, req.WorkloadName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	v.recordEvent(ds, corev1.EventTypeNormal, "VerticalScaleStarted", "Starting rolling update for vertical scaling")

	for i := range ds.Spec.Template.Spec.Containers {
		if ds.Spec.Template.Spec.Containers[i].Name == req.ContainerName {
			if err := v.updateContainerResources(&ds.Spec.Template.Spec.Containers[i], req); err != nil {
				return err
			}
			break
		}
	}

	_, err = v.kubeClient.AppsV1().DaemonSets(req.Namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update daemonset: %v", err)
	}

	go func() {
		monCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := v.waitForRolloutComplete(monCtx, req.Namespace, req.WorkloadName, "DaemonSet"); err != nil {
			v.recordEvent(ds, corev1.EventTypeWarning, "VerticalScaleFailed", fmt.Sprintf("Rollout failed: %v", err))
			klog.Warningf("Rollout monitoring for DaemonSet %s/%s failed: %v", req.Namespace, req.WorkloadName, err)
			return
		}
		v.recordEvent(ds, corev1.EventTypeNormal, "VerticalScaleComplete", "Rolling update completed successfully")
		klog.Infof("Successfully completed rolling update for DaemonSet %s/%s", req.Namespace, req.WorkloadName)
	}()

	return nil
}

func (v *VerticalScaler) updateContainerResources(container *corev1.Container, req *ScaleRequest) error {
	if container.Resources.Requests == nil {
		container.Resources.Requests = corev1.ResourceList{}
	}

	if req.NewCPU != "" {
		cpuQuantity, err := resource.ParseQuantity(req.NewCPU)
		if err != nil {
			return fmt.Errorf("invalid CPU quantity %s: %v", req.NewCPU, err)
		}
		container.Resources.Requests[corev1.ResourceCPU] = cpuQuantity
		klog.V(3).Infof("Updated CPU request to %s", req.NewCPU)
	}

	if req.NewMemory != "" {
		memQuantity, err := resource.ParseQuantity(req.NewMemory)
		if err != nil {
			return fmt.Errorf("invalid memory quantity %s: %v", req.NewMemory, err)
		}
		container.Resources.Requests[corev1.ResourceMemory] = memQuantity
		klog.V(3).Infof("Updated memory request to %s", req.NewMemory)
	}

	return nil
}

func (v *VerticalScaler) waitForRolloutComplete(ctx context.Context, namespace, name, kind string) error {
	timeout := 5 * time.Minute
	interval := 5 * time.Second

	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		switch kind {
		case "Deployment":
			deploy, err := v.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return v.isDeploymentReady(deploy), nil

		case "DaemonSet":
			ds, err := v.kubeClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return v.isDaemonSetReady(ds), nil

		default:
			return false, fmt.Errorf("unsupported kind for rollout: %s", kind)
		}
	})
}

func (v *VerticalScaler) waitForPodReady(ctx context.Context, namespace, podName string) error {
	timeout := 3 * time.Minute
	interval := 5 * time.Second

	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := v.kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func (v *VerticalScaler) isDeploymentReady(deploy *appsv1.Deployment) bool {
	return deploy.Status.UpdatedReplicas == *deploy.Spec.Replicas &&
		deploy.Status.Replicas == *deploy.Spec.Replicas &&
		deploy.Status.AvailableReplicas == *deploy.Spec.Replicas &&
		deploy.Status.ObservedGeneration >= deploy.Generation
}

func (v *VerticalScaler) isDaemonSetReady(ds *appsv1.DaemonSet) bool {
	return ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled &&
		ds.Status.NumberAvailable == ds.Status.DesiredNumberScheduled &&
		ds.Status.ObservedGeneration >= ds.Generation
}

func (v *VerticalScaler) recordEvent(obj interface{}, eventType, reason, message string) {
	if v.eventRecorder == nil {
		return
	}

	switch o := obj.(type) {
	case *appsv1.Deployment:
		v.eventRecorder.Event(o, eventType, reason, message)
	case *appsv1.StatefulSet:
		v.eventRecorder.Event(o, eventType, reason, message)
	case *appsv1.DaemonSet:
		v.eventRecorder.Event(o, eventType, reason, message)
	}
}
