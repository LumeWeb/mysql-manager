package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/mysql-manager/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewCollector(t *testing.T) {
	// Create a unique test config for each test run
	cfg := &config.Config{
		Monitoring: config.MonitoringConfig{
			MetricsPort: 0, // Test mode
		},
	}
	logger := zap.NewNop()

	// Create a new registry for each test
	// Create a new registry for each test to avoid conflicts
	registry := prometheus.NewRegistry()
	collector, err := NewCollector(cfg, logger, registry)
	assert.NoError(t, err)
	assert.NotNil(t, collector)
}

func TestCollectorStartStop(t *testing.T) {
	tests := []struct {
		name       string
		metricsPort int
	}{
		{"First collector", 9101},
		{"Second collector", 9102},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Monitoring: config.MonitoringConfig{
					MetricsPort: tt.metricsPort,
				},
			}
			logger := zap.NewNop()

			registry := prometheus.NewRegistry()
			collector, err := NewCollector(cfg, logger, registry)
			assert.NoError(t, err)

			// Start collector
			err = collector.Start()
			assert.NoError(t, err)

			// Wait for server to start
			time.Sleep(100 * time.Millisecond)

			// Test metrics endpoint with retry
			var resp *http.Response
			var getErr error
			for i := 0; i < 5; i++ {
				resp, getErr = http.Get(fmt.Sprintf("http://localhost:%d/metrics", tt.metricsPort))
				if getErr == nil && resp != nil {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			if !assert.NotNil(t, resp, "HTTP response should not be nil") {
				t.FailNow() // Stop test if response is nil to avoid panic
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			resp.Body.Close()

			// Verify metrics output contains our metrics
			metricsText := string(body)
			assert.Contains(t, metricsText, "mysql_manager_uptime_seconds")
			assert.Contains(t, metricsText, "mysql_backup_total")
			assert.Contains(t, metricsText, "mysql_connections")

			// Stop collector
			ctx := context.Background()
			err = collector.Stop(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestMetricsUpdate(t *testing.T) {
	cfg := &config.Config{
		Monitoring: config.MonitoringConfig{
			MetricsPort: 9102,
		},
	}
	logger := zap.NewNop()

	registry := prometheus.NewRegistry()
	collector, err := NewCollector(cfg, logger, registry)
	assert.NoError(t, err)

	// Test updating various metrics
	collector.IncBackupTotal("full")
	collector.ObserveBackupDuration("full", 120.5)
	collector.SetBackupSize("full", 1024 * 1024 * 100) // 100MB
	collector.IncMySQLErrors("backup_upload_failed")
	collector.SetMySQLConnections("active", 42)
	collector.IncMySQLSlowQueries()
	collector.SetMySQLReplicationStatus("lag", 1.5)

	// Start collector
	err = collector.Start()
	assert.NoError(t, err)

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Get metrics
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", cfg.Monitoring.MetricsPort))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	resp.Body.Close()

	// Verify metrics were updated
	metricsText := string(body)
	assert.Contains(t, metricsText, `mysql_backup_total{type="full"} 1`)
	assert.Contains(t, metricsText, `mysql_backup_in_process 1`)
	assert.Contains(t, metricsText, `mysql_connections 42`)
	assert.Contains(t, metricsText, `mysql_slow_queries_total 1`)
	assert.Contains(t, metricsText, `mysql_replication_lag_seconds 1.5`)

	// Stop collector
	ctx := context.Background()
	err = collector.Stop(ctx)
	assert.NoError(t, err)
}
