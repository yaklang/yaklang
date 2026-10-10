package pcapdb

import (
	"context"
	"errors"
	"os"
	"time"
)

func acquireFileLock(ctx context.Context, path string, wait bool) (*os.File, error) {
	return acquireLock(ctx, path, wait, tryFileLock)
}
func acquireReadFileLock(ctx context.Context, path string, wait bool) (*os.File, error) {
	return acquireLock(ctx, path, wait, tryReadFileLock)
}
func acquireLock(ctx context.Context, path string, wait bool, tryLock func(*os.File) error) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		if err = tryLock(f); err == nil {
			return f, nil
		}
		if !errors.Is(err, ErrBusy) || !wait {
			f.Close()
			return nil, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Never unlink a lock file: another process may already be waiting on its inode.
func releaseFileLock(f *os.File) {
	if f != nil {
		f.Close()
	}
}
