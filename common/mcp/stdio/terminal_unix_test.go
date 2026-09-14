//go:build darwin || linux

package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestRunWithSharedTerminal(t *testing.T) {
	for _, name := range []string{"cancel-before-request", "request-eof", "request-cancel"} {
		t.Run(name, func(t *testing.T) {
			master, slave, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			defer slave.Close()
			// Shells commonly duplicate one terminal descriptor onto stdin/stdout.
			// Create Go's input wrapper while that shared description is blocking,
			// just as os.Stdin is created before the CLI prepares protocol output.
			fd := int(slave.Fd())
			if err := unix.SetNonblock(fd, false); err != nil {
				t.Fatal(err)
			}
			duplicate := func(label string) *os.File {
				t.Helper()
				copyFD, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				f := os.NewFile(uintptr(copyFD), label)
				t.Cleanup(func() { _ = f.Close() })
				return f
			}
			input, output := duplicate("terminal-input"), duplicate("terminal-output")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := helperCommand("normal")
			logs := newDiagnosticFile(t)
			done := make(chan error, 1)
			go func() { done <- Run(ctx, cmd, input, output, logs) }()
			finished := false
			t.Cleanup(func() {
				cancel()
				_ = master.Close()
				_ = slave.Close()
				if !finished {
					select {
					case <-done:
					case <-time.After(10 * time.Second):
						t.Error("terminal supervisor did not stop after closing the PTY")
					}
				}
			})

			select {
			case err := <-done:
				finished = true
				t.Fatalf("exited before terminal input arrived: %v", err)
			case <-time.After(150 * time.Millisecond):
			}

			if name != "cancel-before-request" {
				responses := make(chan error, 1)
				go func() {
					scanner := bufio.NewScanner(master)
					for scanner.Scan() {
						var message envelope
						if json.Unmarshal(scanner.Bytes(), &message) == nil && message.Method == "" && string(message.ID) == `"terminal"` {
							if len(message.Result) == 0 || len(message.Error) != 0 {
								responses <- fmt.Errorf("unexpected response: %s", scanner.Bytes())
							} else {
								responses <- nil
							}
							return
						}
					}
					responses <- fmt.Errorf("terminal closed before response: %v", scanner.Err())
				}()
				if _, err := fmt.Fprintln(master, `{"jsonrpc":"2.0","id":"terminal","method":"ping"}`); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-responses:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("no terminal response")
				}
			}

			if name == "request-eof" {
				if _, err := master.Write([]byte{4}); err != nil { // Ctrl+D on an empty canonical line.
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				finished = true
				if name == "request-eof" && err != nil {
					t.Fatalf("EOF failed: %v", err)
				}
				if name != "request-eof" && !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("terminal input prevented shutdown")
			}
			flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
			if err != nil || flags&unix.O_NONBLOCK != 0 {
				t.Fatalf("terminal blocking mode was not restored: flags=%x err=%v", flags, err)
			}
		})
	}
}
