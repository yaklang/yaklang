//go:build windows

package engineendpoint

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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// These acceptance tests use the built CLI and isolated databases. They do not
// import the engine server or initialize the user's existing Yakit data.
type windowsEngineFixture struct {
	binary, home string
	env          []string
}

func newWindowsEngineFixture(t *testing.T) windowsEngineFixture {
	t.Helper()
	binary := os.Getenv("YAK_STARTUP_TEST_BINARY")
	if binary == "" {
		t.Skip("set YAK_STARTUP_TEST_BINARY to the Windows engine built from this checkout")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("engine binary must be an absolute path")
	}
	home := filepath.Join(t.TempDir(), "存量用户 data & spaces")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "YAKIT_HOME="+home,
		"YAK_DEFAULT_PROJECT_DATABASE_NAME="+filepath.Join(home, "project.db"),
		"YAK_DEFAULT_PROFILE_DATABASE_NAME="+filepath.Join(home, "profile.db"),
		"SSA_DATABASE_RAW="+filepath.Join(home, "ssa.db"))
	return windowsEngineFixture{binary, home, env}
}

func (f windowsEngineFixture) command(t *testing.T, token windows.Token, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, f.binary, args...)
	cmd.Dir, cmd.Env = f.home, f.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(token), HideWindow: true}
	return cmd
}

func (f windowsEngineFixture) check(t *testing.T, token windows.Token, endpoint, password string) (map[string]interface{}, string) {
	t.Helper()
	args := []string{"check-secret-local-grpc", "--transport", "npipe", "--socket-path", endpoint}
	if password != "" {
		args = append(args, "--client-password", password)
	}
	output, cmdErr := f.command(t, token, args...).CombinedOutput()
	const startMarker = "<json-50551aa97b5aa5ae8a3c3243ac60a8a7>"
	const endMarker = "</json-50551aa97b5aa5ae8a3c3243ac60a8a7>"
	start, end := bytes.Index(output, []byte(startMarker)), bytes.Index(output, []byte(endMarker))
	if start < 0 || end < start {
		t.Fatalf("missing structured result: %v\n%s", cmdErr, output)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output[start+len(startMarker):end], &result); err != nil {
		t.Fatal(err)
	}
	if (cmdErr == nil) != (result["ok"] == true) {
		t.Fatalf("exit code and check result disagree: %v, ok=%v", cmdErr, result["ok"])
	}
	// Exclude the protocol credential from diagnostic assertions and logs.
	return result, string(output[:start])
}

func (f windowsEngineFixture) start(t *testing.T, token windows.Token, endpoint, password string) func() {
	t.Helper()
	cmd := f.command(t, token, "grpc", "--transport", "npipe", "--socket-path", endpoint, "--local-password", password)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	done := make(chan struct{})
	var exitErr error
	var output bytes.Buffer
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line + "\n")
			const marker = "yak grpc ready "
			if strings.HasPrefix(line, marker) {
				select {
				case ready <- strings.TrimPrefix(line, marker):
				default:
				}
			}
		}
		exitErr = cmd.Wait()
		close(done)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("engine did not exit")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case raw := <-ready:
		var event struct {
			Address   string `json:"address"`
			Transport string `json:"transport"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err != nil || event.Transport != "npipe" || event.Address != endpoint {
			t.Fatalf("invalid readiness event: %s", raw)
		}
	case <-done:
		t.Fatalf("engine exited before readiness: %v\n%s", exitErr, output.String())
	case <-time.After(45 * time.Second):
		stop()
		t.Fatal("engine readiness exceeded 45 seconds")
	}
	return stop
}

func TestWindowsEngineStartupCompatibility(t *testing.T) {
	f := newWindowsEngineFixture(t)
	_, endpoint := testEndpoint(t)
	// A busy legacy port must not affect explicitly selected npipe startup.
	occupied, err := net.Listen("tcp", "127.0.0.1:9011")
	if err == nil {
		defer occupied.Close()
	} else {
		t.Logf("legacy port already unavailable: %v", err)
	}
	// Inherited proxy settings must not redirect local IPC to a TCP proxy.
	f.env = append(f.env, "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=")
	check, logs := f.check(t, 0, endpoint, "")
	secret, _ := check["secret"].(string)
	if check["ok"] != true || len(secret) != 64 || check["addr"] != endpoint || check["port"] != float64(0) {
		t.Fatalf("npipe preflight failed: phase=%v reason=%v\n%s", check["phase"], check["reasonCode"], logs)
	}
	stop := f.start(t, 0, endpoint, secret)
	client := newWindowsEngineFixture(t)
	for _, password := range []string{"wrong-password", secret} {
		result, logs := client.check(t, 0, endpoint, password)
		if (result["ok"] == true) != (password == secret) {
			t.Fatalf("unexpected authentication result: phase=%v reason=%v", result["phase"], result["reasonCode"])
		}
		if strings.Contains(logs, "127.0.0.1:9011") || strings.Contains(logs, "Server: 127.") {
			t.Fatalf("IPC diagnostics advertise TCP: %s", logs)
		}
	}
	duplicate, _ := client.check(t, 0, endpoint, "")
	if duplicate["ok"] != false || duplicate["reasonCode"] != "ipc_bind_in_use" {
		t.Fatalf("duplicate pipe misclassified: phase=%v reason=%v", duplicate["phase"], duplicate["reasonCode"])
	}
	result, _ := client.check(t, 0, endpoint, secret)
	if result["ok"] != true {
		t.Fatal("duplicate preflight disrupted the live engine")
	}
	stop()
	// An abrupt exit must release the name; the same user data must reopen.
	stop = f.start(t, 0, endpoint, secret)
	result, _ = client.check(t, 0, endpoint, secret)
	if result["ok"] != true {
		t.Fatal("restart with existing data failed")
	}
	stop()
}

func TestWindowsEngineMissingPipeDiagnostics(t *testing.T) {
	f := newWindowsEngineFixture(t)
	_, endpoint := testEndpoint(t)
	result, logs := f.check(t, 0, endpoint, "test-password")
	if result["ok"] != false || result["phase"] != "dial" || result["addr"] != endpoint {
		t.Fatalf("missing pipe was not reported as an IPC connection failure: %v", result["phase"])
	}
	if !strings.Contains(logs, endpoint) {
		t.Fatal("connection failure omitted the actual pipe name")
	}
	for _, misleading := range []string{"127.0.0.1:", "port 9011", "firewall"} {
		if strings.Contains(logs, misleading) {
			t.Fatalf("IPC failure suggested TCP recovery: %s", logs)
		}
	}
}

func TestWindowsEngineMixedElevation(t *testing.T) {
	f := newWindowsEngineFixture(t)
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("run elevated to test actual administrator/ordinary startup and existing-data reuse")
	}
	ordinary := ordinaryTestToken(t)
	defer ordinary.Close()
	_, endpoint := testEndpoint(t)
	const secret = "windows-compatibility-test-password"
	// The first engine creates databases while elevated. The next engine must
	// reopen those same files as the ordinary desktop user, then reverse roles.
	for _, tc := range []struct {
		name           string
		server, client windows.Token
	}{
		{"elevated_server_ordinary_client", 0, ordinary},
		{"ordinary_server_elevated_client_existing_data", ordinary, 0},
		{"elevated_server_again", 0, ordinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stop := f.start(t, tc.server, endpoint, secret)
			client := newWindowsEngineFixture(t)
			result, logs := client.check(t, tc.client, endpoint, secret)
			if result["ok"] != true {
				t.Fatalf("mixed-elevation RPC failed: phase=%v reason=%v\n%s", result["phase"], result["reasonCode"], logs)
			}
			wrong, _ := client.check(t, tc.client, endpoint, "wrong-password")
			if wrong["ok"] != false || wrong["phase"] != "version_rpc" {
				t.Fatal("mixed-elevation client bypassed password authentication")
			}
			stop()
		})
	}
}

func TestWindowsEngineDifferentDrive(t *testing.T) {
	f := newWindowsEngineFixture(t)
	root := os.Getenv("YAK_NPIPE_TEST_SECOND_ROOT")
	if root == "" {
		t.Skip("set YAK_NPIPE_TEST_SECOND_ROOT to test the full engine on another drive")
	}
	if !filepath.IsAbs(root) || strings.EqualFold(filepath.VolumeName(f.binary), filepath.VolumeName(root)) {
		t.Fatal("engine binary and second root must use different absolute drive letters")
	}
	dir, err := os.MkdirTemp(root, "engine-跨盘 & ")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	source, err := os.Open(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	binary := filepath.Join(dir, "引擎 windows.exe")
	dest, err := os.Create(binary)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy engine: %v, close: %v", copyErr, closeErr)
	}
	f.binary = binary
	_, endpoint := testEndpoint(t)
	const secret = "cross-drive-test-password"
	stop := f.start(t, 0, endpoint, secret)
	defer stop()
	client := newWindowsEngineFixture(t)
	result, logs := client.check(t, 0, endpoint, secret)
	if result["ok"] != true {
		t.Fatalf("cross-drive RPC failed: phase=%v reason=%v\n%s", result["phase"], result["reasonCode"], logs)
	}
}

func TestWindowsEngineOccupiedPipeDoesNotInitializeDatabases(t *testing.T) {
	_, endpoint := testEndpoint(t)
	l, err := Listen("npipe", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for _, command := range []string{"grpc", "check-secret-local-grpc"} {
		t.Run(command, func(t *testing.T) {
			f := newWindowsEngineFixture(t)
			args := []string{command, "--transport", "npipe", "--socket-path", endpoint}
			if command == "grpc" {
				args = append(args, "--local-password", "occupied-pipe-test-password")
			}
			output, err := f.command(t, 0, args...).CombinedOutput()
			if err == nil || !bytes.Contains(output, []byte("ipc_bind_in_use")) {
				t.Fatalf("missing IPC collision result: %v", err)
			}
			if command == "grpc" && bytes.Count(output, []byte("yak grpc failed ")) != 1 {
				t.Fatal("expected exactly one structured startup failure")
			}
			for _, name := range []string{"project.db", "profile.db", "ssa.db"} {
				if _, err := os.Stat(filepath.Join(f.home, name)); !os.IsNotExist(err) {
					t.Fatalf("occupied IPC initialized %s", name)
				}
			}
		})
	}
	// Rejected starts must not connect to or disrupt the current owner.
	done := make(chan error, 1)
	go func() { done <- servePipeEcho(l) }()
	exchangePipe(t, endpoint)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWindowsEngineReservedPipeReleasedOnStartupFailure(t *testing.T) {
	for _, command := range []string{"grpc", "check-secret-local-grpc"} {
		t.Run(command, func(t *testing.T) {
			f := newWindowsEngineFixture(t)
			_, endpoint := testEndpoint(t)
			// An invalid Win32 filename fails database opening after reservation.
			// Do not use '?': SQLite interprets it as the start of DSN options.
			args := []string{command, "--transport", "npipe", "--socket-path", endpoint,
				"--profile-db", filepath.Join(f.home, "invalid|.db")}
			if command == "grpc" {
				args = append(args, "--local-password", "failed-start-test-password")
			}
			output, err := f.command(t, 0, args...).CombinedOutput()
			if err == nil || !bytes.Contains(output, []byte(`"phase":"database"`)) {
				// The check protocol is pretty-printed, while grpc events are compact.
				if err == nil || !bytes.Contains(output, []byte(`"phase": "database"`)) {
					t.Fatalf("expected database failure after pipe reservation: %v", err)
				}
			}
			l, err := Listen("npipe", endpoint)
			if err != nil {
				t.Fatalf("failed startup leaked its reserved pipe: %v", err)
			}
			l.Close()
		})
	}
}

func TestWindowsEngineFailureEventBypassesCachedOutput(t *testing.T) {
	// Use a TCP bind failure to reach the same Windows grpc action *after*
	// NewServer enables stdout caching. Npipe now fails before database work.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	f := newWindowsEngineFixture(t)
	port := strconv.Itoa(occupied.Addr().(*net.TCPAddr).Port)
	output, err := f.command(t, 0, "grpc", "--port", port, "--local-password", "cached-output-test-password").CombinedOutput()
	if err == nil || bytes.Count(output, []byte("yak grpc failed ")) != 1 || !bytes.Contains(output, []byte("tcp_bind_in_use")) {
		t.Fatalf("cached logging swallowed or duplicated the structured failure: %v", err)
	}
	if bytes.Contains(output, []byte("cached-output-test-password")) {
		t.Fatal("control output leaked the password")
	}
}
