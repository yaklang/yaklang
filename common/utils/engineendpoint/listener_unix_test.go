//go:build !windows

package engineendpoint

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRefusesFilesDirectoriesLinksAndStaleSockets(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "stale socket"} {
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
			case "stale socket":
				l, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
				if err != nil {
					t.Fatal(err)
				}
				l.SetUnlinkOnClose(false)
				l.Close()
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

func TestRefusesSymlinkParent(t *testing.T) {
	_, endpoint := testEndpoint(t)
	dir := filepath.Dir(endpoint)
	link := dir + "-link"
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(link) })
	if l, err := Listen("unix", filepath.Join(link, "sock")); err == nil {
		l.Close()
		t.Fatal("accepted symlink parent")
	}
}
