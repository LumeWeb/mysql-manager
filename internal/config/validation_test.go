package config

import (
	"fmt"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid standalone config",
			config: Config{
				MySQL: MySQLConfig{
					Mode:             "standalone",
					Role:             "primary", 
					RootPassword:     "secret",
					Port:             3306,
					CheckInterval:    time.Second,
					MaxReplicationLag: time.Second,
				},
				API: APIConfig{
					Port:     8080,
					Username: "admin",
					Password: "secret",
				},
				Cluster: ClusterConfig{
					Name:     "test-cluster",
					ServerID: "1",
					Role:    "primary",
				},
				Backup: BackupConfig{
					CompressionLevel: 6,
					RetentionDays:   7,
					Copies:          5,
				},
				Monitoring: MonitoringConfig{
					MetricsPort: 9100,
				},
			},
			expectError: false,
		},
		{
			name: "Invalid MySQL mode",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "invalid",
					Role:         "primary",
					RootPassword: "secret",
					Port:         3306,
				},
			},
			expectError: true,
			errorMsg:    "invalid MySQL mode",
		},
		{
			name: "Invalid MySQL role",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "standalone",
					Role:         "invalid",
					RootPassword: "secret",
					Port:         3306,
				},
			},
			expectError: true,
			errorMsg:    "invalid MySQL role",
		},
		{
			name: "Missing root password",
			config: Config{
				MySQL: MySQLConfig{
					Mode: "standalone",
					Role: "primary",
					Port: 3306,
				},
			},
			expectError: true,
			errorMsg:    "MYSQL_ROOT_PASSWORD is required",
		},
		{
			name: "Invalid port number",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "standalone",
					Role:         "primary",
					RootPassword: "secret",
					Port:         70000,
				},
			},
			expectError: true,
			errorMsg:    "invalid MySQL port number",
		},
		{
			name: "Invalid backup compression level",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "standalone",
					Role:         "primary",
					RootPassword: "secret",
					Port:         3306,
				},
				Backup: BackupConfig{
					CompressionLevel: 10,
				},
			},
			expectError: true,
			errorMsg:    "backup compression level must be between 0 and 9",
		},
		{
			name: "Invalid retention days",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "standalone",
					Role:         "primary",
					RootPassword: "secret",
					Port:         3306,
				},
				Backup: BackupConfig{
					CompressionLevel: 6,
					RetentionDays:   0,
				},
			},
			expectError: true,
			errorMsg:    "backup retention days must be positive",
		},
		{
			name: "Missing storage config in cluster mode",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "cluster",
					Role:         "primary",
					RootPassword: "secret",
					Port:         3306,
				},
			},
			expectError: true,
			errorMsg:    "S3_ENDPOINT is required for cluster mode",
		},
		{
			name: "Missing cluster name in cluster mode",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "cluster",
					Role:         "primary",
					RootPassword: "secret",
					Port:         3306,
				},
				Storage: StorageConfig{
					Endpoint:  "http://s3",
					Bucket:    "test",
					AccessKey: "key",
					SecretKey: "secret",
				},
			},
			expectError: true,
			errorMsg:    "CLUSTER_NAME is required in cluster mode",
		},
		{
			name: "Missing join address for replica",
			config: Config{
				MySQL: MySQLConfig{
					Mode:         "cluster",
					Role:         "replica",
					RootPassword: "secret",
					Port:         3306,
				},
				Storage: StorageConfig{
					Endpoint:  "http://s3",
					Bucket:    "test",
					AccessKey: "key",
					SecretKey: "secret",
				},
				Cluster: ClusterConfig{
					Name: "test-cluster",
				},
			},
			expectError: true,
			errorMsg:    "CLUSTER_JOIN_ADDR is required for replica nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidMode(t *testing.T) {
	assert.True(t, isValidMode("standalone"))
	assert.True(t, isValidMode("cluster"))
	assert.False(t, isValidMode("invalid"))
	assert.False(t, isValidMode(""))
}

func TestIsValidRole(t *testing.T) {
	assert.True(t, isValidRole("primary"))
	assert.True(t, isValidRole("replica"))
	assert.False(t, isValidRole("invalid"))
	assert.False(t, isValidRole(""))
}

func TestParseCronSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"valid daily", "0 0 * * *", false},
		{"valid hourly", "0 * * * *", false},
		{"valid every 15 min", "*/15 * * * *", false},
		{"valid complex", "15,45 */2 * * 1-5", false},
		{"invalid format", "* * *", true},
		{"invalid minutes", "60 * * * *", true},
		{"invalid hours", "* 24 * * *", true},
		{"empty string", "", true},
		{"invalid characters", "* * * * abc", true},
		{"too many fields", "* * * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCronSchedule(tt.schedule)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name   string
		errors []error
		want   string
	}{
		{
			name: "multiple errors",
			errors: []error{
				fmt.Errorf("error 1"),
				fmt.Errorf("error 2"),
			},
			want: "configuration validation failed:\n  - error 1\n  - error 2",
		},
		{
			name:   "no errors",
			errors: []error{},
			want:   "configuration validation failed:",
		},
		{
			name: "single error",
			errors: []error{
				fmt.Errorf("single error"),
			},
			want: "configuration validation failed:\n  - single error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valErr := NewValidationError(tt.errors)
			assert.NotNil(t, valErr)
			assert.Equal(t, len(tt.errors), len(valErr.Errors))
			assert.Equal(t, tt.want, valErr.Error())
		})
	}
}
