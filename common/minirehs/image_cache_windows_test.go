//go:build windows

package minirehs

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestImageLockPendingDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.lock")
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if h != windows.InvalidHandle {
			_ = windows.CloseHandle(h)
		}
	}()
	if err = windows.DeleteFile(p); err != nil {
		t.Fatal(err)
	}
	type result struct {
		unlock func()
		err    error
	}
	ch := make(chan result, 1)
	go func() { unlock, e := lockImageCache(path, time.Second); ch <- result{unlock: unlock, err: e} }()
	// A traditional NTFS delete leaves the name pending until the old handle
	// closes. File systems providing immediate POSIX deletion may acquire early.
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatal("pending deletion treated as a permanent I/O failure", got.err)
		}
		got.unlock()
	case <-time.After(30 * time.Millisecond):
		_ = windows.CloseHandle(h)
		h = windows.InvalidHandle
		got := <-ch
		if got.err != nil {
			t.Fatal(got.err)
		}
		got.unlock()
	}
}
