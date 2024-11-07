package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

// StartRealTimeBackup initiates real-time backup using Percona XtraBackup
func (m *BackupManager) StartRealTimeBackup(ctx context.Context) error {
	if !m.config.Backup.RealTimeBackupEnabled {
		return fmt.Errorf("real-time backup not enabled in configuration")
	}

	backupDir := filepath.Join(m.config.Backup.ArchiveDirectory, "realtime")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Create initial full backup
	if err := m.createInitialBackup(ctx, backupDir); err != nil {
		return fmt.Errorf("failed to create initial backup: %w", err)
	}

	// Start incremental backup loop
	go m.realTimeBackupLoop(ctx, backupDir)

	return nil
}

// createInitialBackup creates the initial full backup
func (m *BackupManager) createInitialBackup(ctx context.Context, backupDir string) error {
	m.logger.Info("Starting initial full backup for real-time backup")

	cmd := exec.CommandContext(ctx, "xtrabackup",
		"--backup",
		"--target-dir="+backupDir,
		"--datadir=/var/lib/mysql",
		fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
		"--slave-info",
		"--safe-slave-backup",
		"--galera-info",
		"--compress",
		fmt.Sprintf("--compress-threads=%d", runtime.NumCPU()),
		fmt.Sprintf("--compress-level=%d", m.config.Backup.RealTimeCompressionLevel),
		"--lock-ddl",
		"--lock-ddl-per-table",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initial backup failed: %w, output: %s", err, string(output))
	}

	return nil
}

// realTimeBackupLoop performs periodic incremental backups
func (m *BackupManager) realTimeBackupLoop(ctx context.Context, backupDir string) {
	ticker := time.NewTicker(m.config.Backup.RealTimeBackupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.performIncrementalBackup(ctx, backupDir); err != nil {
				m.logger.Error("Incremental backup failed",
					zap.Error(err),
				)
				continue
			}

			// Cleanup old backups
			if err := m.cleanupOldBackups(backupDir); err != nil {
				m.logger.Error("Failed to cleanup old backups",
					zap.Error(err),
				)
			}
		}
	}
}

// performIncrementalBackup creates an incremental backup
func (m *BackupManager) performIncrementalBackup(ctx context.Context, backupDir string) error {
	incrementalDir := filepath.Join(backupDir, fmt.Sprintf("inc_%d", time.Now().Unix()))

	cmd := exec.CommandContext(ctx, "xtrabackup",
		"--backup",
		"--target-dir="+incrementalDir,
		"--incremental-basedir="+backupDir,
		"--datadir=/var/lib/mysql",
		fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
		"--slave-info",
		"--safe-slave-backup",
		"--galera-info",
		"--compress",
		fmt.Sprintf("--compress-threads=%d", runtime.NumCPU()),
		fmt.Sprintf("--compress-level=%d", m.config.Backup.RealTimeCompressionLevel),
		"--lock-ddl",
		"--lock-ddl-per-table",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("incremental backup failed: %w, output: %s", err, string(output))
	}

	// Upload to storage if configured
	if err := m.uploadRealTimeBackup(ctx, incrementalDir); err != nil {
		m.logger.Error("Failed to upload incremental backup",
			zap.Error(err),
		)
	}

	return nil
}

// uploadRealTimeBackup uploads the backup to configured storage
func (m *BackupManager) uploadRealTimeBackup(ctx context.Context, backupDir string) error {
	if m.storage == nil {
		return nil // Skip if no storage configured
	}

	prefix := filepath.Base(backupDir)
	return m.storage.Upload(ctx, backupDir, prefix)
}

// cleanupOldBackups removes backups older than retention period
func (m *BackupManager) cleanupOldBackups(backupDir string) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	retention := m.config.Backup.RealTimeBackupRetention
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "inc_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > retention {
			path := filepath.Join(backupDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				m.logger.Error("Failed to remove old backup",
					zap.String("path", path),
					zap.Error(err),
				)
				continue
			}
		}
	}

	return nil
}

// archiveBinlogs archives binary logs according to retention policy
func (m *BackupManager) archiveBinlogs() error {
	binlogDir := "/var/lib/mysql"
	files, err := filepath.Glob(filepath.Join(binlogDir, "mysql-bin.*"))
	if err != nil {
		return fmt.Errorf("failed to list binlog files: %w", err)
	}

	retentionPeriod := time.Duration(m.config.Backup.BinlogRetentionHours) * time.Hour
	now := time.Now()

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		// Archive files older than retention period
		if now.Sub(info.ModTime()) > retentionPeriod {
			archivePath := filepath.Join(m.config.Backup.ArchiveDirectory, filepath.Base(file))
			if err := os.Rename(file, archivePath); err != nil {
				m.logger.Error("Failed to archive binlog",
					zap.String("file", file),
					zap.Error(err),
				)
				continue
			}

			m.logger.Info("Archived binlog file",
				zap.String("file", file),
				zap.String("archive", archivePath),
			)
		}
	}

	return nil
}
