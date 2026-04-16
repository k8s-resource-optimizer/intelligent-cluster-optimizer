package applier

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type ResourceRecommendation struct {
	Namespace         string
	WorkloadKind      string
	WorkloadName      string
	ContainerName     string
	CurrentCPU        string
	RecommendedCPU    string
	CurrentMemory     string
	RecommendedMemory string
}

type ApplyResult struct {
	Applied      bool
	DryRun       bool
	WorkloadKind string
	WorkloadName string
	Namespace    string
	Changes      []string
	Error        error
}

// minChangePercent is the minimum relative change required before a
// recommendation is considered meaningful. Changes smaller than this
// threshold (e.g. 107m→108m) are ignored to prevent thrashing.
const minChangePercent = 0.05

func (r *ResourceRecommendation) HasChanges() bool {
	return cpuChanged(r.CurrentCPU, r.RecommendedCPU) ||
		memChanged(r.CurrentMemory, r.RecommendedMemory)
}

func cpuChanged(current, recommended string) bool {
	if current == recommended {
		return false
	}
	cur := parseCPUMillicores(current)
	rec := parseCPUMillicores(recommended)
	if cur == 0 {
		return rec != 0
	}
	diff := float64(rec-cur) / float64(cur)
	if diff < 0 {
		diff = -diff
	}
	return diff >= minChangePercent
}

func memChanged(current, recommended string) bool {
	if current == recommended {
		return false
	}
	cur := parseMemBytes(current)
	rec := parseMemBytes(recommended)
	if cur == 0 {
		return rec != 0
	}
	diff := float64(rec-cur) / float64(cur)
	if diff < 0 {
		diff = -diff
	}
	return diff >= minChangePercent
}

// parseCPUMillicores parses "100m" → 100, "1" → 1000, "0.5" → 500
func parseCPUMillicores(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		var v int64
		fmt.Sscanf(s, "%dm", &v)
		return v
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return int64(v * 1000)
}

// parseMemBytes parses "128Mi" → bytes, "1Gi" → bytes, "512" → 512
func parseMemBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"Gi", 1024 * 1024 * 1024},
		{"Mi", 1024 * 1024},
		{"Ki", 1024},
		{"G", 1000 * 1000 * 1000},
		{"M", 1000 * 1000},
		{"K", 1000},
	}
	for _, entry := range suffixes {
		if strings.HasSuffix(s, entry.suffix) {
			var v int64
			fmt.Sscanf(s, "%d"+entry.suffix, &v)
			return v * entry.mult
		}
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func (r *ResourceRecommendation) GetResourceRequirements() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
}
