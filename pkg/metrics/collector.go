package metrics

import (
	"context"
	"intelligent-cluster-optimizer/pkg/models"
	"time"

	corev1 "k8s.io/api/core/v1" // for pod/container structure
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type MetricsCollector struct {
	MetricsClient *metricsv.Clientset
	Clientset     *kubernetes.Clientset // standard client to access pod specs
}

func NewCollector(config *rest.Config) (*MetricsCollector, error) {

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// Create standard kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &MetricsCollector{
		MetricsClient: metricsClient,
		Clientset:     clientset, // <--- Assign it here
	}, nil
}

func (c *MetricsCollector) GetPodMetrics(namespace string) ([]models.PodMetric, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch live usage numbers (metrics api)
	metricsList, err := c.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Fetch pod configurations (core api) to get requests and limits
	podList, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Create a loopup map for pod specs
	podMap := make(map[string]corev1.Pod)
	for _, p := range podList.Items {
		podMap[p.Name] = p
	}

	var results []models.PodMetric

	// Iterate through metrics and combine with spec data
	for _, m := range metricsList.Items {

		// Find the matching Pod Spec
		podSpec, exists := podMap[m.Name]
		if !exists {
			continue // Pod might have been deleted while we were fetching
		}

		var containerMetrics []models.ContainerMetric

		// Match individual containers
		for _, containerUsage := range m.Containers {

			// Initialize default values (0)
			var reqCPU, reqMem, limCPU, limMem int64

			// Find the specific container config inside the Pod Spec
			for _, containerSpec := range podSpec.Spec.Containers {
				if containerSpec.Name == containerUsage.Name {
					// Extract Requests (if they exist)
					reqCPU = containerSpec.Resources.Requests.Cpu().MilliValue()
					reqMem = containerSpec.Resources.Requests.Memory().Value() / (1024 * 1024)

					// Extract Limits (if they exist)
					limCPU = containerSpec.Resources.Limits.Cpu().MilliValue()
					limMem = containerSpec.Resources.Limits.Memory().Value() / (1024 * 1024)
					break
				}
			}

			// Add to list
			containerMetrics = append(containerMetrics, models.ContainerMetric{
				ContainerName: containerUsage.Name,

				// Usage (Real-time)
				UsageCPU:    containerUsage.Usage.Cpu().MilliValue(),
				UsageMemory: containerUsage.Usage.Memory().Value() / (1024 * 1024),

				// Spec (Configured)
				RequestCPU:    reqCPU,
				RequestMemory: reqMem,
				LimitCPU:      limCPU,
				LimitMemory:   limMem,
			})
		}

		results = append(results, models.PodMetric{
			PodName:    m.Name,
			Namespace:  m.Namespace,
			Timestamp:  time.Now(),
			Containers: containerMetrics,
		})
	}
	return results, nil
}
