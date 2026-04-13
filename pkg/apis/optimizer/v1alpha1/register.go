package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName is the group name for the optimizer API
	GroupName = "optimizer.cluster.io"
	// Version is the version for the optimizer API
	Version = "v1alpha1"
)

// SchemeGroupVersion is the group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{
	Group:   GroupName,
	Version: Version,
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the types in this group-version to the given scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// internalGV is the internal (unversioned) group version required by the
// codec factory so watch stream events can be decoded without error.
var internalGV = schema.GroupVersion{Group: GroupName, Version: runtime.APIVersionInternal}

// addKnownTypes adds the set of types defined in this package to the supplied scheme
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&OptimizerConfig{},
		&OptimizerConfigList{},
	)
	// Register the same types for the internal version so the reflector's
	// watch stream decoder does not log "no kind registered for internal version".
	scheme.AddKnownTypes(internalGV,
		&OptimizerConfig{},
		&OptimizerConfigList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
