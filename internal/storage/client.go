package storage

import (
    "context"
    "errors"
    "fmt"
    "math"
    "os"
    "net/url"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "time"
    "math/rand"
    "strconv"
    "github.com/prometheus/client_golang/prometheus"
    "go.lumeweb.com/mysql-manager/internal/config"
    "go.uber.org/zap"
)

// parseSize parses a size string into int64
func parseSize(sizeStr string) (int64, error) {
    return strconv.ParseInt(sizeStr, 10, 64)
}

type StorageClient struct {
    config   *config.Config
    logger   *zap.Logger
    metrics  struct {
        uploadSpeed     prometheus.Gauge
        downloadSpeed   prometheus.Gauge
        uploadErrors    prometheus.Counter
        retries        prometheus.Counter
        cleanups       prometheus.Counter
        timeouts       prometheus.Counter
        bytesUploaded  prometheus.Counter
        bytesDownloaded prometheus.Counter
        uploadLatency   prometheus.Histogram
        downloadLatency prometheus.Histogram
        retryDelays    prometheus.Histogram
    }
}

func NewClient(cfg *config.Config, logger *zap.Logger) (*StorageClient, error) {
    if err := validateStorageConfig(cfg); err != nil {
        return nil, fmt.Errorf("invalid storage configuration: %w", err)
    }

    client := &StorageClient{
        config:   cfg,
        logger:   logger,
    }

    // Initialize metrics
    client.metrics.uploadSpeed = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "mysql_s3_upload_speed_bytes_per_second",
        Help: "Current S3 upload speed in bytes per second",
    })
    client.metrics.downloadSpeed = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "mysql_s3_download_speed_bytes_per_second", 
        Help: "Current S3 download speed in bytes per second",
    })
    client.metrics.uploadErrors = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "mysql_s3_upload_errors_total",
        Help: "Total number of S3 upload errors",
    })
    client.metrics.retries = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "mysql_s3_operation_retries_total",
        Help: "Total number of S3 operation retries",
    })
    client.metrics.cleanups = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "mysql_s3_cleanup_operations_total",
        Help: "Total number of temporary file cleanup operations",
    })

    return client, nil
}

type BackupFile struct {
    Key          string
    LastModified time.Time
    Size         int64
}

func (c *StorageClient) List(ctx context.Context, prefix string) ([]BackupFile, error) {
    // Validate prefix
    if strings.ContainsAny(prefix, "\x00") {
        return nil, fmt.Errorf("invalid prefix containing null bytes")
    }

    cmd := exec.CommandContext(ctx, "xbcloud", "list",
        fmt.Sprintf("--storage=s3"),
        fmt.Sprintf("--s3-endpoint=%s", c.config.Storage.Endpoint),
        fmt.Sprintf("--s3-region=%s", c.config.Storage.Region),
        fmt.Sprintf("--s3-access-key=%s", c.config.Storage.AccessKey),
        fmt.Sprintf("--s3-secret-key=%s", c.config.Storage.SecretKey),
        fmt.Sprintf("--s3-bucket=%s", c.config.Storage.Bucket),
        filepath.Join(c.config.Storage.Prefix, prefix),
    )

    output, err := cmd.CombinedOutput()
    if err != nil {
        // Enhanced error output capture
        errMsg := string(output)
        if len(errMsg) > 1000 {
            errMsg = errMsg[:1000] + "..." // Truncate very long error messages
        }
        return nil, fmt.Errorf("failed to list objects: %v: %s", err, errMsg)
    }

    // Cleanup temporary files
    defer func() {
        if tmpFiles, err := filepath.Glob(filepath.Join(os.TempDir(), "xbcloud*")); err == nil {
            for _, f := range tmpFiles {
                os.Remove(f)
            }
        }
    }()

    // Cleanup temporary files
    defer func() {
        if tmpFiles, err := filepath.Glob(filepath.Join(os.TempDir(), "xbcloud*")); err == nil {
            for _, f := range tmpFiles {
                os.Remove(f)
            }
        }
    }()

    var files []BackupFile
    for _, line := range strings.Split(string(output), "\n") {
        if line == "" {
            continue
        }
        
        parts := strings.Fields(line)
        if len(parts) < 2 {
            continue
        }

        size, err := parseSize(parts[1])
        if err != nil {
            return nil, fmt.Errorf("failed to parse size from line %q: %w", line, err)
        }

        files = append(files, BackupFile{
            Key:  parts[0],
            Size: size,
        })
    }

    return files, nil
}

// validateStorageConfig performs comprehensive S3 config validation
func validateStorageConfig(cfg *config.Config) error {
    if cfg.Storage.Endpoint == "" {
        return fmt.Errorf("S3 endpoint is required")
    }
    
    // Validate S3 endpoint URL format and scheme
    endpoint, err := url.Parse(cfg.Storage.Endpoint)
    if err != nil {
        return fmt.Errorf("invalid S3 endpoint URL: %w", err)
    }
    if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
        return fmt.Errorf("S3 endpoint must use http or https scheme")
    }

    // Validate bucket name according to S3 rules
    if len(cfg.Storage.Bucket) < 3 || len(cfg.Storage.Bucket) > 63 {
        return fmt.Errorf("S3 bucket name must be between 3 and 63 characters")
    }
    if strings.Contains(cfg.Storage.Bucket, "..") {
        return fmt.Errorf("S3 bucket name cannot contain consecutive periods")
    }
    if strings.HasPrefix(cfg.Storage.Bucket, "xn--") {
        return fmt.Errorf("S3 bucket name cannot start with 'xn--'")
    }
    if !isValidBucketName(cfg.Storage.Bucket) {
        return fmt.Errorf("invalid S3 bucket name format")
    }
    if cfg.Storage.AccessKey == "" {
        return fmt.Errorf("S3 access key is required") 
    }
    if cfg.Storage.SecretKey == "" {
        return fmt.Errorf("S3 secret key is required")
    }
    if cfg.Storage.Bucket == "" {
        return fmt.Errorf("S3 bucket is required")
    }
    if cfg.Storage.Region == "" {
        return fmt.Errorf("S3 region is required")
    }
    return nil
}

// isValidBucketName checks if a bucket name meets S3 naming rules
func isValidBucketName(name string) bool {
    // S3 bucket naming rules: https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html
    
    if len(name) < 3 || len(name) > 63 {
        return false
    }

    // Must start and end with lowercase letter or number
    if !isAlphanumeric(rune(name[0])) || !isAlphanumeric(rune(name[len(name)-1])) {
        return false
    }

    // Cannot be formatted as IP address
    if isIPAddress(name) {
        return false
    }

    // Cannot have consecutive periods
    if strings.Contains(name, "..") {
        return false
    }

    // Cannot start with 'xn--' prefix
    if strings.HasPrefix(name, "xn--") {
        return false
    }

    // Cannot end with '-s3alias'
    if strings.HasSuffix(name, "-s3alias") {
        return false
    }

    // Can only contain lowercase letters, numbers, dots, and hyphens
    for _, r := range name {
        if !isAlphanumeric(r) && r != '.' && r != '-' {
            return false
        }
    }
    
    return true
}

func isIPAddress(s string) bool {
    // Check if string is formatted like an IP address
    parts := strings.Split(s, ".")
    if len(parts) != 4 {
        return false
    }
    
    for _, part := range parts {
        if num, err := strconv.Atoi(part); err != nil || num < 0 || num > 255 {
            return false
        }
    }
    
    return true
}

func isAlphanumeric(r rune) bool {
    return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// withRetry executes an operation with exponential backoff retry
func (c *StorageClient) withRetry(op func() error) error {
    var err error
    maxRetries := 5
    baseBackoff := 1 * time.Second
    maxBackoff := 30 * time.Second
    minJitter := 100 * time.Millisecond
    maxJitter := 1 * time.Second
    tempFiles := make([]string, 0)
    startTime := time.Now()
    netTimeouts := 0
    maxNetTimeouts := 3 // Maximum consecutive network timeouts

    // Register cleanup handler
    defer func() {
        if len(tempFiles) > 0 {
            c.metrics.cleanups.Inc()
            for _, f := range tempFiles {
                if err := os.Remove(f); err != nil {
                    c.logger.Warn("Failed to cleanup temp file",
                        zap.String("file", f),
                        zap.Error(err))
                }
            }
        }
    }()

    for i := 0; i < maxRetries; i++ {
        if err = op(); err == nil {
            return nil
        }

        // Don't retry if context cancelled or deadline exceeded
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return err
        }

        // Track retry and timing metrics
        c.metrics.retries.Inc()
        c.metrics.uploadErrors.Inc()
    
        // Calculate and record operation duration
        duration := time.Since(startTime).Seconds()
        if strings.Contains(err.Error(), "timeout") {
            c.metrics.timeouts.Inc()
            c.logger.Warn("Operation timeout",
                zap.Float64("duration_seconds", duration),
                zap.Error(err),
            )
        }

        // Check for network-related errors that warrant retry
        if isRetryableError(err) {
            // Track consecutive network timeouts
            if isNetworkTimeout(err) {
                netTimeouts++
                if netTimeouts >= maxNetTimeouts {
                    c.logger.Error("Maximum consecutive network timeouts reached",
                        zap.Int("timeouts", netTimeouts),
                        zap.Error(err),
                    )
                    break
                }
            } else {
                netTimeouts = 0 // Reset counter for non-timeout errors
            }

            if i < maxRetries-1 {
                // Calculate exponential backoff with controlled jitter
                backoff := time.Duration(float64(baseBackoff) * math.Pow(2, float64(i)))
                if backoff > maxBackoff {
                    backoff = maxBackoff
                }
                
                // Add jitter between min and max values
                jitter := time.Duration(rand.Float64() * float64(maxJitter-minJitter)) + minJitter
                sleepTime := backoff + jitter

                // Increase backoff for network timeouts
                if isNetworkTimeout(err) {
                    sleepTime *= 2
                }

                c.metrics.retryDelays.Observe(float64(sleepTime.Seconds()))
                
                c.logger.Info("Retrying operation",
                    zap.Int("attempt", i+1),
                    zap.Duration("backoff", sleepTime),
                    zap.Error(err),
                )
                
                time.Sleep(sleepTime)
                continue
            }
        }

        // Non-retryable error
        break
    }
    return fmt.Errorf("operation failed after %d retries: %w", maxRetries, err)
}

func isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    
    errStr := err.Error()
    return strings.Contains(errStr, "connection reset") ||
           strings.Contains(errStr, "connection refused") ||
           strings.Contains(errStr, "timeout") ||
           strings.Contains(errStr, "temporary failure") ||
           strings.Contains(errStr, "no such host") ||
           strings.Contains(errStr, "i/o timeout") ||
           strings.Contains(errStr, "TLS handshake timeout") ||
           strings.Contains(errStr, "operation timed out")
}

func isNetworkTimeout(err error) bool {
    if err == nil {
        return false
    }

    errStr := err.Error()
    return strings.Contains(errStr, "timeout") ||
           strings.Contains(errStr, "i/o timeout") ||
           strings.Contains(errStr, "TLS handshake timeout") ||
           strings.Contains(errStr, "operation timed out")
}

func (c *StorageClient) Upload(ctx context.Context, localPath, objectKey string) error {
    startTime := time.Now()
    fileInfo, err := os.Stat(localPath)
    if err != nil {
        return fmt.Errorf("failed to stat file: %w", err)
    }

    // Parse file size
    uploadSize := fileInfo.Size()
    
    defer func() {
        duration := time.Since(startTime).Seconds()
        if duration > 0 {
            speedBytesPerSec := float64(uploadSize) / duration
            c.metrics.uploadSpeed.Set(speedBytesPerSec)
            c.metrics.bytesUploaded.Add(float64(uploadSize))
            c.metrics.uploadLatency.Observe(duration)
            
            c.logger.Info("Upload completed",
                zap.String("object_key", objectKey),
                zap.Int64("size_bytes", uploadSize),
                zap.Float64("speed_mbps", speedBytesPerSec/1024/1024),
                zap.Float64("duration_seconds", duration),
            )
        }
    }()


    // Prepare command
    var cmd *exec.Cmd
    err = c.withRetry(func() error {
        cmd = exec.CommandContext(ctx, "xbcloud", "put",
        fmt.Sprintf("--storage=s3"),
        fmt.Sprintf("--s3-endpoint=%s", c.config.Storage.Endpoint),
        fmt.Sprintf("--s3-access-key=%s", c.config.Storage.AccessKey),
        fmt.Sprintf("--s3-secret-key=%s", c.config.Storage.SecretKey),
        fmt.Sprintf("--s3-bucket=%s", c.config.Storage.Bucket),
        fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
        objectKey,
    )

    cmd.Stdin = os.Stdin // Will read from pipe or file
    cmd.Stderr = os.Stderr

        if err := cmd.Run(); err != nil {
            // Attempt to clean up partial upload
            cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
        
            if cleanupErr := c.cleanupPartialUpload(cleanupCtx, objectKey); cleanupErr != nil {
                c.logger.Error("Failed to cleanup partial upload",
                    zap.String("object_key", objectKey),
                    zap.Error(cleanupErr),
                )
            }
        
            if exitErr, ok := err.(*exec.ExitError); ok {
                return fmt.Errorf("xbcloud upload failed with exit code %d: %w", exitErr.ExitCode(), err)
            }
            return fmt.Errorf("xbcloud upload failed: %w", err)
        }
        return nil
    })

    if err != nil {
        return fmt.Errorf("upload failed for %s: %w", objectKey, err)
    }

    // Record metrics
    metric := prometheus.NewCounter(prometheus.CounterOpts{
        Name: "mysql_s3_upload_total",
        Help: "Total number of S3 uploads",
    })
    metric.Inc()

    return nil
}

func (c *StorageClient) Download(ctx context.Context, objectKey string, localPath string) error {
    startTime := time.Now()
    
    // Track download metrics
    defer func() {
        duration := time.Since(startTime).Seconds()
        if duration > 0 {
            if fileInfo, err := os.Stat(localPath); err == nil {
                speedBytesPerSec := float64(fileInfo.Size()) / duration
                c.metrics.downloadSpeed.Set(speedBytesPerSec)
                c.metrics.bytesDownloaded.Add(float64(fileInfo.Size()))
                c.metrics.downloadLatency.Observe(duration)
                
                c.logger.Info("Download completed",
                    zap.String("object_key", objectKey),
                    zap.Int64("size_bytes", fileInfo.Size()),
                    zap.Float64("speed_mbps", speedBytesPerSec/1024/1024),
                    zap.Float64("duration_seconds", duration),
                )
            }
        }
    }()

    cmd := exec.CommandContext(ctx, "xbcloud", "get",
        fmt.Sprintf("--storage=s3"),
        fmt.Sprintf("--s3-endpoint=%s", c.config.Storage.Endpoint),
        fmt.Sprintf("--s3-access-key=%s", c.config.Storage.AccessKey),
        fmt.Sprintf("--s3-secret-key=%s", c.config.Storage.SecretKey),
        fmt.Sprintf("--s3-bucket=%s", c.config.Storage.Bucket),
        fmt.Sprintf("--s3-region=%s", c.config.Storage.Region),
        fmt.Sprintf("--parallel=%d", runtime.NumCPU()),
        fmt.Sprintf("--s3-max-retries=%d", 5),
        objectKey,
    )

    // Create output directory if it doesn't exist
    if err := os.MkdirAll(filepath.Dir(localPath), 0750); err != nil {
        return fmt.Errorf("failed to create output directory: %w", err)
    }

    outFile, err := os.Create(localPath)
    if err != nil {
        return fmt.Errorf("failed to create output file: %w", err)
    }
    defer outFile.Close()

    cmd.Stdout = outFile
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("xbcloud download failed: %w", err)
    }

    return nil
}

func (c *StorageClient) Delete(ctx context.Context, file BackupFile) error {
    cmd := exec.CommandContext(ctx, "xbcloud", "delete",
        fmt.Sprintf("--storage=s3"),
        fmt.Sprintf("--s3-endpoint=%s", c.config.Storage.Endpoint),
        fmt.Sprintf("--s3-access-key=%s", c.config.Storage.AccessKey),
        fmt.Sprintf("--s3-secret-key=%s", c.config.Storage.SecretKey),
        fmt.Sprintf("--s3-bucket=%s", c.config.Storage.Bucket),
        fmt.Sprintf("--s3-region=%s", c.config.Storage.Region),
        file.Key,
    )

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to delete object %s: %w", file.Key, err)
    }
    return nil
}
func (c *StorageClient) cleanupPartialUpload(ctx context.Context, objectKey string) error {
    startTime := time.Now()
    
    // Create cleanup context with timeout
    cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    cmd := exec.CommandContext(cleanupCtx, "xbcloud", "delete",
        fmt.Sprintf("--storage=s3"),
        fmt.Sprintf("--s3-endpoint=%s", c.config.Storage.Endpoint),
        fmt.Sprintf("--s3-access-key=%s", c.config.Storage.AccessKey),
        fmt.Sprintf("--s3-secret-key=%s", c.config.Storage.SecretKey),
        fmt.Sprintf("--s3-bucket=%s", c.config.Storage.Bucket),
        fmt.Sprintf("--s3-region=%s", c.config.Storage.Region),
        "--delete-all-parts",  // Delete all multipart upload parts
        objectKey + "*",       // Delete all objects with this prefix
    )

    // Capture command output for logging
    output, err := cmd.CombinedOutput()

    if err != nil {
        c.logger.Error("Failed to cleanup partial upload",
            zap.String("object_key", objectKey),
            zap.String("output", string(output)),
            zap.Error(err),
        )
        return fmt.Errorf("failed to cleanup partial upload: %w", err)
    }

    duration := time.Since(startTime)
    c.metrics.cleanups.Inc()
    
    c.logger.Info("Partial upload cleanup completed",
        zap.String("object_key", objectKey),
        zap.Duration("duration", duration),
    )
    
    return nil
}
