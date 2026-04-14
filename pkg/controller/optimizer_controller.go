package controller

import (
	"context"
	"fmt"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type OptimizerController struct {
	kubeClient      kubernetes.Interface
	optimizerClient *optimizerv1alpha1.OptimizerConfigClient
	informer        cache.SharedIndexInformer
	workqueue       workqueue.TypedRateLimitingInterface[string]
	eventRecorder   record.EventRecorder
	reconciler      *Reconciler
}

func NewOptimizerController(
	kubeClient kubernetes.Interface,
	optimizerClient *optimizerv1alpha1.OptimizerConfigClient,
	reconciler *Reconciler,
	eventRecorder record.EventRecorder,
	namespace string,
) *OptimizerController {

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return optimizerClient.List(context.Background(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return optimizerClient.Watch(context.Background(), options)
			},
		},
		&optimizerv1alpha1.OptimizerConfig{},
		time.Minute*10,
		cache.Indexers{},
	)

	controller := &OptimizerController{
		kubeClient:      kubeClient,
		optimizerClient: optimizerClient,
		informer:        informer,
		workqueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		eventRecorder: eventRecorder,
		reconciler:    reconciler,
	}

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				controller.workqueue.Add(key)
				klog.V(4).Infof("Added OptimizerConfig to queue: %s", key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldConfig, ok1 := oldObj.(*optimizerv1alpha1.OptimizerConfig)
			newConfig, ok2 := newObj.(*optimizerv1alpha1.OptimizerConfig)
			if ok1 && ok2 && oldConfig.Generation == newConfig.Generation {
				// Status-only update — the periodic RequeueAfter timer handles the next run
				return
			}
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				controller.workqueue.Add(key)
				klog.V(4).Infof("Updated OptimizerConfig queued: %s", key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				controller.workqueue.Add(key)
				klog.V(4).Infof("Deleted OptimizerConfig queued: %s", key)
			}
		},
	})
	if err != nil {
		klog.Fatalf("Error adding event handler: %v", err)
	}

	return controller
}

func (c *OptimizerController) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting OptimizerConfig controller")
	klog.Infof("Starting %d workers", workers)

	go c.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Cache synced, starting workers")

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	klog.Info("Workers started")
	<-ctx.Done()
	klog.Info("Shutting down workers")

	return nil
}

func (c *OptimizerController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *OptimizerController) processNextItem(ctx context.Context) bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	defer c.workqueue.Done(key)

	err := c.syncHandler(ctx, key)
	c.handleErr(err, key)
	return true
}

func (c *OptimizerController) syncHandler(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("error fetching object with key %s: %v", key, err)
	}

	if !exists {
		klog.V(3).Infof("OptimizerConfig %s/%s deleted", namespace, name)
		return nil
	}

	original, ok := obj.(*optimizerv1alpha1.OptimizerConfig)
	if !ok {
		return fmt.Errorf("unexpected object type: %T", obj)
	}

	config := original.DeepCopy()

	klog.V(3).Infof("Reconciling OptimizerConfig %s/%s", namespace, name)

	result, err := c.reconciler.Reconcile(ctx, config)
	if err != nil {
		c.eventRecorder.Event(config, corev1.EventTypeWarning, "ReconcileError", err.Error())
		return err
	}

	if result.RequeueAfter > 0 {
		c.workqueue.AddAfter(key, result.RequeueAfter)
	}

	if result.Updated && !equality.Semantic.DeepEqual(original.Status, config.Status) {
		updatedStatus := config.Status
		// Retry on conflict: re-fetch the latest resourceVersion and apply our status.
		var statusErr error
		for attempt := 0; attempt < 5; attempt++ {
			latest, getErr := c.optimizerClient.Get(ctx, config.Name, metav1.GetOptions{})
			if getErr != nil {
				statusErr = getErr
				break
			}
			latest.Status = updatedStatus
			_, statusErr = c.optimizerClient.UpdateStatus(ctx, latest, metav1.UpdateOptions{})
			if statusErr == nil || !k8serrors.IsConflict(statusErr) {
				break
			}
			klog.V(4).Infof("Status update conflict for %s/%s, retrying (attempt %d)", config.Namespace, config.Name, attempt+1)
		}
		if statusErr != nil {
			return fmt.Errorf("failed to update status: %v", statusErr)
		}
	}

	return nil
}

func (c *OptimizerController) handleErr(err error, key string) {
	if err == nil {
		c.workqueue.Forget(key)
		return
	}

	if c.workqueue.NumRequeues(key) < 5 {
		klog.Warningf("Error syncing OptimizerConfig %s: %v", key, err)
		c.workqueue.AddRateLimited(key)
		return
	}

	c.workqueue.Forget(key)
	utilruntime.HandleError(err)
	klog.Warningf("Dropping OptimizerConfig %s out of queue: %v", key, err)
}
