package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/rollback"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	kubeconfig    string
	container     string
	historyFile   string
	outputJSON    bool
	pricingModel  string
	allNamespaces bool
	format        string
)

const defaultHistoryFile = "/var/lib/optimizer/rollback-history.json"

func main() {
	klog.InitFlags(nil)
	flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "Path to kubeconfig")
	flag.StringVar(&container, "container", "", "Container name (default: all containers)")
	flag.StringVar(&historyFile, "history-file", defaultHistoryFile, "Path to rollback history file")
	flag.BoolVar(&outputJSON, "json", false, "Output in JSON format")
	flag.StringVar(&pricingModel, "pricing", "default", "Pricing model (aws-us-east-1, gcp-us-central1, azure-eastus, default)")
	flag.BoolVar(&allNamespaces, "all-namespaces", false, "List across all namespaces (for cost command)")
	flag.StringVar(&format, "format", "json", "Output format for reports: json, csv, html")
	flag.Parse()

	if len(flag.Args()) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Args()[0]

	// Commands that don't need kubernetes client
	switch command {
	case "history":
		if err := handleHistory(); err != nil {
			klog.Fatalf("History command failed: %v", err)
		}
		return
	case "cost":
		if len(flag.Args()) > 1 && flag.Args()[1] == "pricing" {
			showPricingModels()
			return
		}
	}

	// Commands that need kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("Failed to build config: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create kubernetes client: %v", err)
	}

	switch command {
	case "rollback":
		if len(flag.Args()) < 2 {
			printUsage()
			os.Exit(1)
		}
		if err := handleRollback(kubeClient, flag.Args()[1]); err != nil {
			klog.Fatalf("Rollback failed: %v", err)
		}
	case "cost":
		namespace := ""
		if len(flag.Args()) > 1 {
			namespace = flag.Args()[1]
		}
		if err := handleCost(kubeClient, namespace); err != nil {
			klog.Fatalf("Cost calculation failed: %v", err)
		}
	case "report":
		subcommand := ""
		if len(flag.Args()) > 1 {
			subcommand = flag.Args()[1]
		}
		namespace := ""
		if len(flag.Args()) > 2 {
			namespace = flag.Args()[2]
		}
		if err := handleReport(kubeClient, subcommand, namespace); err != nil {
			klog.Fatalf("Report generation failed: %v", err)
		}
	case "dashboard":
		if err := handleDashboard(kubeClient); err != nil {
			klog.Fatalf("Dashboard failed: %v", err)
		}
	default:
		klog.Fatalf("Unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: optctl <command> [options]\n")
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  dashboard                             Show cluster overview dashboard\n")
	fmt.Fprintf(os.Stderr, "  cost [namespace]                      Calculate resource costs and savings\n")
	fmt.Fprintf(os.Stderr, "  cost pricing                          Show available pricing models\n")
	fmt.Fprintf(os.Stderr, "  report cost [namespace]               Generate detailed cost optimization report\n")
	fmt.Fprintf(os.Stderr, "  history [resource]                    Show optimization history\n")
	fmt.Fprintf(os.Stderr, "  rollback <namespace/kind/name>        Rollback workload to previous config\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  --kubeconfig      Path to kubeconfig (default: ~/.kube/config)\n")
	fmt.Fprintf(os.Stderr, "  --container       Container name (default: all containers)\n")
	fmt.Fprintf(os.Stderr, "  --pricing         Pricing model (default: default)\n")
	fmt.Fprintf(os.Stderr, "  --all-namespaces  Calculate costs across all namespaces\n")
	fmt.Fprintf(os.Stderr, "  --format          Output format for reports: json, csv, html (default: json)\n")
	fmt.Fprintf(os.Stderr, "  --history-file    Path to history file (default: %s)\n", defaultHistoryFile)
	fmt.Fprintf(os.Stderr, "  --json            Output in JSON format\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  optctl dashboard                                # Show cluster dashboard\n")
	fmt.Fprintf(os.Stderr, "  optctl cost default                             # Cost for namespace\n")
	fmt.Fprintf(os.Stderr, "  optctl cost --all-namespaces                    # Cost for all namespaces\n")
	fmt.Fprintf(os.Stderr, "  optctl cost pricing                             # Show pricing models\n")
	fmt.Fprintf(os.Stderr, "  optctl --pricing=aws-us-east-1 cost default     # Use AWS pricing\n")
	fmt.Fprintf(os.Stderr, "  optctl report cost default                      # Generate cost report (JSON)\n")
	fmt.Fprintf(os.Stderr, "  optctl report cost --format=html default > report.html   # HTML report\n")
	fmt.Fprintf(os.Stderr, "  optctl history                                  # Show all history\n")
	fmt.Fprintf(os.Stderr, "  optctl rollback default/Deployment/nginx        # Rollback workload\n")
}

func showPricingModels() {
	fmt.Println("Available Pricing Models")
	fmt.Println(strings.Repeat("-", 70))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tPROVIDER\tREGION\tCPU/CORE/HR\tMEM/GB/HR\tTIER")

	// Sort keys for consistent output
	var keys []string
	for k := range cost.DefaultPricingModels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		model := cost.DefaultPricingModels[name]
		fmt.Fprintf(w, "%s\t%s\t%s\t$%.4f\t$%.4f\t%s\n",
			name,
			model.Provider,
			model.Region,
			model.CPUPerCoreHour,
			model.MemoryPerGBHour,
			model.Tier)
	}
	_ = w.Flush()

	fmt.Println()
	fmt.Println("Usage: optctl --pricing=<model> cost <namespace>")
}

func handleCost(kubeClient kubernetes.Interface, namespace string) error {
	ctx := context.Background()
	calculator := cost.NewCalculatorWithPreset(pricingModel)

	var namespaces []string

	if allNamespaces || namespace == "" {
		// Get all namespaces
		nsList, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list namespaces: %v", err)
		}
		for _, ns := range nsList.Items {
			// Skip system namespaces
			if !strings.HasPrefix(ns.Name, "kube-") {
				namespaces = append(namespaces, ns.Name)
			}
		}
	} else {
		namespaces = []string{namespace}
	}

	var allWorkloads []workloadCostInfo
	var totalCPUMillis, totalMemBytes int64
	var totalContainers, totalReplicas int

	for _, ns := range namespaces {
		workloads, err := getNamespaceWorkloads(ctx, kubeClient, ns)
		if err != nil {
			klog.Warningf("Failed to get workloads for namespace %s: %v", ns, err)
			continue
		}
		allWorkloads = append(allWorkloads, workloads...)

		for _, w := range workloads {
			totalCPUMillis += w.totalCPU * int64(w.replicas)
			totalMemBytes += w.totalMemory * int64(w.replicas)
			totalContainers += w.containerCount * int(w.replicas)
			totalReplicas += int(w.replicas)
		}
	}

	if len(allWorkloads) == 0 {
		fmt.Println("No workloads found.")
		return nil
	}

	// Calculate total costs
	totalCost := calculator.CalculateCost(totalCPUMillis, totalMemBytes)

	// Print header
	fmt.Printf("Resource Cost Report (Pricing: %s)\n", pricingModel)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Namespaces: %d | Workloads: %d | Containers: %d | Replicas: %d\n",
		len(namespaces), len(allWorkloads), totalContainers, totalReplicas)
	fmt.Println(strings.Repeat("-", 80))

	// Print workload details
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tWORKLOAD\tREPLICAS\tCPU\tMEMORY\tCOST/MONTH")

	// Sort by cost (highest first)
	sort.Slice(allWorkloads, func(i, j int) bool {
		costI := calculator.CalculateCost(
			allWorkloads[i].totalCPU*int64(allWorkloads[i].replicas),
			allWorkloads[i].totalMemory*int64(allWorkloads[i].replicas))
		costJ := calculator.CalculateCost(
			allWorkloads[j].totalCPU*int64(allWorkloads[j].replicas),
			allWorkloads[j].totalMemory*int64(allWorkloads[j].replicas))
		return costI.TotalPerMonth > costJ.TotalPerMonth
	})

	for _, wl := range allWorkloads {
		scaledCPU := wl.totalCPU * int64(wl.replicas)
		scaledMem := wl.totalMemory * int64(wl.replicas)
		wlCost := calculator.CalculateCost(scaledCPU, scaledMem)

		fmt.Fprintf(w, "%s\t%s/%s\t%d\t%s\t%s\t$%.2f\n",
			wl.namespace,
			wl.kind,
			wl.name,
			wl.replicas,
			formatCPU(wl.totalCPU),
			formatMemory(wl.totalMemory),
			wlCost.TotalPerMonth)
	}
	_ = w.Flush()

	// Print summary
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("COST SUMMARY")
	fmt.Println(strings.Repeat("-", 80))

	summaryW := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(summaryW, "Total CPU:\t%s\t(%d millicores)\n", formatCPU(totalCPUMillis), totalCPUMillis)
	fmt.Fprintf(summaryW, "Total Memory:\t%s\t(%d bytes)\n", formatMemory(totalMemBytes), totalMemBytes)
	fmt.Fprintln(summaryW, "\t\t")
	fmt.Fprintf(summaryW, "Hourly Cost:\t$%.4f\t\n", totalCost.TotalPerHour)
	fmt.Fprintf(summaryW, "Daily Cost:\t$%.2f\t\n", totalCost.TotalPerDay)
	fmt.Fprintf(summaryW, "Monthly Cost:\t$%.2f\t\n", totalCost.TotalPerMonth)
	fmt.Fprintf(summaryW, "Yearly Cost:\t$%.2f\t\n", totalCost.TotalPerYear)
	_ = summaryW.Flush()

	// Show optimization tip
	fmt.Println()
	fmt.Println("Tip: Run the optimizer to identify potential savings.")

	return nil
}

type workloadCostInfo struct {
	namespace      string
	kind           string
	name           string
	replicas       int32
	totalCPU       int64 // millicores per replica
	totalMemory    int64 // bytes per replica
	containerCount int
}

func getNamespaceWorkloads(ctx context.Context, kubeClient kubernetes.Interface, namespace string) ([]workloadCostInfo, error) {
	var workloads []workloadCostInfo

	// Get Deployments
	deploys, err := kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %v", err)
	}

	for _, deploy := range deploys.Items {
		replicas := int32(1)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}

		cpu, mem, count := sumContainerResources(deploy.Spec.Template.Spec.Containers)
		workloads = append(workloads, workloadCostInfo{
			namespace:      namespace,
			kind:           "Deployment",
			name:           deploy.Name,
			replicas:       replicas,
			totalCPU:       cpu,
			totalMemory:    mem,
			containerCount: count,
		})
	}

	// Get StatefulSets
	statefulsets, err := kubeClient.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %v", err)
	}

	for _, sts := range statefulsets.Items {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}

		cpu, mem, count := sumContainerResources(sts.Spec.Template.Spec.Containers)
		workloads = append(workloads, workloadCostInfo{
			namespace:      namespace,
			kind:           "StatefulSet",
			name:           sts.Name,
			replicas:       replicas,
			totalCPU:       cpu,
			totalMemory:    mem,
			containerCount: count,
		})
	}

	// Get DaemonSets
	daemonsets, err := kubeClient.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list daemonsets: %v", err)
	}

	for _, ds := range daemonsets.Items {
		// For DaemonSets, use desired number scheduled as replica count
		replicas := ds.Status.DesiredNumberScheduled
		if replicas == 0 {
			replicas = 1
		}

		cpu, mem, count := sumContainerResources(ds.Spec.Template.Spec.Containers)
		workloads = append(workloads, workloadCostInfo{
			namespace:      namespace,
			kind:           "DaemonSet",
			name:           ds.Name,
			replicas:       replicas,
			totalCPU:       cpu,
			totalMemory:    mem,
			containerCount: count,
		})
	}

	return workloads, nil
}

func sumContainerResources(containers []corev1.Container) (cpuMillis int64, memBytes int64, count int) {
	for _, c := range containers {
		if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuMillis += cpu.MilliValue()
		}
		if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			memBytes += mem.Value()
		}
		count++
	}
	return
}

func formatCPU(milliCores int64) string {
	if milliCores >= 1000 {
		cores := float64(milliCores) / 1000.0
		return fmt.Sprintf("%.2f cores", cores)
	}
	return fmt.Sprintf("%dm", milliCores)
}

func formatMemory(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	if bytes >= GB {
		return fmt.Sprintf("%.2f Gi", float64(bytes)/float64(GB))
	}
	if bytes >= MB {
		return fmt.Sprintf("%.0f Mi", float64(bytes)/float64(MB))
	}
	if bytes >= KB {
		return fmt.Sprintf("%.0f Ki", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%d B", bytes)
}

func handleHistory() error {
	manager := rollback.NewRollbackManager(nil)

	// Load history from file
	if err := manager.LoadFromFile(historyFile); err != nil {
		return fmt.Errorf("failed to load history: %v", err)
	}

	history := manager.GetAllHistory()

	if len(history) == 0 {
		fmt.Println("No optimization history found.")
		fmt.Printf("History file: %s\n", historyFile)
		return nil
	}

	// Check if specific workload requested
	if len(flag.Args()) > 1 {
		return showWorkloadHistory(manager, flag.Args()[1])
	}

	// Show all history
	return showAllHistory(history)
}

func showAllHistory(history map[string][]rollback.WorkloadConfig) error {
	// Collect all entries for sorting
	type historyEntry struct {
		key       string
		config    rollback.WorkloadConfig
		entryNum  int
		totalHist int
	}

	var entries []historyEntry
	for key, configs := range history {
		for i, config := range configs {
			entries = append(entries, historyEntry{
				key:       key,
				config:    config,
				entryNum:  i + 1,
				totalHist: len(configs),
			})
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].config.Timestamp.After(entries[j].config.Timestamp)
	})

	// Print summary
	fmt.Printf("Optimization History (%d entries across %d workloads)\n", len(entries), len(history))
	fmt.Println(strings.Repeat("-", 80))

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKLOAD\tCONTAINER\tCPU\tMEMORY\tTIMESTAMP\tAGE")

	for _, entry := range entries {
		age := formatAge(time.Since(entry.config.Timestamp))
		workload := fmt.Sprintf("%s/%s/%s",
			entry.config.Namespace,
			entry.config.Kind,
			entry.config.Name)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			workload,
			entry.config.ContainerName,
			entry.config.CPU,
			entry.config.Memory,
			entry.config.Timestamp.Format("2006-01-02 15:04"),
			age)
	}

	return w.Flush()
}

func showWorkloadHistory(manager *rollback.RollbackManager, resource string) error {
	parts := strings.Split(resource, "/")
	if len(parts) < 3 {
		return fmt.Errorf("invalid resource format, expected: namespace/kind/name[/container]")
	}

	namespace := parts[0]
	kind := parts[1]
	name := parts[2]

	containerName := container
	if len(parts) > 3 {
		containerName = parts[3]
	}
	if containerName == "" {
		containerName = "app" // default
	}

	configs := manager.GetWorkloadHistory(namespace, kind, name, containerName)

	if len(configs) == 0 {
		fmt.Printf("No history found for %s/%s/%s (container: %s)\n", namespace, kind, name, containerName)
		return nil
	}

	fmt.Printf("History for %s/%s/%s (container: %s)\n", namespace, kind, name, containerName)
	fmt.Println(strings.Repeat("-", 60))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tCPU\tMEMORY\tTIMESTAMP\tAGE")

	for i, config := range configs {
		age := formatAge(time.Since(config.Timestamp))
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			i+1,
			config.CPU,
			config.Memory,
			config.Timestamp.Format("2006-01-02 15:04:05"),
			age)
	}

	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("Total entries: %d (max: %d per workload)\n", len(configs), rollback.MaxHistoryPerWorkload)

	if len(configs) >= 2 {
		fmt.Println("\nTo rollback to previous configuration:")
		fmt.Printf("  optctl rollback %s/%s/%s --container=%s\n", namespace, kind, name, containerName)
	}

	return nil
}

func handleRollback(kubeClient kubernetes.Interface, resource string) error {
	parts := strings.Split(resource, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid resource format, expected: namespace/kind/name")
	}

	namespace := parts[0]
	kind := parts[1]
	name := parts[2]

	if container == "" {
		container = "app"
		klog.Infof("No container specified, using default: %s", container)
	}

	manager := rollback.NewRollbackManager(kubeClient)

	// Load existing history
	if err := manager.LoadFromFile(historyFile); err != nil {
		klog.Warningf("Could not load history file: %v", err)
	}

	klog.Infof("Rolling back %s/%s/%s container=%s", namespace, kind, name, container)

	ctx := context.Background()
	if err := manager.RollbackWorkload(ctx, namespace, kind, name, container); err != nil {
		return err
	}

	// Save updated history
	if err := manager.SaveToFile(historyFile); err != nil {
		klog.Warningf("Could not save history file: %v", err)
	}

	fmt.Printf("Successfully rolled back %s/%s/%s\n", namespace, kind, name)
	return nil
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

func handleDashboard(kubeClient kubernetes.Interface) error {
	ctx := context.Background()
	calculator := cost.NewCalculatorWithPreset(pricingModel)

	// Get cluster info
	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %v", err)
	}

	namespaceList, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}

	// Collect workload data
	var allWorkloads []workloadCostInfo
	var totalCPUMillis, totalMemBytes int64
	var totalContainers, totalReplicas int
	namespaceCounts := make(map[string]int)

	for _, ns := range namespaceList.Items {
		if strings.HasPrefix(ns.Name, "kube-") {
			continue
		}

		workloads, err := getNamespaceWorkloads(ctx, kubeClient, ns.Name)
		if err != nil {
			continue
		}

		namespaceCounts[ns.Name] = len(workloads)
		allWorkloads = append(allWorkloads, workloads...)

		for _, w := range workloads {
			totalCPUMillis += w.totalCPU * int64(w.replicas)
			totalMemBytes += w.totalMemory * int64(w.replicas)
			totalContainers += w.containerCount * int(w.replicas)
			totalReplicas += int(w.replicas)
		}
	}

	// Calculate total costs
	totalCost := calculator.CalculateCost(totalCPUMillis, totalMemBytes)

	// Load history
	historyManager := rollback.NewRollbackManager(nil)
	_ = historyManager.LoadFromFile(historyFile)
	history := historyManager.GetAllHistory()
	historyCount := historyManager.GetHistoryCount()

	// Print dashboard
	printDashboardHeader()
	printClusterOverview(len(nodes.Items), len(namespaceList.Items), len(allWorkloads), totalContainers, totalReplicas)
	printResourceSummary(totalCPUMillis, totalMemBytes)
	printCostSummary(totalCost, pricingModel)
	printTopWorkloads(allWorkloads, calculator, 5)
	printRecentHistory(history, 5)
	printQuickStats(historyCount, len(allWorkloads))

	return nil
}

// boxLine formats content to fit exactly within the dashboard box (75 chars content width)
func boxLine(content string) string {
	const width = 75
	if len(content) > width {
		content = content[:width]
	}
	return fmt.Sprintf("│  %-73s│", content)
}

func printDashboardHeader() {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    INTELLIGENT CLUSTER OPTIMIZER                             ║")
	fmt.Println("║                           Dashboard                                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Generated: %s\n", now)
	fmt.Println()
}

func printClusterOverview(nodes, namespaces, workloads, containers, replicas int) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  CLUSTER OVERVIEW                                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println(boxLine(fmt.Sprintf("Nodes: %-6d Namespaces: %-6d Workloads: %-6d", nodes, namespaces, workloads)))
	fmt.Println(boxLine(fmt.Sprintf("Containers: %-6d Replicas: %-6d", containers, replicas)))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func printResourceSummary(cpuMillis, memBytes int64) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  RESOURCE SUMMARY                                                           │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println(boxLine(fmt.Sprintf("Total CPU Requests:    %-10s (%d millicores)", formatCPU(cpuMillis), cpuMillis)))
	fmt.Println(boxLine(fmt.Sprintf("Total Memory Requests: %-10s", formatMemory(memBytes))))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func printCostSummary(totalCost cost.ResourceCost, pricing string) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  COST SUMMARY                                                               │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println(boxLine(fmt.Sprintf("Pricing Model: %s", pricing)))
	fmt.Println(boxLine(fmt.Sprintf("Hourly:  $%-10.4f Daily:  $%-10.2f", totalCost.TotalPerHour, totalCost.TotalPerDay)))
	fmt.Println(boxLine(fmt.Sprintf("Monthly: $%-10.2f Yearly: $%-10.2f", totalCost.TotalPerMonth, totalCost.TotalPerYear)))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func printTopWorkloads(workloads []workloadCostInfo, calculator *cost.Calculator, limit int) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  TOP WORKLOADS BY COST                                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	if len(workloads) == 0 {
		fmt.Println(boxLine("No workloads found"))
		fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
		fmt.Println()
		return
	}

	// Sort by cost
	sort.Slice(workloads, func(i, j int) bool {
		costI := calculator.CalculateCost(
			workloads[i].totalCPU*int64(workloads[i].replicas),
			workloads[i].totalMemory*int64(workloads[i].replicas))
		costJ := calculator.CalculateCost(
			workloads[j].totalCPU*int64(workloads[j].replicas),
			workloads[j].totalMemory*int64(workloads[j].replicas))
		return costI.TotalPerMonth > costJ.TotalPerMonth
	})

	fmt.Println(boxLine(fmt.Sprintf("%-17s %-20s %6s %8s %9s", "NAMESPACE", "WORKLOAD", "CPU", "MEMORY", "COST/MO")))

	count := limit
	if len(workloads) < limit {
		count = len(workloads)
	}

	for i := 0; i < count; i++ {
		wl := workloads[i]
		scaledCPU := wl.totalCPU * int64(wl.replicas)
		scaledMem := wl.totalMemory * int64(wl.replicas)
		wlCost := calculator.CalculateCost(scaledCPU, scaledMem)

		ns := wl.namespace
		if len(ns) > 17 {
			ns = ns[:14] + "..."
		}
		name := wl.name
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		fmt.Println(boxLine(fmt.Sprintf("%-17s %-20s %6s %8s $%-8.2f",
			ns,
			name,
			formatCPU(wl.totalCPU),
			formatMemory(wl.totalMemory),
			wlCost.TotalPerMonth)))
	}

	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func printRecentHistory(history map[string][]rollback.WorkloadConfig, limit int) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  RECENT OPTIMIZATION HISTORY                                                │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	if len(history) == 0 {
		fmt.Println(boxLine("No optimization history found"))
		fmt.Println(boxLine("Run: optctl history --help for more info"))
		fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
		fmt.Println()
		return
	}

	// Collect and sort entries
	type entry struct {
		workload  string
		container string
		cpu       string
		memory    string
		timestamp time.Time
	}

	var entries []entry
	for _, configs := range history {
		for _, config := range configs {
			workload := fmt.Sprintf("%s/%s", config.Namespace, config.Name)
			entries = append(entries, entry{
				workload:  workload,
				container: config.ContainerName,
				cpu:       config.CPU,
				memory:    config.Memory,
				timestamp: config.Timestamp,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp.After(entries[j].timestamp)
	})

	fmt.Println(boxLine(fmt.Sprintf("%-25s %-12s %6s %8s %8s", "WORKLOAD", "CONTAINER", "CPU", "MEMORY", "AGE")))

	count := limit
	if len(entries) < limit {
		count = len(entries)
	}

	for i := 0; i < count; i++ {
		e := entries[i]
		age := formatAge(time.Since(e.timestamp))

		workload := e.workload
		if len(workload) > 25 {
			workload = workload[:22] + "..."
		}
		container := e.container
		if len(container) > 12 {
			container = container[:9] + "..."
		}

		fmt.Println(boxLine(fmt.Sprintf("%-25s %-12s %6s %8s %8s",
			workload,
			container,
			e.cpu,
			e.memory,
			age)))
	}

	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func printQuickStats(historyCount, workloadCount int) {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  QUICK COMMANDS                                                             │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Println("│  optctl cost <namespace>     - Calculate costs for a namespace              │")
	fmt.Println("│  optctl history              - View full optimization history               │")
	fmt.Println("│  optctl rollback <resource>  - Rollback to previous configuration           │")
	fmt.Println("│  optctl cost pricing         - View available pricing models                │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}
