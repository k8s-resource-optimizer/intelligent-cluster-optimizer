package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
)

type InMemoryStorage struct {
	mu      sync.RWMutex                  // Protects the map from concurrent writes
	history map[string][]models.PodMetric // Key: PodName
}

func NewStorage() *InMemoryStorage {
	return &InMemoryStorage{
		history: make(map[string][]models.PodMetric),
	}
}

// Add stores a pod metric in memory
func (s *InMemoryStorage) Add(metric models.PodMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := metric.PodName
	s.history[key] = append(s.history[key], metric)
}

// Cleanup removes metrics older than maxAge and returns the count of removed entries
func (s *InMemoryStorage) Cleanup(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffTime := time.Now().Add(-maxAge)
	removedCount := 0

	for podName, metrics := range s.history {
		var validMetrics []models.PodMetric

		for _, metric := range metrics {
			if metric.Timestamp.After(cutoffTime) {
				validMetrics = append(validMetrics, metric)
			} else {
				removedCount++
			}
		}

		if len(validMetrics) == 0 {
			delete(s.history, podName)
		} else {
			s.history[podName] = validMetrics
		}
	}

	return removedCount
}

// SyncPods removes metrics for pods not in the activePodNames list
func (s *InMemoryStorage) SyncPods(activePodNames []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeSet := make(map[string]bool, len(activePodNames))
	for _, name := range activePodNames {
		activeSet[name] = true
	}

	removedCount := 0

	for podName := range s.history {
		if !activeSet[podName] {
			delete(s.history, podName)
			removedCount++
		}
	}

	return removedCount
}

// SaveToFile writes the current history map to a JSON file
func (s *InMemoryStorage) SaveToFile(filename string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.history, "", " ") // map to json
	if err != nil {
		return err
	}

	// #nosec G306 - config files need to be readable by the process owner
	return os.WriteFile(filename, data, 0600) // write to disk with restrictive permissions
}

// LoadFromFile reads a JSON file and restores the history map
func (s *InMemoryStorage) LoadFromFile(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Sanitize the file path to prevent path traversal
	cleanPath := filepath.Clean(filename)
	// #nosec G304 - filename is provided by the operator, not user input
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file not exists
		}
		return err
	}
	return json.Unmarshal(data, &s.history) // json to map
}

// GetMetricsByNamespace returns all metrics for pods in a specific namespace
// that are newer than the specified duration
func (s *InMemoryStorage) GetMetricsByNamespace(namespace string, since time.Duration) []models.PodMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoffTime := time.Now().Add(-since)
	var result []models.PodMetric

	for _, metrics := range s.history {
		for _, metric := range metrics {
			if metric.Namespace == namespace && metric.Timestamp.After(cutoffTime) {
				result = append(result, metric)
			}
		}
	}

	return result
}

// GetMetricsByWorkload returns metrics for pods matching a workload name prefix
// in the specified namespace, newer than the specified duration
func (s *InMemoryStorage) GetMetricsByWorkload(namespace, workloadName string, since time.Duration) []models.PodMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoffTime := time.Now().Add(-since)
	var result []models.PodMetric

	for podName, metrics := range s.history {
		// Check if pod belongs to this workload (pod name starts with workload name)
		if !hasPrefix(podName, workloadName) {
			continue
		}

		for _, metric := range metrics {
			if metric.Namespace == namespace && metric.Timestamp.After(cutoffTime) {
				result = append(result, metric)
			}
		}
	}

	return result
}

// GetAllMetrics returns all stored metrics (for debugging/inspection)
func (s *InMemoryStorage) GetAllMetrics() map[string][]models.PodMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent mutation
	result := make(map[string][]models.PodMetric, len(s.history))
	for k, v := range s.history {
		metricsCopy := make([]models.PodMetric, len(v))
		copy(metricsCopy, v)
		result[k] = metricsCopy
	}

	return result
}

// GetMetricCount returns the total number of metric entries stored
func (s *InMemoryStorage) GetMetricCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, metrics := range s.history {
		count += len(metrics)
	}
	return count
}

// hasPrefix checks if s starts with prefix (simple implementation to avoid strings import)
func hasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// startGarbageCollector periodically cleans up old metrics from storage
func (s *InMemoryStorage) StartGarbageCollector(interval time.Duration, maxAge time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			removed := s.Cleanup(maxAge)
			if removed > 0 {
				println("[GC] Cleaned up", removed, "old metric entries")
			}
		}
	}()
}

// Snapshot returns a deep copy of the current storage state for backup purposes
func (s *InMemoryStorage) Snapshot() map[string][]models.PodMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a deep copy to avoid race conditions
	snapshot := make(map[string][]models.PodMetric, len(s.history))
	for podName, metrics := range s.history {
		metricsCopy := make([]models.PodMetric, len(metrics))
		copy(metricsCopy, metrics)
		snapshot[podName] = metricsCopy
	}

	return snapshot
}

// Restore replaces the current storage state with the provided data
// This is used to restore from a backup
func (s *InMemoryStorage) Restore(data map[string][]models.PodMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Replace the entire history with the backup data
	s.history = make(map[string][]models.PodMetric, len(data))
	for podName, metrics := range data {
		metricsCopy := make([]models.PodMetric, len(metrics))
		copy(metricsCopy, metrics)
		s.history[podName] = metricsCopy
	}
}
