//go:build !windows

package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCLIStartupUnixSignalRecovery(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, os.Kill} {
		t.Run(signal.String(), func(t *testing.T) {
			f := newEngineCLI(t)
			_, endpoint := integrationEndpoint(t)
			if err := os.Chmod(filepath.Dir(endpoint), 0777); err != nil {
				t.Fatal(err)
			}
			secret, err := newEngineSecret()
			if err != nil {
				t.Fatal(err)
			}
			start := func() *engineCLIChild {
				child := startEngineCLI(t, f, "grpc", "--transport", "unix", "--socket-path", endpoint, "--local-password", secret)
				requireEngineEcho(t, "unix", endpoint, secret)
				return child
			}
			first := start()
			stopErr := first.stop(t, signal)
			_, socketErr := os.Lstat(endpoint)
			if signal == os.Kill {
				if stopErr == nil || socketErr != nil {
					t.Fatal("SIGKILL experiment did not leave a socket to recover")
				}
			} else if stopErr != nil || !os.IsNotExist(socketErr) {
				t.Fatalf("normal signal did not clean up: %v / %v", stopErr, socketErr)
			}
			// No test-side unlink: restart directly with the SAME path and DBs.
			second := start()
			if err := second.stop(t, syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			check := f.check(t, "--transport", "unix", "--socket-path", endpoint)
			if check["ok"] != true || check["addr"] != endpoint || check["transport"] != "unix" {
				t.Fatal("endpoint could not be reused after recovery and normal exit")
			}
			if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
				t.Fatal("self-check left its temporary socket behind")
			}
		})
	}
}

func TestCLIStartupUnixStalePreflightAndLiveCollision(t *testing.T) {
	f := newEngineCLI(t)
	_, endpoint := integrationEndpoint(t)
	// Simulate a crashed older version, which has no endpoint lock file.
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	l.SetUnlinkOnClose(false)
	l.Close()
	check := f.check(t, "--transport", "unix", "--socket-path", endpoint)
	if check["ok"] != true {
		t.Fatal("self-check failed to reclaim an older engine's abandoned socket")
	}
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		t.Fatal("self-check failed to release its recovered endpoint")
	}
	secret, err := newEngineSecret()
	if err != nil {
		t.Fatal(err)
	}
	first := startEngineCLI(t, f, "grpc", "--transport", "unix", "--socket-path", endpoint, "--secret", secret)
	before, err := os.Lstat(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"check-secret-local-grpc", "grpc"} {
		other := newEngineCLI(t)
		args := []string{command, "--transport", "unix", "--socket-path", endpoint}
		if command == "grpc" {
			args = append(args, "--secret", secret)
		}
		output, err := other.run(t, args...)
		if err == nil || !bytes.Contains(output, []byte(ipcBindInUse)) {
			t.Fatal("second startup did not report the live endpoint as in use")
		}
		for _, db := range []string{"project.db", "profile.db", "ssa.db"} {
			if _, err := os.Stat(filepath.Join(other.home, db)); !os.IsNotExist(err) {
				t.Fatal("occupied IPC startup initialized a database")
			}
		}
		after, err := os.Lstat(endpoint)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("second startup removed or replaced the active socket")
		}
		requireEngineEcho(t, "unix", endpoint, secret)
	}
	if err := first.stop(t, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
}

func TestCLIStartupUnixTmpAndSymlinkParent(t *testing.T) {
	for _, kind := range []string{"system tmp", "custom alias"} {
		t.Run(kind, func(t *testing.T) {
			f := newEngineCLI(t)
			secret, err := newEngineSecret()
			if err != nil {
				t.Fatal(err)
			}
			endpoint := filepath.Join("/tmp", "yakcli-"+secret[:16]+".sock")
			if kind == "custom alias" {
				_, base := integrationEndpoint(t)
				dir := filepath.Dir(base)
				realDir := filepath.Join(dir, "真实目录 & spaces")
				if err := os.Mkdir(realDir, 0755); err != nil {
					t.Fatal(err)
				}
				alias := filepath.Join(dir, "alias")
				if err := os.Symlink(realDir, alias); err != nil {
					t.Fatal(err)
				}
				endpoint = filepath.Join(alias, "引擎 socket.sock")
			}
			t.Cleanup(func() {
				os.Remove(endpoint)
				os.Remove(endpoint + ".lock") // Only after our child has exited.
			})
			check := f.check(t, "--transport", "unix", "--socket-path", endpoint)
			if check["ok"] != true || check["addr"] != endpoint {
				t.Fatal("self-check rejected a valid symlinked parent")
			}
			child := startEngineCLI(t, f, "grpc", "--transport", "unix", "--socket-path", endpoint, "--local-password", secret)
			if child.ready.Address != endpoint || child.ready.Transport != "unix" {
				t.Fatal("ready event changed the caller's Unix address")
			}
			requireEngineEcho(t, "unix", endpoint, secret)
			clientCheck := f.check(t, "--transport", "unix", "--socket-path", endpoint, "--client-password", secret)
			if clientCheck["ok"] != true || clientCheck["addr"] != endpoint {
				t.Fatal("client-mode self-check could not connect through the alias")
			}
			if err := child.stop(t, syscall.SIGHUP); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
				t.Fatal("aliased socket was not cleaned up")
			}
		})
	}
}
