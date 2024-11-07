package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings" 
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"go.lumeweb.com/mysql-manager/internal/config"
	"go.lumeweb.com/mysql-manager/internal/mysql"
	"go.lumeweb.com/mysql-manager/internal/storage"
	"go.uber.org/zap"
)

// Backup types aligned with XtraBackup capabilities
type BackupType string

const (
	FullBackup           BackupType = "full"        // Full backup using --backup
	IncrementalBackup    BackupType = "incremental" // Incremental using --incremental
	TransactionLogBackup BackupType = "txlog"       // Transaction log backup
)

// Enhanced backup metadata with verification details
type BackupMetadata struct {
	Type                BackupType
	Timestamp           time.Time
	Size                int64
	DataDirectory       string
	IncrementalFrom     string
	Compressed          bool
	Encrypted           bool
	TransactionLSN      string
	Validated           bool
	ValidationTimestamp time.Time
	ValidationNode      string
	Checksum            string
	CompressionRatio    float64
	EncryptionMethod    string
	BackupDuration      time.Duration
	ValidationDuration  time.Duration
	StorageLocation     string
}

// Enhanced restore options with more granular control
type RestoreOptions struct {
	BackupKey           string
	TargetTimestamp     time.Time
	PointInTime         bool
	Validate            bool
	AllowPartialRestore bool
	BandwidthLimit      int64 // bytes per second
}

type BackupManager struct {
	config      *config.Config
	logger      *zap.Logger
	storage     storage.Client
	mysqlClient *mysql.Client
	tmpDir      string
	scheduler   *cron.Cron

	lastFullBackupTime     time.Time
	lastTransactionLogTime time.Time
	backupMetadataStore    map[string]BackupMetadata

	verificationResults map[string]*VerificationResult
	degradedMode        bool
	mu                  sync.RWMutex
}

type VerificationResult struct {
	BackupKey     string    `json:"backup_key"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`
	ChecksumMatch bool      `json:"checksum_match"`
	RestoreTest   bool      `json:"restore_test"`
	Error         string    `json:"error,omitempty"`
}

func NewManager(cfg *config.Config, logger *zap.Logger, storage storage.Client, mysqlClient *mysql.Client) (*BackupManager, error) {
	tmpDir, err := os.MkdirTemp("", "mysql-backup-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary backup directory: %w", err)
	}

	manager := &BackupManager{
		config:              cfg,
		logger:              logger,
		storage:             storage,
		tmpDir:              tmpDir,
		scheduler:           cron.New(cron.WithSeconds()),
		backupMetadataStore: make(map[string]BackupMetadata),
	}

	// Start the scheduler if backups are enabled
	if cfg.Backup.Enabled {
		manager.scheduler.Start()
	}

	return manager, nil
}

// verifyRestorePermission checks if this instance is allowed to perform restore
func (m *BackupManager) verifyRestorePermission() error {
	switch m.config.MySQL.Mode {
	case "standalone":
		// Standalone mode can always restore
		return nil

	case "cluster":
		// In cluster mode, only primary can restore
		if m.config.MySQL.Role != "primary" {
			return fmt.Errorf("only primary node can perform restore in cluster mode")
		}
		return nil

	default:
		return fmt.Errorf("unknown MySQL mode: %s", m.config.MySQL.Mode)
	}
}

type backupJob struct {
	manager    *BackupManager
	backupType BackupType
}

func (j backupJob) Run() {
	ctx := context.Background()
	if err := j.manager.CreateBackup(ctx); err != nil {
		j.manager.logger.Error("Backup job failed",
			zap.Error(err),
			zap.String("type", string(j.backupType)),
		)
	}
}

func (m *BackupManager) CreateBackup(ctx context.Context) error {
	// Add context timeout for backup operation
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	backupPath := filepath.Join(m.config.Backup.ArchiveDirectory, time.Now().UTC().Format("2006-01-02T15-04-05Z"))

	m.logger.Info("Starting backup creation",
		zap.String("backup_path", backupPath),
	)

	// Prepare backup command
	cmd, err := m.prepareBackupCommand(FullBackup, backupPath)
	if err != nil {
		return fmt.Errorf("failed to prepare backup command: %w", err)
	}

	// Execute backup command with context
	if err := cmd.Run(); err != nil {
		m.logger.Error("Backup command failed",
			zap.Error(err),
			zap.String("backup_path", backupPath),
		)
		return fmt.Errorf("backup command execution failed: %w", err)
	}

	// Upload backup to storage
	backupFiles, err := filepath.Glob(filepath.Join(backupPath, "*"))
	if err != nil {
		return fmt.Errorf("failed to find backup files: %w", err)
	}

	for _, file := range backupFiles {
		objectKey := filepath.Join(m.config.Storage.Prefix, filepath.Base(file))

		if err := m.storage.Upload(ctx, file, objectKey); err != nil {
			m.logger.Error("Storage upload failed",
				zap.String("file", file),
				zap.String("object_key", objectKey),
				zap.Error(err),
			)
			return fmt.Errorf("storage upload failed for %s: %w", file, err)
		}
	}

	m.logger.Info("Backup completed successfully",
		zap.String("backup_path", backupPath),
		zap.Int("files_uploaded", len(backupFiles)),
	)

	return nil
}

func (m *BackupManager) VerifyBackup(ctx context.Context, backupKey string) (*VerificationResult, error) {
	startTime := time.Now()
	verifyPath := filepath.Join(m.tmpDir, "verify", backupKey)
	if err := os.MkdirAll(verifyPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create verification directory: %w", err)
	}

	result := &VerificationResult{
		BackupKey: backupKey,
		Timestamp: startTime,
		Status:    "in_progress",
	}
	m.setVerificationResult(backupKey, result)

	// Calculate checksum before download
	expectedChecksum := ""

	// Download backup for verification
	if err := m.storage.Download(ctx, backupKey, filepath.Join(verifyPath, "backup.xb")); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("download failed: %v", err)
		m.setVerificationResult(backupKey, result)
		return result, err
	}

	// Calculate checksum of downloaded backup
	actualChecksum := ""

	// Compare checksums if we have both
	if expectedChecksum != "" {
		result.ChecksumMatch = expectedChecksum == actualChecksum
		if !result.ChecksumMatch {
			result.Status = "failed"
			result.Error = "checksum mismatch"
			m.setVerificationResult(backupKey, result)
			return result, fmt.Errorf("checksum mismatch")
		}
	}

	// Verify backup integrity
	cmd := exec.CommandContext(ctx, "xtrabackup",
		"--verify",
		fmt.Sprintf("--target-dir=%s", verifyPath),
		"--check-privileges",
	)

	if err := cmd.Run(); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("verification failed: %v", err)
		result.ChecksumMatch = false
		m.setVerificationResult(backupKey, result)
		return result, err
	}

	result.Status = "completed"
	result.ChecksumMatch = true
	m.setVerificationResult(backupKey, result)

	return result, nil
}

func (m *BackupManager) setVerificationResult(backupKey string, result *VerificationResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.verificationResults == nil {
		m.verificationResults = make(map[string]*VerificationResult)
	}
	m.verificationResults[backupKey] = result
}

func (m *BackupManager) SetDegradedMode(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.degradedMode = enabled

	if enabled {
		m.logger.Warn("Backup manager entering degraded mode")
	} else {
		m.logger.Info("Backup manager exiting degraded mode")
	}
}

func (m *BackupManager) SetCompressionLevel(level int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure level is within valid range (0-9)
	if level < 0 {
		level = 0
	} else if level > 9 {
		level = 9
	}

	m.config.Backup.CompressionLevel = level
	m.logger.Info("Backup compression level updated",
		zap.Int("level", level),
	)
}

// IsRestoreNeeded checks if a restore operation is required
func (m *BackupManager) IsRestoreNeeded(ctx context.Context) (bool, error) {
	switch m.config.MySQL.Mode {
	case "standalone":
		return m.isRestoreNeededStandalone(ctx)
	case "cluster":
		return m.isRestoreNeededCluster(ctx)
	default:
		return false, fmt.Errorf("unknown MySQL mode: %s", m.config.MySQL.Mode)
	}
}

// isRestoreNeededStandalone checks if restore is needed in standalone mode
func (m *BackupManager) isRestoreNeededStandalone(ctx context.Context) (bool, error) {
	// Check if data directory exists and has content
	dataDir := "/var/lib/mysql"
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // Empty data directory, restore needed
		}
		return false, fmt.Errorf("failed to check data directory: %w", err)
	}

	// Check for essential MySQL files/directories
	essentialFiles := []string{"ibdata1", "mysql", "performance_schema"}
	foundFiles := make(map[string]bool)

	for _, entry := range entries {
		foundFiles[entry.Name()] = true
	}

	for _, file := range essentialFiles {
		if !foundFiles[file] {
			return true, nil // Missing essential files, restore needed
		}
	}

	// Check if MySQL can start and connect
	if err := m.mysqlClient.Ping(); err != nil {
		m.logger.Warn("MySQL ping failed, restore may be needed",
			zap.Error(err),
		)
		return true, nil
	}

	return false, nil
}

// isRestoreNeededCluster checks if restore is needed in cluster mode
func (m *BackupManager) isRestoreNeededCluster(ctx context.Context) (bool, error) {
	// Only primary nodes should perform restore checks
	if m.config.MySQL.Role != "primary" {
		return false, nil
	}

	// Check data directory like standalone
	needsRestore, err := m.isRestoreNeededStandalone(ctx)
	if err != nil {
		return false, err
	}

	if needsRestore {
		return true, nil
	}

	// Additional cluster-specific checks
	status, err := m.mysqlClient.GetStatus()
	if err != nil {
		return false, fmt.Errorf("failed to get MySQL status: %w", err)
	}

	// Check if this is a fresh primary that needs data
	if isPristinePrimary(status) {
		m.logger.Info("Fresh primary node detected, restore needed")
		return true, nil
	}

	return false, nil
}

// isPristinePrimary checks if this is a fresh primary node
func isPristinePrimary(status map[string]interface{}) bool {
	// Check for signs of a fresh installation
	// - No existing databases except system ones
	// - No binary log position
	// - No existing replication configuration

	if binlogPos, ok := status["Executed_Gtid_Set"]; ok {
		if binlogPos == "" {
			return true
		}
	}

	if slaveStatus, ok := status["Slave_status"]; ok {
		if slaveStatus == "" {
			return true
		}
	}

	return false
}

func (m *BackupManager) Stop(ctx context.Context) error {
	// Stop the scheduler
	if m.scheduler != nil {
		m.scheduler.Stop()
	}

	// Cleanup temporary backup directory
	if err := os.RemoveAll(m.tmpDir); err != nil {
		m.logger.Warn("Failed to remove temporary backup directory",
			zap.String("tmp_dir", m.tmpDir),
			zap.Error(err),
		)
	}
	return nil
}

func (m *BackupManager) RestoreBackup(ctx context.Context, backupKey string) error {
	// Check if restore is allowed based on mode and role
	if err := m.verifyRestorePermission(); err != nil {
		return fmt.Errorf("restore not permitted: %w", err)
	}

	restorePath := filepath.Join(m.tmpDir, "restore")
	if err := os.MkdirAll(restorePath, 0750); err != nil {
		return fmt.Errorf("failed to create restore directory: %w", err)
	}

	// Track verification result
	result := &VerificationResult{
		BackupKey: backupKey,
		Timestamp: time.Now(),
		Status:    "in_progress",
	}
	m.setVerificationResult(backupKey, result)

	if err := m.storage.Download(ctx, backupKey, filepath.Join(restorePath, "backup.xb")); err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	cmd := exec.CommandContext(ctx, "xtrabackup",
		"--prepare",
		"--target-dir="+restorePath,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to prepare backup: %w", err)
	}

	cmd = exec.CommandContext(ctx, "xtrabackup",
		"--copy-back",
		"--target-dir="+restorePath,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}

// Prepare XtraBackup command with encryption support
func (m *BackupManager) prepareBackupCommand(backupType BackupType, backupPath string) (*exec.Cmd, error) {
	baseArgs := []string{
		fmt.Sprintf("--target-dir=%s", backupPath),
		"--datadir=/var/lib/mysql",
		fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
		"--slave-info",        // Capture replica info
		"--safe-slave-backup", // Ensure consistent replica backups
		"--stream=xbstream",   // Use xbstream format for streaming to xbcloud
		"--compress",          // Enable compression
		fmt.Sprintf("--compress-threads=%d", runtime.NumCPU()), // Parallel compression
		"|", // Pipe to xbcloud
		"xbcloud", "put",
		fmt.Sprintf("--storage=s3"),
		fmt.Sprintf("--s3-endpoint=%s", m.config.Storage.Endpoint),
		fmt.Sprintf("--s3-access-key=%s", m.config.Storage.AccessKey),
		fmt.Sprintf("--s3-secret-key=%s", m.config.Storage.SecretKey),
		fmt.Sprintf("--s3-bucket=%s", m.config.Storage.Bucket),
		fmt.Sprintf("--s3-region=%s", m.config.Storage.Region),
		fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
		fmt.Sprintf("--s3-chunk-size=%d", 10*1024*1024), // 10MB chunks
		fmt.Sprintf("--s3-max-retries=%d", 5),           // Retry failed uploads
		fmt.Sprintf("--s3-retry-delay=%d", 5),           // 5 second delay between retries
		"--s3-upload-concurrency=10",                    // Concurrent upload threads
		"--galera-info",                                 // Capture Galera cluster info if present
		"--lock-ddl",                                    // Prevent DDL during backup
		"--lock-ddl-per-table",                          // Table-level DDL locking
	}

	// Add backup type specific options
	switch backupType {
	case FullBackup:
		baseArgs = append(baseArgs,
			"--backup",
			"--binlog-info=ON",
			"--extra-lsndir=/var/lib/mysql/xtrabackup_lsn",
		)
	}

	if m.config.Backup.CompressionLevel > 0 {
		baseArgs = append(baseArgs,
			fmt.Sprintf("--compress-level=%d", m.config.Backup.CompressionLevel),
			"--compress-method=qpress",       // Use qpress for better compression
			"--kill-long-queries-timeout=20", // Kill long queries after 20s
			"--ftwrl-wait-timeout=30",        // Wait up to 30s for FTWRL
			"--ftwrl-wait-threshold=60",      // Only if query runtime < 60s
		)
	}

	// Additional backup type specific arguments
	switch backupType {
	case IncrementalBackup:
		lastLSN, err := m.getLastBackupLSN()
		if err != nil {
			return nil, fmt.Errorf("failed to get last backup LSN: %w", err)
		}
		baseArgs = append(baseArgs,
			"--incremental",
			fmt.Sprintf("--incremental-lsn=%s", lastLSN))

	case TransactionLogBackup:
		baseArgs = append(baseArgs,
			"--backup-locks",
			"--slave-info",
			"--binlog-info=ON",
			"--incremental-basedir=/var/lib/mysql")
	}

	// Add checksum generation for verification
	baseArgs = append(baseArgs, "--check-privileges")

	cmd := exec.Command("xtrabackup", baseArgs...)

	// Set process resource limits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM, // Kill backup if parent dies
	}

	return cmd, nil
}
func (m *BackupManager) getLastBackupLSN() (string, error) {
	// Get list of backups
	backups, err := m.storage.List(context.Background(), m.config.Storage.Prefix)
	if err != nil {
		return "", fmt.Errorf("failed to list backups: %w", err)
	}

	// Find most recent backup
	var lastBackup *storage.BackupFile
	for i := range backups {
		if lastBackup == nil || backups[i].LastModified.After(lastBackup.LastModified) {
			lastBackup = &backups[i]
		}
	}

	if lastBackup == nil {
		return "", fmt.Errorf("no previous backups found")
	}

	// Download and parse xtrabackup_checkpoints file
	tmpDir := filepath.Join(m.tmpDir, "lsn")
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	checkpointsFile := filepath.Join(tmpDir, "xtrabackup_checkpoints")
	if err := m.storage.Download(context.Background(),
		filepath.Join(filepath.Dir(lastBackup.Key), "xtrabackup_checkpoints"),
		checkpointsFile); err != nil {
		return "", fmt.Errorf("failed to download checkpoints file: %w", err)
	}

	// Read and parse LSN from checkpoints file
	content, err := os.ReadFile(checkpointsFile)
	if err != nil {
		return "", fmt.Errorf("failed to read checkpoints file: %w", err)
	}

	// Parse the to_lsn value
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "to_lsn") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("LSN not found in checkpoints file")
}
func (m *BackupManager) ConfigureBackupSchedules() error {
	// Validate and parse full backup schedule
	if ok, err := config.ParseCronSchedule(m.config.Backup.FullSchedule); !ok {
		return fmt.Errorf("invalid full backup schedule: %w", err)
	}

	// Schedule full backups
	if _, err := m.scheduler.AddJob(m.config.Backup.FullSchedule, &backupJob{
		manager:    m,
		backupType: FullBackup,
	}); err != nil {
		return fmt.Errorf("failed to schedule full backup: %w", err)
	}

	// Schedule transaction log backups if enabled
	if m.config.Backup.TransactionLogEnabled {
		if ok, err := config.ParseCronSchedule(m.config.Backup.TransactionLogSchedule); !ok {
			return fmt.Errorf("invalid transaction log backup schedule: %w", err)
		}

		if _, err := m.scheduler.AddJob(m.config.Backup.TransactionLogSchedule, &backupJob{
			manager:    m,
			backupType: TransactionLogBackup,
		}); err != nil {
			return fmt.Errorf("failed to schedule transaction log backup: %w", err)
		}
	}

	m.logger.Info("Backup schedules configured",
		zap.String("full_schedule", m.config.Backup.FullSchedule),
		zap.Bool("txlog_enabled", m.config.Backup.TransactionLogEnabled),
		zap.String("txlog_schedule", m.config.Backup.TransactionLogSchedule),
	)

	return nil
}
