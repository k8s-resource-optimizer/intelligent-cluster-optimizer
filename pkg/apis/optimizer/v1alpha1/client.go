package v1alpha1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// OptimizerConfigClient is a client for OptimizerConfig resources
type OptimizerConfigClient struct {
	restClient rest.Interface
	namespace  string
}

// NewOptimizerConfigClient creates a new client for OptimizerConfig resources
func NewOptimizerConfigClient(config *rest.Config, namespace string) (*OptimizerConfigClient, error) {
	// Create a copy of the config to modify
	configCopy := *config
	configCopy.GroupVersion = &SchemeGroupVersion
	configCopy.APIPath = "/apis"
	configCopy.ContentType = runtime.ContentTypeJSON

	// Create the scheme and add our types
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Add codecs
	configCopy.NegotiatedSerializer = serializer.NewCodecFactory(scheme)

	// Create the REST client
	client, err := rest.RESTClientFor(&configCopy)
	if err != nil {
		return nil, err
	}

	return &OptimizerConfigClient{
		restClient: client,
		namespace:  namespace,
	}, nil
}

// List returns all OptimizerConfigs in the namespace
func (c *OptimizerConfigClient) List(ctx context.Context, opts metav1.ListOptions) (*OptimizerConfigList, error) {
	result := &OptimizerConfigList{}
	err := c.restClient.
		Get().
		Namespace(c.namespace).
		Resource("optimizerconfigs").
		VersionedParams(&opts, metav1.ParameterCodec).
		Do(ctx).
		Into(result)
	return result, err
}

// Get returns a specific OptimizerConfig by name
func (c *OptimizerConfigClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*OptimizerConfig, error) {
	result := &OptimizerConfig{}
	err := c.restClient.
		Get().
		Namespace(c.namespace).
		Resource("optimizerconfigs").
		Name(name).
		VersionedParams(&opts, metav1.ParameterCodec).
		Do(ctx).
		Into(result)
	return result, err
}

// Watch watches for changes to OptimizerConfigs
func (c *OptimizerConfigClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.restClient.
		Get().
		Namespace(c.namespace).
		Resource("optimizerconfigs").
		VersionedParams(&opts, metav1.ParameterCodec).
		Watch(ctx)
}

// UpdateStatus updates the status of an OptimizerConfig
func (c *OptimizerConfigClient) UpdateStatus(ctx context.Context, config *OptimizerConfig, opts metav1.UpdateOptions) (*OptimizerConfig, error) {
	result := &OptimizerConfig{}
	err := c.restClient.
		Put().
		Namespace(c.namespace).
		Resource("optimizerconfigs").
		Name(config.Name).
		SubResource("status").
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(config).
		Do(ctx).
		Into(result)
	return result, err
}

// Patch applies a patch to an OptimizerConfig.
func (c *OptimizerConfigClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*OptimizerConfig, error) {
	result := &OptimizerConfig{}
	err := c.restClient.
		Patch(pt).
		Namespace(c.namespace).
		Resource("optimizerconfigs").
		Name(name).
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return result, err
}

// GroupVersionResource returns the GVR for OptimizerConfig
func GroupVersionResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "optimizerconfigs",
	}
}
