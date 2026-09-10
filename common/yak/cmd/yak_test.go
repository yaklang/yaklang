package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils/engineendpoint"
)

func TestIPCListenDiagnosticsDoNotSuggestTCPRecovery(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{engineendpoint.ErrInvalidDirectory, ipcEndpointInvalid},
		{syscall.EACCES, ipcBindDenied},
		{os.ErrPermission, ipcBindDenied},
		{syscall.EADDRINUSE, ipcBindInUse},
		{syscall.ENOENT, ipcBindGeneric},
	} {
		code, hint := classifyEndpointListenError("unix", fmt.Errorf("endpoint: %w", tc.err))
		if code != tc.code || hint == "" || grpcEventReasonI18n(code) == nil {
			t.Fatalf("invalid IPC diagnostic: code=%s hint=%s", code, hint)
		}
		code, _ = classifyEndpointListenError("tcp", tc.err)
		legacy, _ := classifyListenError(tc.err)
		if code != legacy {
			t.Fatal("changed TCP error classification")
		}
	}
	if runtime.GOOS == "windows" {
		code, _ := classifyEndpointListenError("npipe", &os.PathError{Op: "listen", Path: `\\.\pipe\test`, Err: syscall.Errno(5)})
		if code != ipcBindDenied {
			t.Fatalf("Win32 access denied was misclassified: %s", code)
		}
	}
}

func TestCheckProtocolPreservesLegacyReasons(t *testing.T) {
	for code, legacy := range map[string]string{
		databaseError: "database error", dialGrpcServerFailed: "dial grpc server failed",
		callVersionFailed: "call Version RPC failed", buildYakGrpcServer: "build yak grpc server failed",
		waitConnectFailed: "waiting grpc listener failed", tcpBindInUse: "net.Listen(tcp, addr) failed",
		tcpBindDenied: "net.Listen(tcp, addr) failed", tcpBindGeneric: "net.Listen(tcp, addr) failed", "": "",
	} {
		if got := legacyCheckReason(code); got != legacy {
			t.Fatalf("%s broke old Yakit: %q", code, got)
		}
	}
}

func TestEngineSessionSecretsAreRandomCredentials(t *testing.T) {
	first, err := newEngineSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newEngineSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || first == second || strings.Trim(first, "*") == "" {
		t.Fatal("invalid or reused session secret")
	}
}

func TestIPCOptionsAreValidatedBeforeStartup(t *testing.T) {
	transport, endpoint := "unix", "/tmp/yak-test/engine.sock"
	if runtime.GOOS == "windows" {
		transport, endpoint = "npipe", `\\.\pipe\yakit-test`
	}
	for _, tc := range []struct {
		name  string
		args  []string
		valid bool
	}{
		{"legacy TCP", nil, true},
		{"unknown transport", []string{"--transport", "unexpected"}, false},
		{"missing IPC auth", []string{"--transport", transport, "--socket-path", endpoint}, false},
		{"masked IPC auth", []string{"--transport", transport, "--socket-path", endpoint, "--secret", "***"}, false},
		{"authenticated IPC", []string{"--transport", transport, "--socket-path", endpoint, "--local-password", "test-secret"}, true},
		{"ignored TLS", []string{"--transport", transport, "--socket-path", endpoint, "--secret", "test-secret", "--tls"}, false},
		{"ignored port", []string{"--transport", transport, "--socket-path", endpoint, "--secret", "test-secret", "--port", "9011"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			for _, option := range startGRPCServerCommand.Flags {
				option.Apply(set)
			}
			if err := set.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			ctx := cli.NewContext(nil, set, nil)
			if err := validateEngineTransportOptions(ctx, true); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestWriteGRPCReadyEvent(t *testing.T) {
	var output bytes.Buffer
	if err := writeGRPCReadyEvent(&output, "127.0.0.1:54321", "tcp", "testid1"); err != nil {
		t.Fatalf("write ready event: %v", err)
	}

	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, grpcReadyMarkerPrefix) {
		t.Fatalf("unexpected marker: %q", line)
	}

	var event grpcReadyEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, grpcReadyMarkerPrefix)), &event); err != nil {
		t.Fatalf("decode ready event: %v", err)
	}
	if event.SchemaVersion != 2 || event.Address != "127.0.0.1:54321" || event.Transport != "tcp" || event.InstanceId != "testid1" {
		t.Fatalf("unexpected ready event: %#v", event)
	}
}

func TestStartGRPCPProfServerPublishesLoopbackReadyEvent(t *testing.T) {
	var output bytes.Buffer
	server, actualAddress, err := startGRPCPProfServer(&output, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start pprof server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, grpcPProfReadyMarkerPrefix) {
		t.Fatalf("unexpected marker: %q", line)
	}
	var event grpcPProfReadyEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, grpcPProfReadyMarkerPrefix)), &event); err != nil {
		t.Fatalf("decode pprof ready event: %v", err)
	}
	if event.SchemaVersion != 1 || event.Address != actualAddress || !strings.HasPrefix(event.Address, "127.0.0.1:") {
		t.Fatalf("unexpected pprof ready event: %#v", event)
	}

	response, err := http.Get("http://" + event.Address + "/debug/pprof/")
	if err != nil {
		t.Fatalf("get pprof index: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected pprof status: %s", response.Status)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("read pprof index: %v", err)
	}
}

func TestRunCheckSecretCleanupWithTimeoutRunsCleanup(t *testing.T) {
	var called atomic.Bool
	runCheckSecretCleanupWithTimeout("test cleanup", time.Second, func() {
		called.Store(true)
	})
	if !called.Load() {
		t.Fatal("expected cleanup to run")
	}
}

func TestRunCheckSecretCleanupWithTimeoutReturnsAfterTimeout(t *testing.T) {
	unblock := make(chan struct{})
	start := time.Now()

	runCheckSecretCleanupWithTimeout("blocked cleanup", 10*time.Millisecond, func() {
		<-unblock
	})
	close(unblock)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected timeout cleanup to return quickly, elapsed: %s", elapsed)
	}
}
