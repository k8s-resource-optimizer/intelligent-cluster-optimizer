package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	// Import local packages
	"intelligent-cluster-optimizer/pkg/metrics"
	"intelligent-cluster-optimizer/pkg/storage"
)

func main() {
	// 1. Parse command line flags
	namespace := flag.String("namespace", "", "Target namespace to collect metrics from")
	pollIntervalStr := flag.String("interval", "", "Poll interval (e.g., 10s, 1m)")
	flag.Parse()

	// 2. Fall back to environment variables if flags not provided
	if *namespace == "" {
		*namespace = os.Getenv("TARGET_NAMESPACE")
		if *namespace == "" {
			*namespace = "default"
		}
	}

	if *pollIntervalStr == "" {
		*pollIntervalStr = os.Getenv("POLL_INTERVAL")
		if *pollIntervalStr == "" {
			*pollIntervalStr = "30s"
		}
	}

	pollInterval, err := time.ParseDuration(*pollIntervalStr)
	if err != nil {
		log.Fatalf("Invalid interval format '%s': %v. Use format like '30s', '1m', '2m30s'", *pollIntervalStr, err)
	}

	// 3. Setup Kubeconfig
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Configuration:\n")
	fmt.Printf("  - Target Namespace: %s\n", *namespace)
	fmt.Printf("  - Poll Interval: %s\n", pollInterval)
	fmt.Println()

	// 3. Initialize Components
	collector, err := metrics.NewCollector(config)
	if err != nil {
		log.Fatal(err)
	}

	// initialize storage
	store := storage.NewStorage()
	dataFile := fmt.Sprintf("metrics_data_%s.json", *namespace)

	// load existing data on startup
	if err := store.LoadFromFile(dataFile); err != nil {
		log.Printf("Warning: Could not load old data: %v", err)
	} else {
		fmt.Println("Loaded historical data from disk.")
	}

	// save data automatically when the program exits
	defer func() {
		fmt.Println("Saving data to disk...")
		if err := store.SaveToFile(dataFile); err != nil {
			log.Printf("Error: Saving data: %v", err)
		} else {
			fmt.Println("Data saved successfully.")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.StartGarbageCollector(ctx, 1*time.Hour, 24*time.Hour)

	// 4. Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 5. Start the Loop (Ticker)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	startTime := time.Now()
	sampleCount := 0
	podStats := make(map[string]*podStatistics)

	fmt.Println("Starting Metric Collector... (Press Ctrl+C to stop)")

	for {
		select {
		case <-sigChan:
			fmt.Println()
			printSummary(startTime, sampleCount, podStats)
			return
		case <-ticker.C:
			// A. Fetch
			data, err := collector.GetPodMetrics(*namespace)
			if err != nil {
				log.Printf("Error fetching metrics: %v", err)
				continue
			}

			// B. Store & Print
			fmt.Printf("[%s] Collecting metrics for %d pods...\n", time.Now().Format("15:04:05"), len(data))

			activePodNames := make([]string, 0, len(data))

			for _, pod := range data {
				// store the whole pod structure (includes containers)
				store.Add(pod)
				activePodNames = append(activePodNames, pod.PodName)
				sampleCount++

				// Track statistics per pod
				if _, exists := podStats[pod.PodName]; !exists {
					podStats[pod.PodName] = &podStatistics{}
				}
				for _, container := range pod.Containers {
					podStats[pod.PodName].update(container.UsageCPU, container.UsageMemory)
				}

				fmt.Printf("Pod: %s\n", pod.PodName)

				// loop through containers, where data is placed
				for _, container := range pod.Containers {
					fmt.Printf("   └─ %s\n", container.ContainerName)

					// print usage-request-limit
					fmt.Printf("      Usage:   CPU: %4dm | Mem: %4dMi\n", container.UsageCPU, container.UsageMemory)
					fmt.Printf("      Request: CPU: %4dm | Mem: %4dMi\n", container.RequestCPU, container.RequestMemory)
					fmt.Printf("      Limit:   CPU: %4dm | Mem: %4dMi\n", container.LimitCPU, container.LimitMemory)
				}
			}

			deadPodsRemoved := store.SyncPods(activePodNames)
			if deadPodsRemoved > 0 {
				fmt.Printf("[SYNC] Removed %d dead pod(s) from storage\n", deadPodsRemoved)
			}

			fmt.Println("---------------------------------------------------")
		}
	}
}

type podStatistics struct {
	cpuSum int64
	memSum int64
	cpuMin int64
	cpuMax int64
	memMin int64
	memMax int64
	count  int
}

func (s *podStatistics) update(cpu, mem int64) {
	if s.count == 0 {
		s.cpuMin, s.cpuMax = cpu, cpu
		s.memMin, s.memMax = mem, mem
	} else {
		if cpu < s.cpuMin {
			s.cpuMin = cpu
		}
		if cpu > s.cpuMax {
			s.cpuMax = cpu
		}
		if mem < s.memMin {
			s.memMin = mem
		}
		if mem > s.memMax {
			s.memMax = mem
		}
	}
	s.cpuSum += cpu
	s.memSum += mem
	s.count++
}

func printSummary(startTime time.Time, sampleCount int, podStats map[string]*podStatistics) {
	duration := time.Since(startTime)

	fmt.Println("===================================================")
	fmt.Println("           COLLECTION SUMMARY")
	fmt.Println("===================================================")
	fmt.Printf("  Duration:        %s\n", duration.Round(time.Second))
	fmt.Printf("  Pods monitored:  %d\n", len(podStats))
	fmt.Printf("  Total samples:   %d\n", sampleCount)
	fmt.Println("---------------------------------------------------")

	for podName, stats := range podStats {
		if stats.count == 0 {
			continue
		}
		avgCPU := stats.cpuSum / int64(stats.count)
		avgMem := stats.memSum / int64(stats.count)

		fmt.Printf("  Pod: %s\n", podName)
		fmt.Printf("    Samples: %d\n", stats.count)
		fmt.Printf("    CPU  - Avg: %4dm | Min: %4dm | Max: %4dm\n", avgCPU, stats.cpuMin, stats.cpuMax)
		fmt.Printf("    Mem  - Avg: %4dMi | Min: %4dMi | Max: %4dMi\n", avgMem, stats.memMin, stats.memMax)
		fmt.Println()
	}
	fmt.Println("===================================================")
}
