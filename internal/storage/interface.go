package storage

import (
    "context"
)

// Client defines the interface for storage operations
type Client interface {
    Upload(ctx context.Context, localPath, objectKey string) error
    Download(ctx context.Context, objectKey, localPath string) error
    List(ctx context.Context, prefix string) ([]BackupFile, error)
    Delete(ctx context.Context, file BackupFile) error
}
