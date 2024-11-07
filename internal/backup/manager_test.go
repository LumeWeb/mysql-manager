package backup

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "go.uber.org/zap"
    "go.lumeweb.com/mysql-manager/internal/config"
    "go.lumeweb.com/mysql-manager/internal/storage"
)

func TestCreateBackupWithStorageError(t *testing.T) {
	cfg := &config.Config{
		MySQL: config.MySQLConfig{
			Mode:         "standalone",
			Role:         "primary",
			RootPassword: "test",
		},
		Backup: config.BackupConfig{
			CompressionLevel: 6,
			Copies:          5,
		},
		Storage: config.StorageConfig{
			Prefix: "test-backups/",
		},
	}
	logger := zap.NewNop()
	mockStorage := &MockStorage{}

	manager, err := NewManager(cfg, logger, mockStorage, nil) // nil mysqlClient for test
	assert.NoError(t, err)

	// Simulate storage upload failure
	mockStorage.On("Upload", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(fmt.Errorf("upload failed"))
	mockStorage.On("List", mock.Anything, mock.AnythingOfType("string")).Return([]storage.BackupFile{}, nil)

	// Create a dummy MySQL backup file for testing
	tmpBackupDir := filepath.Join(manager.tmpDir, time.Now().UTC().Format("2006-01-02T15-04-05Z"))
	err = os.MkdirAll(tmpBackupDir, 0750)
	assert.NoError(t, err)
	
	backupFile := filepath.Join(tmpBackupDir, "backup.sql.gz")
	err = os.WriteFile(backupFile, []byte("dummy backup content"), 0644)
	assert.NoError(t, err)

	// Test backup creation with storage upload failure
	ctx := context.Background()
	err = manager.CreateBackup(ctx)
	assert.Error(t, err)
	if err.Error() == "backup command execution failed: exec: \"xtrabackup\": executable file not found in $PATH" {
		t.Skip("xtrabackup not installed, skipping test")
	}
	assert.Contains(t, err.Error(), "storage upload failed")

	mockStorage.AssertExpectations(t)

	// Cleanup
	os.RemoveAll(manager.tmpDir)
}
