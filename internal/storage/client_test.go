package storage

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"go.lumeweb.com/mysql-manager/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewClientWithEmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{},
	}
	logger := zap.NewNop()

	client, err := NewClient(cfg, logger)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestXbcloudAvailable(t *testing.T) {
	if _, err := exec.LookPath("xbcloud"); err != nil {
		t.Skip("xbcloud not available, skipping test")
	}
}

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Endpoint:  "http://localhost:9000",
			Region:    "us-east-1",
			Bucket:    "test-bucket",
			AccessKey: "test",
			SecretKey: "test",
		},
	}
	logger := zap.NewNop()

	client, err := NewClient(cfg, logger)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}


// Integration tests - these require a real S3-compatible server
// Enable these tests by setting the TEST_INTEGRATION environment variable
func TestIntegration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration tests")
	}

	cfg := &config.Config{
		Storage: config.StorageConfig{
			Endpoint:  os.Getenv("TEST_S3_ENDPOINT"),
			Region:    os.Getenv("TEST_S3_REGION"),
			Bucket:    os.Getenv("TEST_S3_BUCKET"),
			AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("TEST_S3_SECRET_KEY"),
		},
	}
	logger := zap.NewNop()

	client, err := NewClient(cfg, logger)
	assert.NoError(t, err)

	ctx := context.Background()
	testFile := "test.txt"
	testContent := "test content"

	// Create test file
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(t, err)
	defer os.Remove(testFile)

	// Test upload using xbcloud
	cmd := exec.Command("xbcloud", "put",
		"--storage=s3",
		"--s3-endpoint=" + cfg.Storage.Endpoint,
		"--s3-access-key=" + cfg.Storage.AccessKey,
		"--s3-secret-key=" + cfg.Storage.SecretKey,
		"--s3-bucket=" + cfg.Storage.Bucket,
		"test/" + testFile,
	)
	cmd.Stdin, _ = os.Open(testFile)
	err = cmd.Run()
	assert.NoError(t, err)

	// Test list using xbcloud
	files, err := client.List(ctx, "test/")
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "test/"+testFile, files[0].Key)

	// Test download using xbcloud
	downloadFile := "test_download.txt"
	err = client.Download(ctx, "test/"+testFile, downloadFile)
	assert.NoError(t, err)
	defer os.Remove(downloadFile)

	content, err := os.ReadFile(downloadFile)
	assert.NoError(t, err)
	assert.Equal(t, testContent, string(content))

	// Test delete
	err = client.Delete(ctx, files[0])
	assert.NoError(t, err)

	// Verify deletion
	files, err = client.List(ctx, "test/")
	assert.NoError(t, err)
	assert.Len(t, files, 0)
}

func TestUploadWithInvalidFile(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Endpoint:  "http://localhost:9000",
			Region:    "us-east-1",
			Bucket:    "test-bucket",
			AccessKey: "test",
			SecretKey: "test",
		},
	}
	logger := zap.NewNop()

	client, err := NewClient(cfg, logger)
	assert.NoError(t, err)

	ctx := context.Background()
	err = client.Upload(ctx, "nonexistent-file.txt", "test/file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
}

func TestListWithInvalidPrefix(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Endpoint:  "http://localhost:9000",
			Region:    "us-east-1",
			Bucket:    "test-bucket",
			AccessKey: "test",
			SecretKey: "test",
		},
	}
	logger := zap.NewNop()

	client, err := NewClient(cfg, logger)
	assert.NoError(t, err)

	ctx := context.Background()
	files, err := client.List(ctx, "\000invalid-prefix")
	assert.Error(t, err)
	assert.Nil(t, files)
}
