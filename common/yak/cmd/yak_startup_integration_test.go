package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/engineendpoint"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type engineCLI struct {
	binary, home string
	env          []string
}

func newEngineCLI(t *testing.T) engineCLI {
	t.Helper()
	binary := os.Getenv("YAK_STARTUP_TEST_BINARY")
	if binary == "" {
		t.Skip("set YAK_STARTUP_TEST_BINARY to run real engine CLI acceptance tests")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("YAK_STARTUP_TEST_BINARY must be an absolute path")
	}
	home := t.TempDir()
	env := append(os.Environ(), "YAKIT_HOME="+home,
		"YAK_DEFAULT_PROJECT_DATABASE_NAME="+filepath.Join(home, "project.db"),
		"YAK_DEFAULT_PROFILE_DATABASE_NAME="+filepath.Join(home, "profile.db"),
		"SSA_DATABASE_RAW="+filepath.Join(home, "ssa.db"))
	return engineCLI{binary, home, env}
}

func (f engineCLI) run(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.binary, args...)
	cmd.Env, cmd.Dir = f.env, f.home
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("CLI did not exit within its deadline")
	}
	return output, err
}

func (f engineCLI) check(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	output, _ := f.run(t, append([]string{"check-secret-local-grpc"}, args...)...)
	const open, close = "<json-50551aa97b5aa5ae8a3c3243ac60a8a7>", "</json-50551aa97b5aa5ae8a3c3243ac60a8a7>"
	if bytes.Count(output, []byte(open)) != 1 {
		t.Fatal("missing/duplicate legacy check marker")
	}
	start, end := bytes.Index(output, []byte(open))+len(open), bytes.Index(output, []byte(close))
	if end < start {
		t.Fatal("incomplete check output")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output[start:end], &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func freeEnginePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	return port
}

func TestCLIStartupLegacyCheckContract(t *testing.T) {
	f := newEngineCLI(t)
	port := freeEnginePort(t)
	first := f.check(t, "--port", port)
	second := f.check(t, "--port", port)
	secret, _ := first["secret"].(string)
	if first["ok"] != true || len(secret) != 64 || secret == second["secret"] {
		t.Fatal("check does not return a fresh legacy-compatible credential")
	}
	if first["host"] != "127.0.0.1" || first["addr"] != "127.0.0.1:"+port || first["transport"] != "tcp" {
		t.Fatal("legacy TCP address changed")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	failed := f.check(t, "--port", strconv.Itoa(l.Addr().(*net.TCPAddr).Port))
	reasons, ok := failed["reason"].([]interface{})
	if !ok || len(reasons) != 1 || reasons[0] != "net.Listen(tcp, addr) failed" || failed["reasonCode"] != "tcp_bind_in_use" {
		t.Fatal("occupied port no longer reaches old Yakit's port recovery route")
	}
}

func TestCLIStartupInvalidOptionsDoNotInitializeDatabases(t *testing.T) {
	for _, command := range []string{"grpc", "check-secret-local-grpc"} {
		t.Run(command, func(t *testing.T) {
			f := newEngineCLI(t)
			output, err := f.run(t, command, "--transport", "invalid-transport")
			if err == nil {
				t.Fatal("invalid transport silently succeeded")
			}
			if !bytes.Contains(output, []byte(`"phase":"init"`)) && !bytes.Contains(output, []byte(`"phase": "init"`)) {
				t.Fatal("missing structured init failure")
			}
			if _, err := os.Stat(filepath.Join(f.home, "project.db")); !os.IsNotExist(err) {
				t.Fatal("invalid startup initialized databases")
			}
		})
	}
}

func integrationEndpoint(t *testing.T) (string, string) {
	t.Helper()
	secret, err := newEngineSecret()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return "npipe", `\\.\pipe\yakit-integration-` + secret[:16]
	}
	dir, err := os.MkdirTemp("/tmp", "yakcli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return "unix", filepath.Join(dir, "engine.sock")
}

func TestCLIStartupIPCRequiresAuthAndPreservesLiveEndpoint(t *testing.T) {
	f := newEngineCLI(t)
	transport, endpoint := integrationEndpoint(t)
	output, err := f.run(t, "grpc", "--transport", transport, "--socket-path", endpoint)
	if err == nil || !bytes.Contains(output, []byte("yak grpc failed")) {
		t.Fatal("unauthenticated IPC startup was accepted")
	}
	if _, err := os.Stat(filepath.Join(f.home, "project.db")); !os.IsNotExist(err) {
		t.Fatal("invalid IPC startup initialized databases")
	}

	secret, err := newEngineSecret()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.binary, "grpc", "--transport", transport, "--socket-path", endpoint, "--local-password", secret)
	cmd.Env, cmd.Dir, cmd.Stderr = f.env, f.home, io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	ready := make(chan grpcReadyEvent, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, grpcReadyMarkerPrefix) {
				var event grpcReadyEvent
				if json.Unmarshal([]byte(strings.TrimPrefix(line, grpcReadyMarkerPrefix)), &event) == nil {
					select {
					case ready <- event:
					default:
					}
				}
			}
		}
		done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		cancel()
		cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("engine child did not exit")
		}
	})
	select {
	case event := <-ready:
		if event.Transport != transport || event.Address != endpoint {
			t.Fatal("ready advertises a different endpoint")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("IPC engine readiness timeout")
	}
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()
	conn, err := grpc.DialContext(dialCtx, "passthrough:///engine", grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return engineendpoint.DialContext(ctx, transport, endpoint)
		}))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := ypb.NewYakClient(conn)
	echo := func(password string) error {
		rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer rpcCancel()
		if password != "" {
			rpcCtx = metadata.AppendToOutgoingContext(rpcCtx, "authorization", "bearer "+password)
		}
		response, err := client.Echo(rpcCtx, &ypb.EchoRequest{Text: "startup-test"})
		if err == nil && response.Result != "startup-test" {
			t.Fatal("unexpected Echo reply")
		}
		return err
	}
	if err := echo(""); status.Code(err) != codes.Unauthenticated {
		t.Fatal("anonymous IPC request was not rejected")
	}
	if err := echo("wrong"); err == nil || !strings.Contains(err.Error(), "secret verify failed") {
		t.Fatal("wrong IPC password was not rejected")
	}
	if err := echo(secret); err != nil {
		t.Fatalf("authenticated IPC failed: %v", err)
	}
	check := newEngineCLI(t).check(t, "--transport", transport, "--socket-path", endpoint)
	if check["ok"] != false || check["phase"] != "listen" {
		t.Fatal("check took over a live endpoint")
	}
	if err := echo(secret); err != nil {
		t.Fatal("check disrupted the existing engine")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(endpoint)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("insecure socket permissions")
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			done <- err // cleanup also observes termination
			if err != nil {
				t.Fatalf("graceful IPC shutdown failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("IPC shutdown timed out")
		}
		if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
			t.Fatal("normal shutdown left its socket behind")
		}
	}
}
