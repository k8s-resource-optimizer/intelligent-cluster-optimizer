package gitops

import (
	"time"
)

// ExportFormat defines the format for exporting recommendations
type ExportFormat string

const (
	// FormatKustomize exports as Kustomize strategic merge patches
	FormatKustomize ExportFormat = "kustomize"

	// FormatKustomizeJSON6902 exports as Kustomize JSON 6902 patches
	FormatKustomizeJSON6902 ExportFormat = "kustomize-json6902"

	// FormatHelm exports as Helm values overrides
	FormatHelm ExportFormat = "helm"
)

// PatchType defines the type of patch to generate
type PatchType string

const (
	// PatchTypeStrategicMerge is a strategic merge patch (default for Kustomize)
	PatchTypeStrategicMerge PatchType = "strategic-merge"

	// PatchTypeJSON6902 is a JSON 6902 patch
	PatchTypeJSON6902 PatchType = "json-6902"
)

// ResourceRecommendation represents a resource optimization recommendation
type ResourceRecommendation struct {
	// Namespace of the workload
	Namespace string

	// Name of the workload
	Name string

	// Kind of workload (Deployment, StatefulSet, DaemonSet)
	Kind string

	// ContainerName is the container to update
	ContainerName string

	// RecommendedCPU in millicores
	RecommendedCPU int64

	// RecommendedMemory in bytes
	RecommendedMemory int64

	// SetLimits indicates whether to set limits equal to requests
	SetLimits bool

	// Confidence score (0-100)
	Confidence float64

	// Reason for the recommendation
	Reason string
}

// ExportConfig configures the export behavior
type ExportConfig struct {
	// Format specifies the export format
	Format ExportFormat

	// OutputPath is the directory where patches will be written
	OutputPath string

	// PatchType specifies the patch type (for Kustomize)
	PatchType PatchType

	// GitConfig contains Git repository configuration
	GitConfig *GitConfig

	// Metadata contains additional metadata to include
	Metadata map[string]string
}

// GitConfig configures Git operations
type GitConfig struct {
	// RepositoryURL is the Git repository URL
	RepositoryURL string

	// Branch is the branch to commit to (default: creates new branch)
	Branch string

	// BaseBranch is the base branch to create PR against (default: main)
	BaseBranch string

	// CommitMessage is the commit message template
	CommitMessage string

	// AuthToken is the Git authentication token.
	// SECURITY: This field is intentionally exported for configuration purposes.
	// Users should NEVER hardcode tokens - instead use:
	//   - Environment variables: ${GIT_TOKEN}
	//   - Kubernetes secrets mounted as env vars
	//   - Secret management systems (Vault, AWS Secrets Manager, etc.)
	// The token is used at runtime and never logged or persisted.
	// #nosec G117 - Exported by design, secure usage documented
	AuthToken string

	// Author is the commit author
	Author GitAuthor

	// CreatePR indicates whether to create a pull request
	CreatePR bool

	// PRTitle is the pull request title
	PRTitle string

	// PRBody is the pull request body
	PRBody string
}

// GitAuthor represents a Git commit author
type GitAuthor struct {
	Name  string
	Email string
}

// ExportResult contains the result of an export operation
type ExportResult struct {
	// Files contains the generated files with their content
	Files map[string]string

	// CommitHash is the Git commit hash (if committed)
	CommitHash string

	// Branch is the Git branch name
	Branch string

	// PRURL is the pull request URL (if created)
	PRURL string

	// Errors contains any errors encountered
	Errors []error

	// Timestamp of the export
	Timestamp time.Time
}

// KustomizePatch represents a Kustomize patch
type KustomizePatch struct {
	// APIVersion of the resource
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`

	// Kind of the resource
	Kind string `json:"kind" yaml:"kind"`

	// Metadata contains the resource metadata
	Metadata KustomizeMetadata `json:"metadata" yaml:"metadata"`

	// Spec contains the patch specification
	Spec interface{} `json:"spec,omitempty" yaml:"spec,omitempty"`
}

// KustomizeMetadata represents Kustomize patch metadata
type KustomizeMetadata struct {
	Name      string            `json:"name" yaml:"name"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// JSON6902Patch represents a JSON 6902 patch operation
type JSON6902Patch struct {
	Op    string      `json:"op" yaml:"op"`
	Path  string      `json:"path" yaml:"path"`
	Value interface{} `json:"value,omitempty" yaml:"value,omitempty"`
}

// HelmValues represents Helm values structure
type HelmValues map[string]interface{}

// Exporter is the interface for exporting recommendations
type Exporter interface {
	// Export exports recommendations in the specified format
	Export(recommendations []ResourceRecommendation, config ExportConfig) (*ExportResult, error)

	// ValidateConfig validates the export configuration
	ValidateConfig(config ExportConfig) error
}

// KustomizeGenerator generates Kustomize patches
type KustomizeGenerator interface {
	// GenerateStrategicMerge generates a strategic merge patch
	GenerateStrategicMerge(rec ResourceRecommendation) (string, error)

	// GenerateJSON6902 generates a JSON 6902 patch
	GenerateJSON6902(rec ResourceRecommendation) (string, error)

	// GenerateKustomization generates a kustomization.yaml file
	GenerateKustomization(patchFiles []string) (string, error)
}

// HelmGenerator generates Helm values
type HelmGenerator interface {
	// GenerateValues generates Helm values for recommendations
	GenerateValues(recommendations []ResourceRecommendation) (string, error)

	// MergeValues merges new values with existing values
	MergeValues(existing, new HelmValues) (HelmValues, error)
}

// GitOperator performs Git operations
type GitOperator interface {
	// Clone clones a repository
	Clone(url, authToken string) (string, error)

	// CreateBranch creates a new branch
	CreateBranch(repoPath, branchName string) error

	// Commit commits changes
	Commit(repoPath, message string, author GitAuthor) (string, error)

	// Push pushes commits to remote
	Push(repoPath, branch, authToken string) error

	// CreatePullRequest creates a pull request
	CreatePullRequest(config GitConfig, commitHash string) (string, error)
}
