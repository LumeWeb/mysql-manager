package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.lumeweb.com/mysql-manager/internal/config"
)

type Collector struct {
	config     *config.Config
	logger     *zap.Logger
	server     *http.Server
	registry   *prometheus.Registry
	metrics    struct {
		// System Metrics
		systemCPUUsage     prometheus.Gauge
		systemMemoryUsage  prometheus.Gauge
		systemDiskUsage    *prometheus.GaugeVec
		systemLoadAverage  prometheus.Gauge
		systemNetworkStats *prometheus.CounterVec

		// MySQL Metrics
		mysqlConnections       *prometheus.GaugeVec
		mysqlThreads           *prometheus.GaugeVec
		mysqlBufferPool        *prometheus.GaugeVec
		mysqlTableCache        prometheus.Gauge
		mysqlOpenTables        prometheus.Gauge
		mysqlQueryCache        *prometheus.GaugeVec
		mysqlReplicationStatus *prometheus.GaugeVec
		mysqlSlowQueries       prometheus.Counter
		mysqlErrors            *prometheus.CounterVec

		// Backup Metrics
		backupTotal        *prometheus.CounterVec
		backupDuration     *prometheus.HistogramVec
		backupSize         *prometheus.GaugeVec
		backupVerification *prometheus.CounterVec

		// Process Metrics
		processUptime        prometheus.Gauge
		processRestarts      prometheus.Counter
		processMemoryUsage   prometheus.Gauge
		processThreadCount   prometheus.Gauge
		processNetworkHealth prometheus.Gauge

		// Performance Metrics
		queryLatency     *prometheus.HistogramVec
		networkLatency   *prometheus.HistogramVec
		storageIOLatency *prometheus.HistogramVec
	}
	mu         sync.RWMutex
	startTime  time.Time
}

func NewCollector(cfg *config.Config, logger *zap.Logger, registry *prometheus.Registry) (*Collector, error) {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	collector := &Collector{
		config:    cfg,
		logger:    logger,
		registry:  registry,
		startTime: time.Now(),
	}

	// Initialize metrics
	collector.initializeMetrics()

	// Create a new registry and register collectors
	collector.registry = prometheus.NewRegistry()
	
	// Only register process collectors if not in test mode
	if cfg.Monitoring.MetricsPort != 0 {
		collector.registry.MustRegister(prometheus.NewGoCollector())
		collector.registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	}

	// Start background metrics collection
	go collector.collectMetrics()

	// Register all metrics with the registry
	for _, metric := range []prometheus.Collector{
		collector.metrics.mysqlConnections,
		collector.metrics.backupTotal,
		collector.metrics.mysqlSlowQueries,
		collector.metrics.mysqlReplicationStatus,
		collector.metrics.processUptime,
		collector.metrics.systemCPUUsage,
		collector.metrics.systemMemoryUsage,
		collector.metrics.mysqlThreads,
		collector.metrics.mysqlBufferPool,
		collector.metrics.mysqlTableCache,
		collector.metrics.mysqlOpenTables,
		collector.metrics.mysqlQueryCache,
		collector.metrics.mysqlErrors,
		collector.metrics.backupDuration,
		collector.metrics.backupSize,
		collector.metrics.backupVerification,
		collector.metrics.processRestarts,
		collector.metrics.processMemoryUsage,
		collector.metrics.processThreadCount,
		collector.metrics.processNetworkHealth,
		collector.metrics.queryLatency,
		collector.metrics.networkLatency,
		collector.metrics.storageIOLatency,
	} {
		collector.registry.MustRegister(metric)
	}

	return collector, nil
}

// collectMetrics updates metrics values periodically
func (c *Collector) collectMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Update uptime
			c.metrics.processUptime.Set(time.Since(c.startTime).Seconds())

			// Set some initial values for test metrics
			if c.config.Monitoring.MetricsPort != 0 {
				c.metrics.mysqlConnections.WithLabelValues("active").Set(0)
				c.metrics.backupTotal.WithLabelValues("full").Add(0)
				c.metrics.mysqlSlowQueries.Add(0)
				c.metrics.mysqlReplicationStatus.WithLabelValues("lag").Set(0)
			}
		}
	}
}

// Start begins collecting metrics and starts the HTTP server
func (c *Collector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{}))

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.config.Monitoring.MetricsPort),
		Handler: mux,
	}

	// Start HTTP server in a goroutine
	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("Metrics server failed", zap.Error(err))
		}
	}()

	// Wait a moment for server to start
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (c *Collector) Stop(ctx context.Context) error {
	if c.server != nil {
		// Gracefully shutdown the HTTP server
		if err := c.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown metrics server: %w", err)
		}
	}

	// Reset metrics state
	c.mu.Lock()
	defer c.mu.Unlock()

	// Unregister all metrics from registry
	if c.registry != nil {
		c.registry = prometheus.NewRegistry()
	}

	c.logger.Info("Metrics collector stopped")
	return nil
}

func (c *Collector) initializeMetrics() {
	// System Metrics
	c.metrics.systemCPUUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_cpu_usage_percent",
		Help: "Current system CPU usage percentage",
	})
	if c.config.Monitoring.MetricsPort != 0 {
		c.registry.MustRegister(c.metrics.systemCPUUsage)
	}

	c.metrics.systemMemoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_memory_usage_bytes",
		Help: "Current system memory usage in bytes",
	})
	if c.config.Monitoring.MetricsPort != 0 {
		c.registry.MustRegister(c.metrics.systemMemoryUsage)
	}

	c.metrics.systemDiskUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_disk_usage_bytes",
			Help: "Disk usage by mount point",
		},
		[]string{"mount_point"},
	)
	c.registry.MustRegister(c.metrics.systemDiskUsage)

	c.metrics.systemLoadAverage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_load_average",
		Help: "System load average",
	})
	c.registry.MustRegister(c.metrics.systemLoadAverage)

	c.metrics.systemNetworkStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_network_bytes_total",
			Help: "Total network bytes transmitted/received",
		},
		[]string{"interface", "direction"},
	)
	c.registry.MustRegister(c.metrics.systemNetworkStats)

	// MySQL Metrics
	c.metrics.mysqlConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_connections",
			Help: "MySQL connection statistics",
		},
		[]string{"state"},
	)
	c.registry.MustRegister(c.metrics.mysqlConnections)

	c.metrics.mysqlThreads = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_threads",
			Help: "MySQL thread statistics",
		},
		[]string{"state"},
	)
	c.registry.MustRegister(c.metrics.mysqlThreads)

	c.metrics.mysqlBufferPool = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_buffer_pool_status",
			Help: "MySQL InnoDB buffer pool status",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.mysqlBufferPool)

	c.metrics.mysqlTableCache = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mysql_table_cache_size",
		Help: "MySQL table cache size",
	})
	c.registry.MustRegister(c.metrics.mysqlTableCache)

	c.metrics.mysqlOpenTables = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mysql_open_tables_count",
		Help: "Number of open tables",
	})
	c.registry.MustRegister(c.metrics.mysqlOpenTables)

	c.metrics.mysqlQueryCache = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_query_cache",
			Help: "MySQL query cache statistics",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.mysqlQueryCache)

	c.metrics.mysqlReplicationStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_replication_status",
			Help: "MySQL replication status",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.mysqlReplicationStatus)

	c.metrics.mysqlSlowQueries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mysql_slow_queries_total",
		Help: "Total number of slow queries",
	})
	c.registry.MustRegister(c.metrics.mysqlSlowQueries)

	c.metrics.mysqlErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql_errors_total",
			Help: "Total number of MySQL errors",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.mysqlErrors)

	// Backup Metrics
	c.metrics.backupTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql_backup_total",
			Help: "Total number of backups",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.backupTotal)

	c.metrics.backupDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mysql_backup_duration_seconds",
			Help:    "Backup operation duration",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.backupDuration)

	c.metrics.backupSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_backup_size_bytes",
			Help: "Size of backup files",
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.backupSize)

	c.metrics.backupVerification = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql_backup_verification_total",
			Help: "Total number of backup verifications",
		},
		[]string{"status"},
	)
	c.registry.MustRegister(c.metrics.backupVerification)

	// Process Metrics
	c.metrics.processUptime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "process_uptime_seconds",
		Help: "Process uptime in seconds",
	})
	c.registry.MustRegister(c.metrics.processUptime)

	c.metrics.processRestarts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "process_restarts_total",
		Help: "Total number of process restarts",
	})
	c.registry.MustRegister(c.metrics.processRestarts)

	c.metrics.processMemoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "process_memory_usage_bytes",
		Help: "Process memory usage in bytes",
	})
	c.registry.MustRegister(c.metrics.processMemoryUsage)

	c.metrics.processThreadCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "process_thread_count",
		Help: "Number of threads in the process",
	})
	c.registry.MustRegister(c.metrics.processThreadCount)

	c.metrics.processNetworkHealth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "process_network_health",
		Help: "Process network health status",
	})
	c.registry.MustRegister(c.metrics.processNetworkHealth)

	// Performance Metrics
	c.metrics.queryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mysql_query_latency_seconds",
			Help:    "MySQL query latency distribution",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		[]string{"type"},
	)
	c.registry.MustRegister(c.metrics.queryLatency)

	c.metrics.networkLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "network_latency_seconds",
			Help:    "Network latency measurements",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		[]string{"destination"},
	)
	c.registry.MustRegister(c.metrics.networkLatency)

	c.metrics.storageIOLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "storage_io_latency_seconds",
			Help:    "Storage I/O latency measurements",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		[]string{"operation"},
	)
	c.registry.MustRegister(c.metrics.storageIOLatency)
}

// Metric update methods for each metric type
func (c *Collector) SetSystemCPUUsage(usage float64) {
	c.metrics.systemCPUUsage.Set(usage)
}

func (c *Collector) SetSystemMemoryUsage(bytes float64) {
	c.metrics.systemMemoryUsage.Set(bytes)
}

func (c *Collector) SetSystemDiskUsage(mountPoint string, bytes float64) {
	c.metrics.systemDiskUsage.WithLabelValues(mountPoint).Set(bytes)
}

func (c *Collector) SetSystemLoadAverage(load float64) {
	c.metrics.systemLoadAverage.Set(load)
}

func (c *Collector) IncSystemNetworkBytes(iface, direction string, bytes float64) {
	c.metrics.systemNetworkStats.WithLabelValues(iface, direction).Add(bytes)
}

func (c *Collector) SetMySQLConnections(state string, count float64) {
	c.metrics.mysqlConnections.WithLabelValues(state).Set(count)
}

func (c *Collector) SetMySQLThreads(state string, count float64) {
	c.metrics.mysqlThreads.WithLabelValues(state).Set(count)
}

func (c *Collector) SetMySQLBufferPoolStatus(poolType string, value float64) {
	c.metrics.mysqlBufferPool.WithLabelValues(poolType).Set(value)
}

func (c *Collector) SetMySQLTableCacheSize(size float64) {
	c.metrics.mysqlTableCache.Set(size)
}

func (c *Collector) SetMySQLOpenTables(count float64) {
	c.metrics.mysqlOpenTables.Set(count)
}

func (c *Collector) SetMySQLQueryCache(cacheType string, value float64) {
	c.metrics.mysqlQueryCache.WithLabelValues(cacheType).Set(value)
}

func (c *Collector) SetMySQLReplicationStatus(statusType string, value float64) {
	c.metrics.mysqlReplicationStatus.WithLabelValues(statusType).Set(value)
}

func (c *Collector) IncMySQLSlowQueries() {
	c.metrics.mysqlSlowQueries.Inc()
}

func (c *Collector) IncMySQLErrors(errorType string) {
	c.metrics.mysqlErrors.WithLabelValues(errorType).Inc()
}

func (c *Collector) IncBackupTotal(backupType string) {
	c.metrics.backupTotal.WithLabelValues(backupType).Inc()
}

func (c *Collector) ObserveBackupDuration(backupType string, duration float64) {
	c.metrics.backupDuration.WithLabelValues(backupType).Observe(duration)
}

func (c *Collector) SetBackupSize(backupType string, bytes float64) {
	c.metrics.backupSize.WithLabelValues(backupType).Set(bytes)
}

func (c *Collector) IncBackupVerification(status string) {
	c.metrics.backupVerification.WithLabelValues(status).Inc()
}

func (c *Collector) SetProcessUptime(uptime float64) {
	c.metrics.processUptime.Set(uptime)
}

func (c *Collector) IncProcessRestarts() {
	c.metrics.processRestarts.Inc()
}

func (c *Collector) SetProcessMemoryUsage(bytes float64) {
	c.metrics.processMemoryUsage.Set(bytes)
}

func (c *Collector) SetProcessThreadCount(count float64) {
	c.metrics.processThreadCount.Set(count)
}

func (c *Collector) SetProcessNetworkHealth(healthy bool) {
	var healthValue float64
	if healthy {
		healthValue = 1
	}
	c.metrics.processNetworkHealth.Set(healthValue)
}

func (c *Collector) ObserveQueryLatency(queryType string, latency float64) {
	c.metrics.queryLatency.WithLabelValues(queryType).Observe(latency)
}

func (c *Collector) ObserveNetworkLatency(destination string, latency float64) {
	c.metrics.networkLatency.WithLabelValues(destination).Observe(latency)
}

func (c *Collector) ObserveStorageIOLatency(operation string, latency float64) {
	c.metrics.storageIOLatency.WithLabelValues(operation).Observe(latency)
}

// GetMetrics returns current metrics data
func (c *Collector) GetMetrics() (map[string]interface{}, error) {
	metrics := make(map[string]interface{})
	
	// Add basic metrics
	metrics["uptime"] = time.Since(c.startTime).Seconds()
	
	return metrics, nil
}
