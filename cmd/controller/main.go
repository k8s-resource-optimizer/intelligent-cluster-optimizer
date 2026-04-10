package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/controller"
	"intelligent-cluster-optimizer/pkg/forecaster"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

var (
	kubeconfig    string
	namespace     string
	workers       int
	leaseLockName string
	leaseLockNS   string
	leaderElect   bool
	leaseDuration time.Duration
	renewDeadline time.Duration
	retryPeriod   time.Duration
	mlServiceURL  string
	apiAddr       string
)

func main() {
	klog.InitFlags(nil)
	flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "Path to kubeconfig")
	flag.StringVar(&namespace, "namespace", "default", "Namespace to watch OptimizerConfigs")
	flag.IntVar(&workers, "workers", 2, "Number of worker threads")
	flag.StringVar(&leaseLockName, "lease-lock-name", "optimizer-controller", "Name of lease lock")
	flag.StringVar(&leaseLockNS, "lease-lock-namespace", "default", "Namespace for lease lock")
	flag.BoolVar(&leaderElect, "leader-elect", true, "Enable leader election")
	flag.DurationVar(&leaseDuration, "lease-duration", 15*time.Second, "Lease duration")
	flag.DurationVar(&renewDeadline, "renew-deadline", 10*time.Second, "Renew deadline")
	flag.DurationVar(&retryPeriod, "retry-period", 2*time.Second, "Retry period")
	flag.StringVar(&mlServiceURL, "ml-service-url", os.Getenv("ML_SERVICE_URL"),
		"Base URL of the Chronos-2 ML forecasting service (e.g. http://ml-service:8080). "+
			"Falls back to Holt-Winters when empty or unreachable.")
	flag.StringVar(&apiAddr, "api-addr", ":8090",
		"Address for the GUI REST API server (e.g. :8090). Set to empty string to disable.")
	flag.Parse()

	if err := v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("Failed to register OptimizerConfig scheme: %v", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("Failed to build config: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create kubernetes client: %v", err)
	}

	optimizerClient, err := v1alpha1.NewOptimizerConfigClient(config, namespace)
	if err != nil {
		klog.Fatalf("Failed to create optimizer client: %v", err)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})
	defer eventBroadcaster.Shutdown()

	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{
		Component: "optimizer-controller",
	})

	reconciler := controller.NewReconciler(kubeClient, eventRecorder)

	// Wire up ML forecaster with Holt-Winters fallback.
	// The FallbackForecaster tries the Chronos-2 service first; on any error it
	// transparently falls back to the local Holt-Winters predictor so the controller
	// keeps scaling even when the ML service is not yet deployed.
	zapLogger, err := zap.NewProduction()
	if err != nil {
		klog.Fatalf("Failed to create zap logger: %v", err)
	}
	defer zapLogger.Sync() //nolint:errcheck
	hwForecaster := forecaster.NewHoltWintersForecaster(zapLogger)
	var cpuForecaster forecaster.CpuForecaster = hwForecaster
	if mlServiceURL != "" {
		mlClient := forecaster.NewForecastClient(mlServiceURL, 10*time.Second, zapLogger)
		cpuForecaster = forecaster.NewFallbackForecaster(mlClient, hwForecaster, zapLogger)
		klog.Infof("ML forecaster enabled: %s (Holt-Winters fallback active)", mlServiceURL)
	} else {
		klog.Info("ML_SERVICE_URL not set — using Holt-Winters forecaster only")
	}
	mlScaler := forecaster.NewHorizontalScaler(kubeClient, zapLogger)
	reconciler.SetMLForecaster(cpuForecaster, mlScaler)

	// ── GUI API server ────────────────────────────────────────────────────────
	scalingHistory := apiserver.NewScalingHistoryStore(500)
	forecastCache := apiserver.NewForecastCache()
	dryRunQueue := apiserver.NewDryRunQueue()
	reconciler.SetAPIStores(scalingHistory, forecastCache, dryRunQueue)
	// ─────────────────────────────────────────────────────────────────────────

	ctrl := controller.NewOptimizerController(kubeClient, optimizerClient, reconciler, eventRecorder, namespace)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		klog.Infof("Received signal %v, shutting down gracefully", sig)
		cancel()
	}()

	// Start the GUI API server now that ctx is available.
	if apiAddr != "" {
		apiSrv := apiserver.NewServer(apiserver.Config{
			Addr:            apiAddr,
			Namespace:       namespace,
			KubeClient:      kubeClient,
			OptimizerClient: optimizerClient,
			MetricsStorage:  reconciler.GetMetricsStorage(),
			ScalingHistory:  scalingHistory,
			ForecastCache:   forecastCache,
			DryRunQueue:     dryRunQueue,
			MLScaler:        mlScaler,
		})
		go apiSrv.Start(ctx)
		klog.Infof("GUI API server started on %s", apiAddr)
	}

	if !leaderElect {
		klog.Info("Running without leader election")
		if err := ctrl.Run(ctx, workers); err != nil {
			klog.Fatalf("Error running controller: %v", err)
		}
		return
	}

	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Failed to get hostname: %v", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseLockName,
			Namespace: leaseLockNS,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.Infof("Started leading as %s", id)
				if err := ctrl.Run(ctx, workers); err != nil {
					klog.Fatalf("Error running controller: %v", err)
				}
			},
			OnStoppedLeading: func() {
				klog.Infof("Leader lost: %s", id)
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				klog.Infof("New leader elected: %s", identity)
			},
		},
	})
}
