package backup

import (
    "context"
    "github.com/stretchr/testify/mock"
    "go.lumeweb.com/mysql-manager/internal/storage"
)

var _ storage.Client = (*MockStorage)(nil) // Ensure MockStorage implements storage.Client interface

type MockStorage struct {
    mock.Mock
}

// Ensure MockStorage implements storage.Client interface

func (m *MockStorage) Upload(ctx context.Context, localPath, objectKey string) error {
    args := m.Called(ctx, localPath, objectKey)
    return args.Error(0)
}

func (m *MockStorage) Download(ctx context.Context, objectKey, localPath string) error {
    args := m.Called(ctx, objectKey, localPath)
    return args.Error(0)
}

func (m *MockStorage) List(ctx context.Context, prefix string) ([]storage.BackupFile, error) {
    args := m.Called(ctx, prefix)
    return args.Get(0).([]storage.BackupFile), args.Error(1)
}

func (m *MockStorage) Delete(ctx context.Context, file storage.BackupFile) error {
    args := m.Called(ctx, file)
    return args.Error(0)
}
