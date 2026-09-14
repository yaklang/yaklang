package engineendpoint

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateTransport(t *testing.T) {
	for _, c := range []struct{ transport, path string }{
		{"invalid", ""}, {"tcp", "unexpected.sock"}, {"unix", "relative.sock"},
		{"unix", "/" + strings.Repeat("x", 104)}, {"unix", "/tmp/null\x00"},
		{"npipe", ""}, {"npipe", `\\server\pipe\remote`}, {"npipe", `\\.\pipe\a\b`},
	} {
		t.Run(c.transport+"/"+c.path, func(t *testing.T) {
			if err := Validate(c.transport, c.path); err == nil {
				t.Fatal("accepted invalid endpoint")
			}
		})
	}
	if err := Validate("tcp", ""); err != nil {
		t.Fatal(err)
	}
	transport, wrong, endpoint := "unix", "npipe", "/tmp/yak/sock"
	if runtime.GOOS == "windows" {
		transport, wrong, endpoint = "npipe", "unix", `\\.\pipe\yakit-test.sock`
	}
	if err := Validate(transport, endpoint); err != nil {
		t.Fatal(err)
	}
	if err := Validate(wrong, endpoint); err == nil {
		t.Fatal("accepted another platform's transport")
	}
}

func testEndpoint(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "npipe", `\\.\pipe\yakit-test-` + filepath.Base(t.TempDir()) + "-" + time.Now().Format("150405.000000000")
	}
	dir, err := os.MkdirTemp("/tmp", "yakep-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return "unix", filepath.Join(dir, "engine.sock")
}

func TestListenDoesNotTakeOverAndCloseAllowsReuse(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	first, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })
	if second, err := Listen(transport, endpoint); err == nil {
		second.Close()
		t.Fatal("second listener took over")
	}
	accepted := make(chan error, 1)
	go func() {
		c, err := first.Accept()
		if err == nil {
			_, err = c.Write([]byte("alive"))
			c.Close()
		}
		accepted <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := DialContext(ctx, transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 5)
	n, err := io.ReadFull(client, buf)
	client.Close()
	if err != nil || string(buf[:n]) != "alive" {
		t.Fatalf("original listener unreachable: %v", err)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
}

func TestDialHonorsCancelledContext(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if c, err := DialContext(ctx, transport, endpoint); err == nil {
		c.Close()
		t.Fatal("cancelled dial succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancelled dial did not return promptly")
	}
}
