package pty

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const ptyHelperEnv = "YAK_PTY_TEST_HELPER"

// TestCommandRoundTrip starts the test binary as a real child process attached
// to the PTY. It covers the command, resize, and bidirectional I/O used by the
// Yaklang terminal and the Memfit TTY tests.
func TestCommandRoundTrip(t *testing.T) {
	if os.Getenv(ptyHelperEnv) == "1" {
		fmt.Fprintln(os.Stdout, "PTY_READY")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "PTY_REPLY:%s", line)
		return
	}

	terminal, err := New()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := terminal.Resize(80, 24); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := terminal.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandRoundTrip$", "-test.count=1")
	cmd.Env = append(os.Environ(), ptyHelperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	var mu sync.Mutex
	var output bytes.Buffer
	updated := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := terminal.Read(buf)
			if n > 0 {
				mu.Lock()
				output.Write(buf[:n])
				mu.Unlock()
				select {
				case updated <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	waitFor := func(want string) {
		t.Helper()
		for {
			mu.Lock()
			got := output.String()
			mu.Unlock()
			if strings.Contains(got, want) {
				return
			}
			select {
			case <-updated:
			case <-ctx.Done():
				t.Fatalf("waiting for %q in PTY output %q: %v", want, got, ctx.Err())
			}
		}
	}

	waitFor("PTY_READY")
	if err := terminal.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("hello from yak\n")); err != nil {
		t.Fatal(err)
	}
	waitFor("PTY_REPLY:hello from yak")
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandContextCancellation(t *testing.T) {
	if os.Getenv(ptyHelperEnv) == "cancel" {
		time.Sleep(30 * time.Second)
		return
	}
	terminal, err := New()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := terminal.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandContextCancellation$", "-test.count=1")
	cmd.Env = append(os.Environ(), ptyHelperEnv+"=cancel")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cancel()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled command exited successfully")
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("cancelled command did not exit")
	}
}
