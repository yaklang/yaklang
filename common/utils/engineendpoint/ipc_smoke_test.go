package engineendpoint

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This fast suite imports only the endpoint layer, not the CLI/server or any
// database. Platform-specific cases reuse the existing listener regressions.
func TestIPCSmoke(t *testing.T) {
	t.Run("round-trip-and-reuse", TestListenDoesNotTakeOverAndCloseAllowsReuse)
	t.Run("cancelled-dial", TestDialHonorsCancelledContext)
	t.Run("bidirectional-payload", testIPCSmokePayload)
	t.Run("crash-restart", testIPCSmokeCrashRestart)
	runIPCSmokePlatform(t)
}

func testIPCSmokePayload(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	l, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	testIPCSmokeExchange(t, transport, endpoint, l)
}

func testIPCSmokeExchange(t *testing.T, transport, endpoint string, l net.Listener) {
	t.Helper()
	payload := []byte(strings.Repeat("IPC 引擎 & spaces\x00", 2048))
	serverDone := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err == nil {
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			_, err = io.CopyN(c, c, int64(len(payload)))
		}
		serverDone <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := DialContext(ctx, transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	written := make(chan error, 1)
	go func() { _, err := io.Copy(c, bytes.NewReader(payload)); written <- err }()
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reply, payload) {
		t.Fatal("IPC payload was corrupted")
	}
}

// The child stays connected until the parent kills only this test-owned PID.
// Unix must recover its abandoned socket; Windows must release the pipe name.
func TestIPCSmokeHelper(t *testing.T) {
	endpoint := os.Getenv("YAK_IPC_SMOKE_CHILD_ENDPOINT")
	if endpoint == "" {
		t.Skip("subprocess helper")
	}
	l, err := Listen(os.Getenv("YAK_IPC_SMOKE_CHILD_TRANSPORT"), endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fmt.Println("ipc smoke ready")
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("alive")); err != nil {
		t.Fatal(err)
	}
	select {}
}

func testIPCSmokeCrashRestart(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestIPCSmokeHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "YAK_IPC_SMOKE_CHILD_TRANSPORT="+transport, "YAK_IPC_SMOKE_CHILD_ENDPOINT="+endpoint)
	configureIPCSmokeChild(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready, done := make(chan struct{}, 1), make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "ipc smoke ready" {
				ready <- struct{}{}
			}
		}
		cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("IPC helper did not exit")
		}
	})
	select {
	case <-ready:
	case <-done:
		t.Fatalf("IPC helper exited before readiness: %s", stderr.String())
	case <-ctx.Done():
		t.Fatal("IPC helper readiness timeout")
	}
	client, err := DialContext(ctx, transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(client, make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("IPC helper kill timeout")
	}
	// Retain the old client handle: it must not prevent a new listener.
	l, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatalf("cannot reuse endpoint after crash: %v", err)
	}
	defer l.Close()
	testIPCSmokeExchange(t, transport, endpoint, l)
}
