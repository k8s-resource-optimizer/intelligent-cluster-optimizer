package storage

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"intelligent-cluster-optimizer/pkg/models"

	"go.uber.org/zap"
)

// BackupManager manages periodic backups of the metrics storage
type BackupManager struct {
	storage        *InMemoryStorage
	storageDir     string
	retentionCount int
	backupInterval time.Duration
	logger         *zap.Logger
	stopCh         chan struct{}
	enabled        bool
}

// BackupConfig contains configuration for the backup manager
type BackupConfig struct {
	Enabled        bool
	StorageDir     string
	RetentionCount int
	BackupInterval time.Duration
}

// NewBackupManager creates a new backup manager
func NewBackupManager(storage *InMemoryStorage, config BackupConfig, logger *zap.Logger) *BackupManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Set defaults
	if config.StorageDir == "" {
		config.StorageDir = "/var/lib/optimizer/backups"
	}
	if config.RetentionCount == 0 {
		config.RetentionCount = 10
	}
	if config.BackupInterval == 0 {
		config.BackupInterval = 5 * time.Minute
	}

	return &BackupManager{
		storage:        storage,
		storageDir:     config.StorageDir,
		retentionCount: config.RetentionCount,
		backupInterval: config.BackupInterval,
		logger:         logger,
		stopCh:         make(chan struct{}),
		enabled:        config.Enabled,
	}
}

// Start begins the periodic backup process
func (bm *BackupManager) Start() error {
	if !bm.enabled {
		bm.logger.Info("backup manager disabled, skipping start")
		return nil
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(bm.storageDir, 0750); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	bm.logger.Info("starting backup manager",
		zap.String("storage_dir", bm.storageDir),
		zap.Int("retention_count", bm.retentionCount),
		zap.Duration("interval", bm.backupInterval))

	// Start periodic backup loop
	go bm.backupLoop()

	return nil
}

// Stop stops the backup manager
func (bm *BackupManager) Stop() {
	if !bm.enabled {
		return
	}

	bm.logger.Info("stopping backup manager")
	close(bm.stopCh)
}

// backupLoop runs the periodic backup process
func (bm *BackupManager) backupLoop() {
	ticker := time.NewTicker(bm.backupInterval)
	defer ticker.Stop()

	// Perform initial backup immediately
	if err := bm.CreateBackup(); err != nil {
		bm.logger.Error("initial backup failed", zap.Error(err))
	}

	for {
		select {
		case <-ticker.C:
			if err := bm.CreateBackup(); err != nil {
				bm.logger.Error("periodic backup failed", zap.Error(err))
			}
		case <-bm.stopCh:
			// Perform final backup before stopping
			if err := bm.CreateBackup(); err != nil {
				bm.logger.Error("final backup failed", zap.Error(err))
			}
			return
		}
	}
}

// CreateBackup creates a new backup file
func (bm *BackupManager) CreateBackup() error {
	start := time.Now()

	// Take snapshot of current state
	snapshot := bm.storage.Snapshot()

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("backup-%s.json.gz", timestamp)
	backupPath := filepath.Join(bm.storageDir, filename)

	// Use temp file for atomic writes
	tempPath := backupPath + ".tmp"

	// Create gzip compressed file
	// #nosec G304 - tempPath is constructed from validated storageDir config, not user input
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp backup file: %w", err)
	}

	gzipWriter := gzip.NewWriter(file)

	// Marshal and write data
	encoder := json.NewEncoder(gzipWriter)
	if err := encoder.Encode(snapshot); err != nil {
		_ = gzipWriter.Close()  // #nosec G104 - Cleanup, error already being returned
		_ = file.Close()        // #nosec G104 - Cleanup, error already being returned
		_ = os.Remove(tempPath) // #nosec G104 - Best effort cleanup
		return fmt.Errorf("failed to encode backup data: %w", err)
	}

	// Close writers
	if err := gzipWriter.Close(); err != nil {
		_ = file.Close()        // #nosec G104 - Cleanup, error already being returned
		_ = os.Remove(tempPath) // #nosec G104 - Best effort cleanup
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath) // #nosec G104 - Best effort cleanup
		return fmt.Errorf("failed to close backup file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, backupPath); err != nil {
		_ = os.Remove(tempPath) // #nosec G104 - Best effort cleanup
		return fmt.Errorf("failed to rename backup file: %w", err)
	}

	// Get file size for logging
	fileInfo, _ := os.Stat(backupPath)
	fileSize := int64(0)
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	duration := time.Since(start)
	bm.logger.Info("backup created successfully",
		zap.String("file", filename),
		zap.Int64("size_bytes", fileSize),
		zap.Duration("duration", duration),
		zap.Int("metric_pods", len(snapshot)))

	// Clean up old backups
	if err := bm.cleanupOldBackups(); err != nil {
		bm.logger.Warn("failed to cleanup old backups", zap.Error(err))
	}

	return nil
}

// cleanupOldBackups removes old backup files beyond retention count
func (bm *BackupManager) cleanupOldBackups() error {
	files, err := filepath.Glob(filepath.Join(bm.storageDir, "backup-*.json.gz"))
	if err != nil {
		return fmt.Errorf("failed to list backup files: %w", err)
	}

	// Sort files by name (which includes timestamp)
	sort.Strings(files)

	// Remove oldest files if we exceed retention count
	if len(files) > bm.retentionCount {
		filesToDelete := files[:len(files)-bm.retentionCount]
		for _, file := range filesToDelete {
			if err := os.Remove(file); err != nil {
				bm.logger.Warn("failed to delete old backup", zap.String("file", file), zap.Error(err))
			} else {
				bm.logger.Info("deleted old backup", zap.String("file", filepath.Base(file)))
			}
		}
	}

	return nil
}

// ListBackups returns a list of available backup files
func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	files, err := filepath.Glob(filepath.Join(bm.storageDir, "backup-*.json.gz"))
	if err != nil {
		return nil, fmt.Errorf("failed to list backup files: %w", err)
	}

	// Sort by name descending (newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	var backups []BackupInfo
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Filename:  filepath.Base(file),
			Path:      file,
			Size:      info.Size(),
			Timestamp: info.ModTime(),
		})
	}

	return backups, nil
}

// RestoreFromBackup restores storage from a backup file
func (bm *BackupManager) RestoreFromBackup(backupPath string) error {
	bm.logger.Info("restoring from backup", zap.String("file", backupPath))

	// Open and decompress backup file
	file, err := os.Open(filepath.Clean(backupPath))
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Decode backup data
	var data map[string][]models.PodMetric
	decoder := json.NewDecoder(gzipReader)
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode backup data: %w", err)
	}

	// Restore to storage
	bm.storage.Restore(data)

	bm.logger.Info("backup restored successfully",
		zap.String("file", backupPath),
		zap.Int("pods", len(data)))

	return nil
}

// RestoreLatest restores from the most recent backup
func (bm *BackupManager) RestoreLatest() error {
	backups, err := bm.ListBackups()
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	if len(backups) == 0 {
		return fmt.Errorf("no backups available")
	}

	// Restore from the newest backup (first in list)
	return bm.RestoreFromBackup(backups[0].Path)
}

// BackupInfo contains information about a backup file
type BackupInfo struct {
	Filename  string
	Path      string
	Size      int64
	Timestamp time.Time
}
