package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

func buildBackupMetadata(path string, started time.Time) (*BackupMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup metadata: %w", err)
	}

	checksum, err := fileChecksum(path)
	if err != nil {
		return nil, err
	}

	return &BackupMetadata{
		BackupSize:  info.Size(),
		Checksum:    checksum,
		Location:    path,
		StartedAt:   started,
		CompletedAt: time.Now(),
	}, nil
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
