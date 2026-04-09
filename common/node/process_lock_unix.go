//go:build unix

package node

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type nodeInstanceLock struct {
	path string
	file *os.File
}

func acquireNodeInstanceLock(nodeID string) (*nodeInstanceLock, error) {
	lockDir := filepath.Join(os.TempDir(), "legion-scannode-locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create node lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, sanitizeNodeIDForFilename(nodeID)+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open node lock file: %w", err)
	}

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		owner := readLockOwner(lockFile)
		_ = lockFile.Close()
		return nil, fmt.Errorf(
			"node_id=%s is already running in another process (lock_file=%s owner=%s)",
			nodeID,
			lockPath,
			owner,
		)
	}

	if err := writeLockOwner(lockFile, nodeID); err != nil {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("write node lock owner: %w", err)
	}

	return &nodeInstanceLock{
		path: lockPath,
		file: lockFile,
	}, nil
}

func (l *nodeInstanceLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	var releaseErr error
	if err := l.file.Truncate(0); err != nil {
		releaseErr = fmt.Errorf("truncate node lock file: %w", err)
	}
	if _, err := l.file.Seek(0, 0); err != nil && releaseErr == nil {
		releaseErr = fmt.Errorf("seek node lock file: %w", err)
	}
	if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil && releaseErr == nil {
		releaseErr = fmt.Errorf("unlock node lock file: %w", err)
	}
	if err := l.file.Close(); err != nil && releaseErr == nil {
		releaseErr = fmt.Errorf("close node lock file: %w", err)
	}

	l.file = nil
	return releaseErr
}

func writeLockOwner(lockFile *os.File, nodeID string) error {
	if err := lockFile.Truncate(0); err != nil {
		return err
	}
	if _, err := lockFile.Seek(0, 0); err != nil {
		return err
	}

	payload := fmt.Sprintf(
		"pid=%d node_id=%s started_at=%s\n",
		os.Getpid(),
		nodeID,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if _, err := io.WriteString(lockFile, payload); err != nil {
		return err
	}
	return lockFile.Sync()
}

func readLockOwner(lockFile *os.File) string {
	if _, err := lockFile.Seek(0, 0); err != nil {
		return "unknown"
	}

	raw, err := io.ReadAll(io.LimitReader(lockFile, 256))
	if err != nil {
		return "unknown"
	}

	owner := strings.TrimSpace(string(raw))
	if owner == "" {
		return "unknown"
	}
	return owner
}

func sanitizeNodeIDForFilename(nodeID string) string {
	trimmed := strings.TrimSpace(nodeID)
	if trimmed == "" {
		return "unnamed-node"
	}

	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "unnamed-node"
	}
	return sanitized
}
