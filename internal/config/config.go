package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// BackupConfig contains backup settings

// BackupConfig contains essential backup settings
type BackupConfig struct {
	Enabled                  bool          `json:"enabled"`
	FullSchedule             string        `json:"full_schedule"`
	TransactionLogEnabled    bool          `json:"transaction_log_enabled"`
	TransactionLogSchedule   string        `json:"transaction_log_schedule"`
	RetentionDays            int           `json:"retention_days"`
	CompressionLevel         int           `json:"compression_level"`
	ArchiveDirectory         string        `json:"archive_directory"`
	RealTimeBackupEnabled    bool          `json:"realtime_backup_enabled"`
	RealTimeBackupInterval   time.Duration `json:"realtime_backup_interval"`
	RealTimeBackupRetention  time.Duration `json:"realtime_backup_retention"`
	RealTimeCompressionLevel int           `json:"realtime_compression_level"`
	BinlogRetentionHours     int           `json:"binlog_retention_hours"`
	Copies                   int           `json:"copies"`
}

// StorageConfig contains S3-compatible storage settings
type StorageConfig struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Prefix    string `json:"prefix"`
}

// APIConfig contains API settings
type APIConfig struct {
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// MonitoringConfig contains monitoring and metrics settings
type MonitoringConfig struct {
	MetricsPort int `json:"metrics_port"`
}

// ClusterConfig contains cluster-specific settings
type ClusterConfig struct {
	Name          string        `json:"name"`
	ServerID      string        `json:"server_id"`
	JoinAddress   string        `json:"join_address"`
	JoinTimeout   time.Duration `json:"join_timeout"`
	StateSync     time.Duration `json:"state_sync"`
	FailoverDelay time.Duration `json:"failover_delay"`
	Role          string        `json:"role"`
}

// MySQL operational modes
const (
	ModeStandalone = "standalone"
	ModeCluster    = "cluster"
	ModePrimary    = "primary"
	ModeReplica    = "replica"
)

// MySQLConfig contains MySQL-specific settings
type MySQLConfig struct {
	Mode         string        `json:"mode"`
	Role         string        `json:"role"`
	RootPassword string        `json:"root_password"`
	Port         int           `json:"port"`
	JoinAddress  string        `json:"join_address"`
	CheckInterval    time.Duration `json:"check_interval"`
	MaxReplicationLag time.Duration `json:"max_replication_lag"`
}

type Config struct {
	MySQL      MySQLConfig
	Backup     BackupConfig
	Storage    StorageConfig
	Monitoring MonitoringConfig
	API        APIConfig
	Cluster    ClusterConfig
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		MySQL: MySQLConfig{
			Mode:         getEnvWithDefault("MYSQL_MODE", ModeStandalone),
			Role:         getEnvWithDefault("MYSQL_ROLE", ModePrimary),
			RootPassword: os.Getenv("MYSQL_ROOT_PASSWORD"),
			Port:         getEnvAsIntWithDefault("MYSQL_PORT", 3306),
			JoinAddress:  os.Getenv("MYSQL_JOIN_ADDRESS"),
		},
		Backup: BackupConfig{
			Enabled:                  getEnvAsBoolWithDefault("BACKUP_ENABLED", true),
			FullSchedule:             getEnvWithDefault("BACKUP_FULL_SCHEDULE", "0 0 * * *"),
			TransactionLogEnabled:    getEnvAsBoolWithDefault("BACKUP_TRANSACTION_LOG_ENABLED", false),
			TransactionLogSchedule:   getEnvWithDefault("BACKUP_TRANSACTION_LOG_SCHEDULE", "*/15 * * * *"),
			RetentionDays:            getEnvAsIntWithDefault("BACKUP_RETENTION_DAYS", 7),
			CompressionLevel:         getEnvAsIntWithDefault("BACKUP_COMPRESSION_LEVEL", 6),
			ArchiveDirectory:         getEnvWithDefault("BACKUP_ARCHIVE_DIR", "/var/lib/mysql/backup"),
			RealTimeBackupEnabled:    getEnvAsBoolWithDefault("BACKUP_REALTIME_ENABLED", false),
			RealTimeBackupInterval:   getEnvAsDurationWithDefault("BACKUP_REALTIME_INTERVAL", 5*time.Minute),
			RealTimeBackupRetention:  getEnvAsDurationWithDefault("BACKUP_REALTIME_RETENTION", 24*time.Hour),
			RealTimeCompressionLevel: getEnvAsIntWithDefault("BACKUP_REALTIME_COMPRESSION", 6),
			BinlogRetentionHours:     getEnvAsIntWithDefault("BACKUP_BINLOG_RETENTION_HOURS", 24),
			Copies:                   getEnvAsIntWithDefault("BACKUP_COPIES", 2),
		},
		Storage: StorageConfig{
			Endpoint:  os.Getenv("STORAGE_ENDPOINT"),
			Region:    getEnvWithDefault("STORAGE_REGION", "us-east-1"),
			Bucket:    os.Getenv("STORAGE_BUCKET"),
			AccessKey: os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_SECRET_KEY"),
			Prefix:    getEnvWithDefault("STORAGE_PREFIX", "backups/"),
		},
		API: APIConfig{
			Port:     getEnvAsIntWithDefault("API_PORT", 8080),
			Username: getEnvWithDefault("API_USERNAME", "admin"),
			Password: os.Getenv("API_PASSWORD"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var errors []error

	// Validate MySQL settings
	if c.MySQL.RootPassword == "" {
		errors = append(errors, fmt.Errorf("MYSQL_ROOT_PASSWORD is required"))
	}
	if !isValidMode(c.MySQL.Mode) {
		errors = append(errors, fmt.Errorf("invalid MYSQL_MODE: %s", c.MySQL.Mode))
	}
	if !isValidRole(c.MySQL.Role) {
		errors = append(errors, fmt.Errorf("invalid MYSQL_ROLE: %s", c.MySQL.Role))
	}
	if c.MySQL.Mode == ModeCluster && c.MySQL.Role == ModeReplica && c.MySQL.JoinAddress == "" {
		errors = append(errors, fmt.Errorf("MYSQL_JOIN_ADDRESS required for replica in cluster mode"))
	}

	// Validate Storage settings for cluster mode
	if c.MySQL.Mode == ModeCluster {
		if c.Storage.Endpoint == "" || c.Storage.Bucket == "" ||
			c.Storage.AccessKey == "" || c.Storage.SecretKey == "" {
			errors = append(errors, fmt.Errorf("storage configuration required in cluster mode"))
		}
	}

	// Validate API settings
	if c.API.Password == "" {
		errors = append(errors, fmt.Errorf("API_PASSWORD is required"))
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errors)
	}
	return nil
}

func (c *Config) validateMySQLConfig() error {
	var errors []error

	// Validate MySQL mode
	if !isValidMode(c.MySQL.Mode) {
		errors = append(errors, fmt.Errorf("invalid MySQL mode: %s", c.MySQL.Mode))
	}

	// Validate MySQL role
	if !isValidRole(c.MySQL.Role) {
		errors = append(errors, fmt.Errorf("invalid MySQL role: %s", c.MySQL.Role))
	}

	// Root password is required
	if c.MySQL.RootPassword == "" {
		errors = append(errors, fmt.Errorf("MYSQL_ROOT_PASSWORD is required"))
	}

	// Validate port number
	if c.MySQL.Port <= 0 || c.MySQL.Port > 65535 {
		errors = append(errors, fmt.Errorf("invalid MySQL port number: %d", c.MySQL.Port))
	}

	return combineErrors(errors)
}

func (c *Config) validateBackupConfig() error {
	var errors []error

	// Validate compression level
	if c.Backup.CompressionLevel < 0 || c.Backup.CompressionLevel > 9 {
		errors = append(errors, fmt.Errorf("backup compression level must be between 0 and 9"))
	}

	// Validate retention days
	if c.Backup.RetentionDays <= 0 {
		errors = append(errors, fmt.Errorf("backup retention days must be positive"))
	}

	// Validate backup schedule if enabled
	if c.Backup.Enabled {
		if ok, err := ParseCronSchedule(c.Backup.FullSchedule); !ok {
			errors = append(errors, fmt.Errorf("invalid full backup schedule: %v", err))
		}
	}

	return combineErrors(errors)
}

func (c *Config) validateStorageConfig() error {
	var errors []error

	// In cluster mode, storage configuration is required
	if c.MySQL.Mode == ModeCluster {
		// Validate S3 endpoint
		if c.Storage.Endpoint == "" {
			errors = append(errors, fmt.Errorf("S3_ENDPOINT is required for cluster mode"))
		}

		// Validate endpoint URL
		if c.Storage.Endpoint != "" {
			if _, err := url.Parse(c.Storage.Endpoint); err != nil {
				errors = append(errors, fmt.Errorf("invalid S3 endpoint URL: %v", err))
			}
		}

		// Validate bucket name
		if c.Storage.Bucket == "" {
			errors = append(errors, fmt.Errorf("S3_BUCKET is required"))
		}

		// Validate access and secret keys
		if c.Storage.AccessKey == "" {
			errors = append(errors, fmt.Errorf("S3_ACCESS_KEY is required"))
		}
		if c.Storage.SecretKey == "" {
			errors = append(errors, fmt.Errorf("S3_SECRET_KEY is required"))
		}
	}

	return combineErrors(errors)
}

func (c *Config) validateMonitoringConfig() error {
	var errors []error

	// Validate metrics port
	if c.Monitoring.MetricsPort <= 0 || c.Monitoring.MetricsPort > 65535 {
		errors = append(errors, fmt.Errorf("invalid metrics port number: %d", c.Monitoring.MetricsPort))
	}

	return combineErrors(errors)
}

func (c *Config) validateAPIConfig() error {
	var errors []error

	// Validate API port
	if c.API.Port <= 0 || c.API.Port > 65535 {
		errors = append(errors, fmt.Errorf("invalid API port number: %d", c.API.Port))
	}

	// Validate username and password
	if c.API.Username == "" {
		errors = append(errors, fmt.Errorf("API username is required"))
	}
	if c.API.Password == "" {
		errors = append(errors, fmt.Errorf("API password is required"))
	}

	return combineErrors(errors)
}

// Helper function to combine multiple errors
func combineErrors(errors []error) error {
	if len(errors) == 0 {
		return nil
	}
	return NewValidationError(errors)
}

// ValidationError represents multiple configuration validation errors
type ValidationError struct {
	Errors []error
}

// NewValidationError creates a new ValidationError
func NewValidationError(errors []error) *ValidationError {
	return &ValidationError{Errors: errors}
}

// Error implements the error interface
func (ve *ValidationError) Error() string {
	var errorMsgs []string
	errorMsgs = append(errorMsgs, "configuration validation failed:")
	for _, err := range ve.Errors {
		errorMsgs = append(errorMsgs, "  - "+err.Error())
	}
	return strings.Join(errorMsgs, "\n")
}

// Validate mode and role
func isValidMode(mode string) bool {
	return mode == ModeStandalone || mode == ModeCluster
}

func isValidRole(role string) bool {
	return role == ModePrimary || role == ModeReplica
}
