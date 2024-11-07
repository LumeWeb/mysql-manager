package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadFullConfig(t *testing.T) {
	// Set environment variables
	os.Setenv("MYSQL_MODE", "cluster")
	os.Setenv("MYSQL_ROLE", "primary") 
	os.Setenv("MYSQL_ROOT_PASSWORD", "secret")
	os.Setenv("MYSQL_PORT", "3306")
	os.Setenv("CLUSTER_NAME", "test-cluster")
	os.Setenv("API_PASSWORD", "test-password")
	os.Setenv("CLUSTER_SERVER_ID", "1234")
	os.Setenv("CLUSTER_ROLE", "primary")
	os.Setenv("CLUSTER_JOIN_TIMEOUT", "5m")
	os.Setenv("CLUSTER_STATE_SYNC", "10s")
	os.Setenv("CLUSTER_FAILOVER_DELAY", "30s")
	os.Setenv("BACKUP_ENABLED", "true")
	os.Setenv("BACKUP_FULL_INTERVAL", "24h")
	os.Setenv("BACKUP_INCREMENTAL_INTERVAL", "1h")
	os.Setenv("BACKUP_RETENTION_DAYS", "30")
	os.Setenv("BACKUP_COPIES", "2")
	os.Setenv("BACKUP_COMPRESSION_LEVEL", "6")
	os.Setenv("BACKUP_MAX_BANDWIDTH", "0")
	os.Setenv("BACKUP_VALIDATE_INTERVAL", "6h")
	os.Setenv("BACKUP_RESTORE_TEST_INTERVAL", "24h")
	os.Setenv("STORAGE_ENDPOINT", "https://s3.amazonaws.com")
	os.Setenv("STORAGE_BUCKET", "test-bucket")
	os.Setenv("STORAGE_ACCESS_KEY", "test-key")
	os.Setenv("STORAGE_SECRET_KEY", "test-secret")
	os.Setenv("MONITORING_METRICS_PORT", "9100")

	// Load configuration
	cfg, err := LoadConfig()
	assert.NoError(t, err)

	// Validate configuration
	assert.Equal(t, "cluster", cfg.MySQL.Mode)
	assert.Equal(t, "primary", cfg.MySQL.Role)
	assert.Equal(t, "secret", cfg.MySQL.RootPassword)
	assert.Equal(t, 3306, cfg.MySQL.Port)
	assert.Equal(t, "test-cluster", cfg.Cluster.Name)
	assert.Equal(t, "1234", cfg.Cluster.ServerID)
	assert.Equal(t, "primary", cfg.Cluster.Role)
	assert.Equal(t, 5*time.Minute, cfg.Cluster.JoinTimeout)
	assert.Equal(t, 10*time.Second, cfg.Cluster.StateSync)
	assert.Equal(t, 30*time.Second, cfg.Cluster.FailoverDelay)
	assert.True(t, cfg.Backup.Enabled)
	assert.Equal(t, 30, cfg.Backup.RetentionDays)
	assert.Equal(t, "https://s3.amazonaws.com", cfg.Storage.Endpoint)
	assert.Equal(t, "test-bucket", cfg.Storage.Bucket)
	assert.Equal(t, "test-key", cfg.Storage.AccessKey)
	assert.Equal(t, "test-secret", cfg.Storage.SecretKey)
	assert.Equal(t, 9100, cfg.Monitoring.MetricsPort)

	// Unset environment variables
	os.Unsetenv("MYSQL_MODE")
	os.Unsetenv("MYSQL_ROLE")
	os.Unsetenv("MYSQL_ROOT_PASSWORD")
	os.Unsetenv("MYSQL_PORT")
	os.Unsetenv("CLUSTER_NAME")
	os.Unsetenv("CLUSTER_SERVER_ID")
	os.Unsetenv("CLUSTER_ROLE")
	os.Unsetenv("CLUSTER_JOIN_TIMEOUT")
	os.Unsetenv("CLUSTER_STATE_SYNC")
	os.Unsetenv("CLUSTER_FAILOVER_DELAY")
	os.Unsetenv("BACKUP_ENABLED")
	os.Unsetenv("BACKUP_RETENTION_DAYS")
	os.Unsetenv("BACKUP_SCHEDULE")
	os.Unsetenv("BACKUP_S3_ENDPOINT")
	os.Unsetenv("BACKUP_S3_BUCKET")
	os.Unsetenv("BACKUP_S3_ACCESS_KEY")
	os.Unsetenv("BACKUP_S3_SECRET_KEY")
	os.Unsetenv("MONITORING_METRICS_PORT")
}
