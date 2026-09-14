//go:build !windows

package engineendpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type ownedListener struct {
	net.Listener
	path string
	file os.FileInfo
	lock *os.File
	once sync.Once
	err  error
}

func (l *ownedListener) Close() error {
	l.once.Do(func() {
		defer l.lock.Close()
		l.err = l.Listener.Close()
		// Never unlink a replacement path, even if this instance's socket was moved.
		if current, err := os.Lstat(l.path); err == nil && current.Mode()&os.ModeSocket != 0 && os.SameFile(l.file, current) {
			if err := os.Remove(l.path); l.err == nil {
				l.err = err
			}
		}
	})
	return l.err
}

// PrepareListener checks the endpoint before expensive server/database startup.
// It creates only a missing immediate parent; existing directories are never chmodded.
// Stale sockets are only removed by Listen, under the same endpoint lock used
// during preflight. A successful preflight does not reserve the endpoint.
func PrepareListener(transport, endpoint string) error {
	path, parent, err := prepareUnixParent(transport, endpoint)
	if err != nil {
		return err
	}
	lock, err := lockUnixEndpoint(path)
	if err != nil {
		return err
	}
	defer lock.Close()
	_, err = staleUnixSocket(endpoint, path, parent)
	return err
}

func prepareUnixParent(transport, endpoint string) (string, os.FileInfo, error) {
	if err := Validate(transport, endpoint); err != nil {
		return "", nil, err
	}
	if transport != "unix" {
		return "", nil, fmt.Errorf("IPC listener requires unix transport")
	}
	parent := filepath.Dir(endpoint)
	if err := os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return "", nil, err
	}
	// macOS /tmp and /var are system symlinks. Resolve the parent, never the
	// socket leaf. Keep the caller's short address for bind/dial (sockaddr_un
	// has a byte limit), and the real path for identity checks and cleanup.
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.Stat(realParent)
	if err != nil {
		return "", nil, err
	}
	// Existing project directories may be shared (0755/0777). The CLI requires
	// authentication, and the socket itself is restricted to 0600 below. Do not
	// change permissions on an existing directory or require users to move it.
	if !dir.IsDir() {
		return "", nil, fmt.Errorf("%w: %q", ErrInvalidDirectory, parent)
	}
	return filepath.Join(realParent, filepath.Base(endpoint)), dir, nil
}

// The lock is held until the listener closes and is released by the OS even
// after SIGKILL. Never unlink this file: removing a lock file allows concurrent
// starters to lock different inodes and accidentally reclaim each other's socket.
// It contains no credentials and is also shared by aliases of the same parent.
func lockUnixEndpoint(path string) (*os.File, error) {
	name := path + ".lock"
	fd, err := syscall.Open(name, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, &os.PathError{Op: "lock", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	fail := func(err error) (*os.File, error) {
		file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return fail(fmt.Errorf("unsafe endpoint lock: %w", os.ErrPermission))
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = syscall.EADDRINUSE
		}
		return fail(&os.PathError{Op: "lock", Path: name, Err: err})
	}
	current, err := os.Lstat(name)
	if err != nil || !os.SameFile(info, current) {
		return fail(fmt.Errorf("endpoint lock changed: %w", syscall.EADDRINUSE))
	}
	return file, nil
}

func sameUnixParent(endpoint string, parent os.FileInfo) bool {
	current, err := os.Stat(filepath.Dir(endpoint))
	return err == nil && os.SameFile(parent, current)
}

// A failed connection alone is not proof of a stale socket. Only ECONNREFUSED
// on an unchanged socket owned by this effective user permits reclamation.
// Timeouts, access errors, other socket types and replacement paths fail closed.
func staleUnixSocket(endpoint, path string, parent os.FileInfo) (os.FileInfo, error) {
	if !sameUnixParent(endpoint, parent) {
		return nil, fmt.Errorf("endpoint parent changed: %w", syscall.EADDRINUSE)
	}
	file, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stat, ok := file.Sys().(*syscall.Stat_t)
	if file.Mode()&os.ModeSocket == 0 || !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("endpoint already exists: %w", syscall.EADDRINUSE)
	}
	conn, err := net.DialTimeout("unix", endpoint, 250*time.Millisecond)
	if err == nil {
		conn.Close()
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return nil, fmt.Errorf("endpoint is live or cannot be verified as stale: %w", syscall.EADDRINUSE)
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(file, current) || !sameUnixParent(endpoint, parent) {
		return nil, fmt.Errorf("endpoint changed during stale check: %w", syscall.EADDRINUSE)
	}
	return file, nil
}

func Listen(transport, endpoint string) (net.Listener, error) {
	path, parent, err := prepareUnixParent(transport, endpoint)
	if err != nil {
		return nil, err
	}
	lock, err := lockUnixEndpoint(path)
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			lock.Close()
		}
	}()
	stale, err := staleUnixSocket(endpoint, path, parent)
	if err != nil {
		return nil, err
	}
	if stale != nil {
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(stale, current) || !sameUnixParent(endpoint, parent) {
			return nil, fmt.Errorf("stale endpoint was replaced: %w", syscall.EADDRINUSE)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	if !sameUnixParent(endpoint, parent) {
		listener.Close()
		return nil, fmt.Errorf("endpoint parent changed during bind: %w", syscall.EADDRINUSE)
	}
	file, err := os.Lstat(path)
	if err != nil {
		listener.Close()
		return nil, err
	}
	owned := &ownedListener{Listener: listener, path: path, file: file, lock: lock}
	keepLock = true
	if err := os.Chmod(path, 0600); err != nil {
		owned.Close()
		return nil, err
	}
	return owned, nil
}

func DialContext(ctx context.Context, transport, endpoint string) (net.Conn, error) {
	if err := Validate(transport, endpoint); err != nil {
		return nil, err
	}
	if transport != "unix" {
		return nil, fmt.Errorf("IPC dial requires unix transport")
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
