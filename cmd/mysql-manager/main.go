package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/mysql-manager/internal/backup"
	"go.lumeweb.com/mysql-manager/internal/config"
	"go.lumeweb.com/mysql-manager/internal/metrics"
	"go.lumeweb.com/mysql-manager/internal/mysql"
	"go.lumeweb.com/mysql-manager/internal/storage"
	"go.uber.org/zap"
)

type Application struct {
	logger           *zap.Logger
	config           *config.Config
	mysqlClient      *mysql.Client
	storageClient    *storage.StorageClient
	backupManager    *backup.BackupManager
	metricsCollector *metrics.Collector
	startTime        time.Time
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Create application context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize application
	app, err := newApplication(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize application", zap.Error(err))
	}

	// Handle graceful shutdown
	go handleGracefulShutdown(app, cancel)

	// Start application components
	if err := app.start(ctx); err != nil {
		logger.Fatal("Failed to start application", zap.Error(err))
	}

	// Block until context is cancelled
	<-ctx.Done()
}

func newApplication(cfg *config.Config, logger *zap.Logger) (*Application, error) {
	// Initialize storage client
	storageClient, err := storage.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	// Initialize metrics collector
	registry := prometheus.NewRegistry()
	metricsCollector, err := metrics.NewCollector(cfg, logger, registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics collector: %w", err)
	}

	// Initialize MySQL client
	mysqlClient, err := mysql.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create MySQL client: %w", err)
	}

	// Initialize backup manager
	backupManager, err := backup.NewManager(cfg, logger, storageClient, mysqlClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup manager: %w", err)
	}

	app := &Application{
		logger:           logger,
		config:           cfg,
		mysqlClient:      mysqlClient,
		storageClient:    storageClient,
		backupManager:    backupManager,
		metricsCollector: metricsCollector,
		startTime:        time.Now(),
	}

	return app, nil
}

func (app *Application) start(ctx context.Context) error {
	// Start metrics collector
	if err := app.metricsCollector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}

	// Start backup scheduler if enabled
	if app.config.Backup.Enabled {
		if err := app.backupManager.ConfigureBackupSchedules(); err != nil {
			return fmt.Errorf("failed to configure backup schedules: %w", err)
		}
	}

	app.logger.Info("Application started successfully",
		zap.Time("start_time", app.startTime),
	)

	return nil
}

func handleGracefulShutdown(app *Application, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	<-sigChan
	app.logger.Info("Received shutdown signal, initiating graceful shutdown")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer shutdownCancel()

	// Create error channel to collect shutdown errors
	errChan := make(chan error, 2)

	// Stop components concurrently
	go func() { errChan <- app.metricsCollector.Stop(shutdownCtx) }()
	go func() { errChan <- app.backupManager.Stop(shutdownCtx) }()

	// Wait for all components or timeout
	for i := 0; i < 2; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				app.logger.Error("Component shutdown error", zap.Error(err))
			}
		case <-shutdownCtx.Done():
			app.logger.Error("Shutdown timeout reached")
			return
		}
	}

	cancel() // Cancel main context
	app.logger.Info("Graceful shutdown completed")
}
