//go:build !windows

package engineendpoint

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestRefusesFilesDirectoriesAndLinks(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			_, endpoint := testEndpoint(t)
			var sentinel string
			switch kind {
			case "file":
				sentinel = endpoint
			case "directory":
				if err := os.Mkdir(endpoint, 0700); err != nil {
					t.Fatal(err)
				}
				sentinel = filepath.Join(endpoint, "keep")
			case "symlink":
				sentinel = endpoint + ".target"
				if err := os.Symlink(sentinel, endpoint); err != nil {
					t.Fatal(err)
				}
			}
			if sentinel != "" {
				if err := os.WriteFile(sentinel, []byte("keep me"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(endpoint)
			if err != nil {
				t.Fatal(err)
			}
			if l, err := Listen("unix", endpoint); err == nil {
				l.Close()
				t.Fatal("overwrote existing endpoint")
			}
			after, err := os.Lstat(endpoint)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("endpoint changed: %v", err)
			}
			if sentinel != "" {
				data, err := os.ReadFile(sentinel)
				if err != nil || string(data) != "keep me" {
					t.Fatalf("sentinel changed: %v", err)
				}
			}
		})
	}
}

func TestPrivateModesAndReplacementPreservedOnClose(t *testing.T) {
	_, endpoint := testEndpoint(t)
	l, err := Listen("unix", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	info, err := os.Stat(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("insecure socket permissions: %s", info.Mode())
	}
	if err := os.Rename(endpoint, endpoint+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpoint, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	l.Close()
	data, err := os.ReadFile(endpoint)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("close removed replacement: %v", err)
	}
}

func TestExistingSharedDirectoryPermissionsArePreserved(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0777} {
		t.Run(mode.String(), func(t *testing.T) {
			_, endpoint := testEndpoint(t)
			dir := filepath.Dir(endpoint)
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			l, err := Listen("unix", endpoint)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			parent, err := os.Stat(dir)
			if err != nil || parent.Mode().Perm() != mode {
				t.Fatal("changed existing directory permissions")
			}
			socket, err := os.Stat(endpoint)
			if err != nil || socket.Mode().Perm() != 0600 {
				t.Fatal("socket was not restricted to 0600")
			}
		})
	}
}

func TestPrepareCreatesPrivateParentWithoutBinding(t *testing.T) {
	_, endpoint := testEndpoint(t)
	endpoint = filepath.Join(filepath.Dir(endpoint), "ipc", "sock")
	if err := PrepareListener("unix", endpoint); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Stat(filepath.Dir(endpoint))
	if err != nil || parent.Mode().Perm() != 0700 {
		t.Fatal("new parent was not restricted to 0700")
	}
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		t.Fatal("preflight bound the endpoint")
	}
}

func TestSymlinkParentAndAliasCannotTakeOver(t *testing.T) {
	_, endpoint := testEndpoint(t)
	dir := filepath.Dir(endpoint)
	link := dir + "-link"
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(link) })
	l, err := Listen("unix", filepath.Join(link, "engine.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if other, err := Listen("unix", endpoint); err == nil {
		other.Close()
		t.Fatal("real path took over an alias's live socket")
	}
	// Retargeting the alias must not make Close remove a replacement there.
	_, replacement := testEndpoint(t)
	if err := os.WriteFile(replacement, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(replacement), link); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		t.Fatal("original socket was not cleaned up after alias changed")
	}
	if data, err := os.ReadFile(replacement); err != nil || string(data) != "keep" {
		t.Fatal("close removed the retargeted alias's file")
	}
}

func abandonUnixSocket(t *testing.T, endpoint string) os.FileInfo {
	t.Helper()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	l.SetUnlinkOnClose(false)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Lstat(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestReclaimsStaleSocketAfterReadOnlyPreflight(t *testing.T) {
	_, endpoint := testEndpoint(t)
	abandonUnixSocket(t, endpoint) // Also covers leftovers from older engines without a lock.
	if err := PrepareListener("unix", endpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(endpoint); err != nil {
		t.Fatal("preflight deleted the stale endpoint")
	}
	l, err := Listen("unix", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		t.Fatal("recovered listener did not clean up")
	}
	lock, err := os.Stat(endpoint + ".lock")
	if err != nil || lock.Mode().Perm() != 0600 {
		t.Fatal("endpoint lock must remain private and reusable")
	}
}

func TestConcurrentStaleRecoveryHasExactlyOneListener(t *testing.T) {
	_, endpoint := testEndpoint(t)
	abandonUnixSocket(t, endpoint)
	const count = 12
	start := make(chan struct{})
	listeners := make(chan net.Listener, count)
	errorsFound := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			l, err := Listen("unix", endpoint)
			if err == nil {
				listeners <- l
			} else {
				errorsFound <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(listeners)
	close(errorsFound)
	for l := range listeners {
		defer l.Close()
	}
	if len(errorsFound) != count-1 {
		t.Fatalf("expected exactly one surviving listener, got %d failures", len(errorsFound))
	}
	for err := range errorsFound {
		if !errors.Is(err, syscall.EADDRINUSE) {
			t.Fatalf("unexpected concurrent startup failure: %v", err)
		}
	}
}

func TestRefusesForeignLiveSocketAndDatagramSocket(t *testing.T) {
	for _, network := range []string{"unix", "unixgram"} {
		t.Run(network, func(t *testing.T) {
			_, endpoint := testEndpoint(t)
			var closer interface{ Close() error }
			var err error
			if network == "unix" {
				closer, err = net.ListenUnix(network, &net.UnixAddr{Name: endpoint, Net: network})
			} else {
				closer, err = net.ListenUnixgram(network, &net.UnixAddr{Name: endpoint, Net: network})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Close()
			before, _ := os.Lstat(endpoint)
			if l, err := Listen("unix", endpoint); err == nil {
				l.Close()
				t.Fatal("reclaimed a live or different socket type")
			}
			after, err := os.Lstat(endpoint)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("changed an unrelated endpoint")
			}
		})
	}
}

func TestLockSymlinkIsPreserved(t *testing.T) {
	_, endpoint := testEndpoint(t)
	target := endpoint + ".target"
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, endpoint+".lock"); err != nil {
		t.Fatal(err)
	}
	if l, err := Listen("unix", endpoint); err == nil {
		l.Close()
		t.Fatal("accepted symlink lock")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatal("changed lock symlink target")
	}
}

func TestStaleSocketIsNotRemovedWhileStartupOwnsLock(t *testing.T) {
	_, endpoint := testEndpoint(t)
	before := abandonUnixSocket(t, endpoint)
	lock, err := lockUnixEndpoint(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := PrepareListener("unix", endpoint); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("preflight ignored an in-progress startup: %v", err)
	}
	if l, err := Listen("unix", endpoint); err == nil {
		l.Close()
		t.Fatal("reclaimed a socket held by another startup")
	}
	after, err := os.Lstat(endpoint)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("changed a socket while another startup held the lock")
	}
}

func TestUnverifiableSocketPermissionsArePreserved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses socket access permissions")
	}
	_, endpoint := testEndpoint(t)
	before := abandonUnixSocket(t, endpoint)
	if err := os.Chmod(endpoint, 0000); err != nil {
		t.Fatal(err)
	}
	if l, err := Listen("unix", endpoint); err == nil {
		l.Close()
		t.Fatal("treated permission denied as proof of a stale socket")
	}
	after, err := os.Lstat(endpoint)
	if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0000 {
		t.Fatal("changed a socket that could not be verified")
	}
}

func TestShortSymlinkAddressDoesNotInheritRealPathLength(t *testing.T) {
	_, endpoint := testEndpoint(t)
	dir := filepath.Dir(endpoint)
	realDir := filepath.Join(dir, strings.Repeat("d", 80))
	if err := os.Mkdir(realDir, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "s")
	if err := os.Symlink(realDir, alias); err != nil {
		t.Fatal(err)
	}
	address := filepath.Join(alias, strings.Repeat("x", 103-len(alias)-1))
	l, err := Listen("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	if _, err := os.Lstat(address); !os.IsNotExist(err) {
		t.Fatal("long real path was not cleaned up")
	}
}
