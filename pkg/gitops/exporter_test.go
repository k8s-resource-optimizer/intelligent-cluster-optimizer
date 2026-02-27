package gitops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExport(t *testing.T) {
	exporter := NewExporter()

	recommendations := []ResourceRecommendation{
		{
			Namespace:         "production",
			Name:              "api-server",
			Kind:              "Deployment",
			ContainerName:     "api",
			RecommendedCPU:    500,
			RecommendedMemory: 512 * 1024 * 1024,
		},
		{
			Namespace:         "database",
			Name:              "postgres",
			Kind:              "StatefulSet",
			ContainerName:     "postgres",
			RecommendedCPU:    2000,
			RecommendedMemory: 4 * 1024 * 1024 * 1024,
			SetLimits:         true,
		},
	}

	tests := []struct {
		name    string
		config  ExportConfig
		wantErr bool
		verify  func(t *testing.T, result *ExportResult)
	}{
		{
			name: "kustomize export",
			config: ExportConfig{
				Format: FormatKustomize,
			},
			wantErr: false,
			verify: func(t *testing.T, result *ExportResult) {
				// Should have patch files + kustomization.yaml
				if len(result.Files) < 3 {
					t.Errorf("Expected at least 3 files, got %d", len(result.Files))
				}

				// Check for kustomization.yaml
				if _, ok := result.Files["kustomization.yaml"]; !ok {
					t.Error("Expected kustomization.yaml file")
				}

				// Check for patch files
				hasPatch := false
				for filename := range result.Files {
					if strings.HasPrefix(filename, "patch-") {
						hasPatch = true
						break
					}
				}
				if !hasPatch {
					t.Error("Expected patch files")
				}
			},
		},
		{
			name: "kustomize JSON 6902 export",
			config: ExportConfig{
				Format: FormatKustomizeJSON6902,
			},
			wantErr: false,
			verify: func(t *testing.T, result *ExportResult) {
				if len(result.Files) != 2 {
					t.Errorf("Expected 2 patch files, got %d", len(result.Files))
				}

				for filename := range result.Files {
					if !strings.HasSuffix(filename, ".json") {
						t.Errorf("Expected JSON file, got %s", filename)
					}
				}
			},
		},
		{
			name: "helm export",
			config: ExportConfig{
				Format: FormatHelm,
			},
			wantErr: false,
			verify: func(t *testing.T, result *ExportResult) {
				if len(result.Files) != 1 {
					t.Errorf("Expected 1 file, got %d", len(result.Files))
				}

				if _, ok := result.Files["values.yaml"]; !ok {
					t.Error("Expected values.yaml file")
				}
			},
		},
		{
			name: "invalid format",
			config: ExportConfig{
				Format: "invalid",
			},
			wantErr: true,
		},
		{
			name: "empty format",
			config: ExportConfig{
				Format: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := exporter.Export(recommendations, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
				return
			}

			if result.Timestamp.IsZero() {
				t.Error("Expected non-zero timestamp")
			}

			if tt.verify != nil {
				tt.verify(t, result)
			}
		})
	}
}

func TestExportToFile(t *testing.T) {
	exporter := NewExporter()

	// Create temp directory
	tmpDir := t.TempDir()

	recommendations := []ResourceRecommendation{
		{
			Namespace:         "default",
			Name:              "test-app",
			Kind:              "Deployment",
			ContainerName:     "app",
			RecommendedCPU:    500,
			RecommendedMemory: 512 * 1024 * 1024,
		},
	}

	config := ExportConfig{
		Format:     FormatKustomize,
		OutputPath: tmpDir,
	}

	result, err := exporter.Export(recommendations, config)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify files were written
	for filename := range result.Files {
		filePath := filepath.Join(tmpDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist", filename)
		}
	}

	// Verify file contents
	kustomizationPath := filepath.Join(tmpDir, "kustomization.yaml")
	content, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatalf("Failed to read kustomization.yaml: %v", err)
	}

	if len(content) == 0 {
		t.Error("Expected non-empty kustomization.yaml")
	}
}

func TestValidateConfig(t *testing.T) {
	exporter := NewExporter()

	tests := []struct {
		name    string
		config  ExportConfig
		wantErr bool
	}{
		{
			name: "valid kustomize config",
			config: ExportConfig{
				Format: FormatKustomize,
			},
			wantErr: false,
		},
		{
			name: "valid helm config",
			config: ExportConfig{
				Format: FormatHelm,
			},
			wantErr: false,
		},
		{
			name: "valid with git config",
			config: ExportConfig{
				Format: FormatKustomize,
				GitConfig: &GitConfig{
					RepositoryURL: "https://github.com/user/repo.git",
				},
			},
			wantErr: false,
		},
		{
			name: "empty format",
			config: ExportConfig{
				Format: "",
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			config: ExportConfig{
				Format: "unknown",
			},
			wantErr: true,
		},
		{
			name: "git config without URL",
			config: ExportConfig{
				Format: FormatKustomize,
				GitConfig: &GitConfig{
					Branch: "main",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exporter.ValidateConfig(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExportToString(t *testing.T) {
	recommendations := []ResourceRecommendation{
		{
			Namespace:         "production",
			Name:              "api-server",
			Kind:              "Deployment",
			ContainerName:     "api",
			RecommendedCPU:    500,
			RecommendedMemory: 512 * 1024 * 1024,
		},
	}

	tests := []struct {
		name    string
		format  ExportFormat
		wantErr bool
		verify  func(t *testing.T, output string)
	}{
		{
			name:    "kustomize to string",
			format:  FormatKustomize,
			wantErr: false,
			verify: func(t *testing.T, output string) {
				if !strings.Contains(output, "# File:") {
					t.Error("Expected file headers")
				}
				if !strings.Contains(output, "kustomization.yaml") {
					t.Error("Expected kustomization.yaml in output")
				}
			},
		},
		{
			name:    "helm to string",
			format:  FormatHelm,
			wantErr: false,
			verify: func(t *testing.T, output string) {
				if !strings.Contains(output, "values.yaml") {
					t.Error("Expected values.yaml in output")
				}
				if !strings.Contains(output, "500m") {
					t.Error("Expected CPU value in output")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ExportToString(recommendations, tt.format)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if output == "" {
				t.Error("Expected non-empty output")
			}

			if tt.verify != nil {
				tt.verify(t, output)
			}
		})
	}
}

func TestExportMultipleFormats(t *testing.T) {
	exporter := NewExporter()
	tmpDir := t.TempDir()

	recommendations := []ResourceRecommendation{
		{
			Namespace:         "production",
			Name:              "api-server",
			Kind:              "Deployment",
			ContainerName:     "api",
			RecommendedCPU:    500,
			RecommendedMemory: 512 * 1024 * 1024,
		},
	}

	formats := []ExportFormat{
		FormatKustomize,
		FormatKustomizeJSON6902,
		FormatHelm,
	}

	for _, format := range formats {
		t.Run(string(format), func(t *testing.T) {
			outputPath := filepath.Join(tmpDir, string(format))

			config := ExportConfig{
				Format:     format,
				OutputPath: outputPath,
			}

			result, err := exporter.Export(recommendations, config)
			if err != nil {
				t.Fatalf("Export failed for format %s: %v", format, err)
			}

			if len(result.Files) == 0 {
				t.Errorf("Expected files for format %s", format)
			}

			// Verify directory was created
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Errorf("Expected directory %s to exist", outputPath)
			}
		})
	}
}

func TestExportWithConfidence(t *testing.T) {
	exporter := NewExporter()

	recommendations := []ResourceRecommendation{
		{
			Namespace:         "production",
			Name:              "api-server",
			Kind:              "Deployment",
			ContainerName:     "api",
			RecommendedCPU:    500,
			RecommendedMemory: 512 * 1024 * 1024,
			Confidence:        95.5,
			Reason:            "Based on 7 days of P95 usage data",
		},
	}

	config := ExportConfig{
		Format: FormatHelm,
	}

	result, err := exporter.Export(recommendations, config)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	values := result.Files["values.yaml"]
	if !strings.Contains(values, "confidence") {
		t.Error("Expected confidence in values")
	}
	if !strings.Contains(values, "95.5%") {
		t.Error("Expected confidence percentage in values")
	}
	if !strings.Contains(values, "Based on 7 days") {
		t.Error("Expected reason in values")
	}
}

func TestExportEmptyRecommendations(t *testing.T) {
	exporter := NewExporter()

	config := ExportConfig{
		Format: FormatKustomize,
	}

	result, err := exporter.Export([]ResourceRecommendation{}, config)

	// Should succeed but produce empty/minimal output
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Result should still be valid
	if result == nil {
		t.Error("Expected non-nil result")
	}
}
