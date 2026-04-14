package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"

	"go.uber.org/zap"
)

func TestBackupManager_CreateBackup(t *testing.T) {
	// Create temp directory for backups
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage with test data
	storage := NewStorage()
	storage.Add(models.PodMetric{
		PodName:   "test-pod-1",
		Namespace: "default",
		Timestamp: time.Now(),
	})
	storage.Add(models.PodMetric{
		PodName:   "test-pod-2",
		Namespace: "production",
		Timestamp: time.Now(),
	})

	// Create backup manager
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled:        true,
		StorageDir:     tempDir,
		RetentionCount: 5,
		BackupInterval: 1 * time.Minute,
	}, zap.NewNop())

	// Create backup
	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup file exists
	files, err := filepath.Glob(filepath.Join(tempDir, "backup-*.json.gz"))
	if err != nil {
		t.Fatalf("Failed to list backup files: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 backup file, got %d", len(files))
	}

	// Verify file is not empty
	fileInfo, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("Failed to stat backup file: %v", err)
	}

	if fileInfo.Size() == 0 {
		t.Error("Backup file is empty")
	}
}

func TestBackupManager_RestoreFromBackup(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage with initial data
	originalStorage := NewStorage()
	originalStorage.Add(models.PodMetric{
		PodName:   "test-pod-1",
		Namespace: "default",
		Timestamp: time.Now(),
	})
	originalStorage.Add(models.PodMetric{
		PodName:   "test-pod-2",
		Namespace: "production",
		Timestamp: time.Now(),
	})

	// Create backup
	bm := NewBackupManager(originalStorage, nil, BackupConfig{
		Enabled:        true,
		StorageDir:     tempDir,
		RetentionCount: 5,
	}, zap.NewNop())

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Create new empty storage
	newStorage := NewStorage()

	// Create new backup manager for restore
	restoreBM := NewBackupManager(newStorage, nil, BackupConfig{
		Enabled:    true,
		StorageDir: tempDir,
	}, zap.NewNop())

	// Restore from backup
	if err := restoreBM.RestoreLatest(); err != nil {
		t.Fatalf("Failed to restore backup: %v", err)
	}

	// Verify data was restored
	if newStorage.GetMetricCount() != 2 {
		t.Errorf("Expected 2 metrics after restore, got %d", newStorage.GetMetricCount())
	}

	// Verify specific pods were restored
	allMetrics := newStorage.GetAllMetrics()
	if _, exists := allMetrics["test-pod-1"]; !exists {
		t.Error("test-pod-1 not found after restore")
	}
	if _, exists := allMetrics["test-pod-2"]; !exists {
		t.Error("test-pod-2 not found after restore")
	}
}

func TestBackupManager_RetentionPolicy(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage
	storage := NewStorage()
	storage.Add(models.PodMetric{
		PodName:   "test-pod",
		Namespace: "default",
		Timestamp: time.Now(),
	})

	// Create backup manager with retention of 3
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled:        true,
		StorageDir:     tempDir,
		RetentionCount: 3,
	}, zap.NewNop())

	// Create 5 backups
	for i := 0; i < 5; i++ {
		if err := bm.CreateBackup(); err != nil {
			t.Fatalf("Failed to create backup %d: %v", i, err)
		}
		time.Sleep(1 * time.Second) // Ensure unique timestamps in filename
	}

	// Verify only 3 backups remain
	files, err := filepath.Glob(filepath.Join(tempDir, "backup-*.json.gz"))
	if err != nil {
		t.Fatalf("Failed to list backup files: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 backups after retention cleanup, got %d", len(files))
	}
}

func TestBackupManager_ListBackups(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage
	storage := NewStorage()
	storage.Add(models.PodMetric{
		PodName:   "test-pod",
		Namespace: "default",
		Timestamp: time.Now(),
	})

	// Create backup manager
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled:        true,
		StorageDir:     tempDir,
		RetentionCount: 10,
	}, zap.NewNop())

	// Create multiple backups
	for i := 0; i < 3; i++ {
		if err := bm.CreateBackup(); err != nil {
			t.Fatalf("Failed to create backup %d: %v", i, err)
		}
		time.Sleep(1 * time.Second) // Ensure unique timestamps in filename
	}

	// List backups
	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("Expected 3 backups, got %d", len(backups))
	}

	// Verify backups are sorted (newest first)
	for i := 0; i < len(backups)-1; i++ {
		if backups[i].Timestamp.Before(backups[i+1].Timestamp) {
			t.Error("Backups not sorted correctly (should be newest first)")
		}
	}

	// Verify backup info fields
	for _, backup := range backups {
		if backup.Filename == "" {
			t.Error("Backup filename is empty")
		}
		if backup.Path == "" {
			t.Error("Backup path is empty")
		}
		if backup.Size == 0 {
			t.Error("Backup size is zero")
		}
		if backup.Timestamp.IsZero() {
			t.Error("Backup timestamp is zero")
		}
	}
}

func TestBackupManager_DisabledBackup(t *testing.T) {
	// Create storage
	storage := NewStorage()

	// Create backup manager with disabled backups
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled: false,
	}, zap.NewNop())

	// Start should not return error
	if err := bm.Start(); err != nil {
		t.Errorf("Start() should not error when disabled: %v", err)
	}

	// Stop should not panic
	bm.Stop()
}

func TestBackupManager_AtomicWrites(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage
	storage := NewStorage()
	storage.Add(models.PodMetric{
		PodName:   "test-pod",
		Namespace: "default",
		Timestamp: time.Now(),
	})

	// Create backup manager
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled:    true,
		StorageDir: tempDir,
	}, zap.NewNop())

	// Create backup
	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify no .tmp files remain
	tempFiles, err := filepath.Glob(filepath.Join(tempDir, "*.tmp"))
	if err != nil {
		t.Fatalf("Failed to list temp files: %v", err)
	}

	if len(tempFiles) > 0 {
		t.Errorf("Found %d temporary files after backup (should be cleaned up)", len(tempFiles))
	}
}

func TestBackupManager_EmptyStorage(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create empty storage
	storage := NewStorage()

	// Create backup manager
	bm := NewBackupManager(storage, nil, BackupConfig{
		Enabled:    true,
		StorageDir: tempDir,
	}, zap.NewNop())

	// Create backup of empty storage
	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("Failed to create backup of empty storage: %v", err)
	}

	// Verify backup file exists
	files, err := filepath.Glob(filepath.Join(tempDir, "backup-*.json.gz"))
	if err != nil {
		t.Fatalf("Failed to list backup files: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 backup file, got %d", len(files))
	}
}

func TestSnapshot_DeepCopy(t *testing.T) {
	// Create storage with data
	storage := NewStorage()
	originalMetric := models.PodMetric{
		PodName:   "test-pod",
		Namespace: "default",
		Timestamp: time.Now(),
	}
	storage.Add(originalMetric)

	// Take snapshot
	snapshot := storage.Snapshot()

	// Modify original storage
	storage.Add(models.PodMetric{
		PodName:   "test-pod-2",
		Namespace: "production",
		Timestamp: time.Now(),
	})

	// Verify snapshot was not affected
	if len(snapshot) != 1 {
		t.Errorf("Snapshot should have 1 pod, got %d", len(snapshot))
	}

	// Verify storage has 2 pods
	if storage.GetMetricCount() != 2 {
		t.Errorf("Storage should have 2 metrics, got %d", storage.GetMetricCount())
	}
}

func TestRestore_DeepCopy(t *testing.T) {
	// Create backup data
	backupData := map[string][]models.PodMetric{
		"test-pod": {
			{
				PodName:   "test-pod",
				Namespace: "default",
				Timestamp: time.Now(),
			},
		},
	}

	// Create storage and restore
	storage := NewStorage()
	storage.Restore(backupData)

	// Modify backup data
	backupData["test-pod"] = append(backupData["test-pod"], models.PodMetric{
		PodName:   "test-pod",
		Namespace: "default",
		Timestamp: time.Now(),
	})

	// Verify storage was not affected
	allMetrics := storage.GetAllMetrics()
	if len(allMetrics["test-pod"]) != 1 {
		t.Errorf("Storage should have 1 metric for test-pod, got %d", len(allMetrics["test-pod"]))
	}
}
