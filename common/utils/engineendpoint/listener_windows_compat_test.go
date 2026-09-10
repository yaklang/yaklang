//go:build windows

package engineendpoint

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNamedPipeWindowsCollision(t *testing.T) {
	transport, endpoint := testEndpoint(t)
	l, err := Listen(transport, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	second, err := Listen(transport, endpoint)
	if err == nil {
		second.Close()
		t.Fatal("accepted a duplicate pipe")
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("duplicate pipe must be identifiable as already existing: %v", err)
	}
}

func TestNamedPipeUnicodeName(t *testing.T) {
	_, unique := testEndpoint(t)
	for _, upper := range []bool{false, true} {
		endpoint := unique + strings.Repeat("引擎", 60)
		if upper {
			endpoint = strings.ToUpper(endpoint)
		}
		if err := Validate("npipe", endpoint); err != nil {
			t.Fatal(err)
		}
		l, err := Listen("npipe", endpoint)
		if err != nil {
			t.Fatal(err)
		}
		l.Close()
	}
	for _, suffix := range []string{strings.Repeat("界", 249), strings.Repeat("😀", 125), "invalid\xff"} {
		if err := Validate("npipe", unique[:8]+suffix); err == nil {
			t.Fatalf("accepted invalid UTF-16 pipe name: %q", suffix)
		}
	}
}

const pipeCompatibilityPayload = "Windows named pipe: 引擎 & spaces"

func exchangePipe(t *testing.T, endpoint string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialContext(ctx, "npipe", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, pipeCompatibilityPayload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(pipeCompatibilityPayload))
	if _, err := io.ReadFull(c, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != pipeCompatibilityPayload {
		t.Fatalf("corrupted pipe response: %q", response)
	}
}

func servePipeEcho(l net.Listener) error {
	c, err := l.Accept()
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = io.CopyN(c, c, int64(len(pipeCompatibilityPayload)))
	return err
}

// A child process exercises actual tokens, working directories and drive letters.
func TestNamedPipeProcessHelper(t *testing.T) {
	mode := os.Getenv("YAK_NPIPE_HELPER_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	endpoint := os.Getenv("YAK_NPIPE_HELPER_ENDPOINT")
	fmt.Printf("helper elevated=%t\n", windows.GetCurrentProcessToken().IsElevated())
	if mode == "dial" {
		exchangePipe(t, endpoint)
		return
	}
	l, err := Listen("npipe", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fmt.Println("pipe helper ready")
	if err := servePipeEcho(l); err != nil {
		t.Fatal(err)
	}
}

func pipeHelperCommand(t *testing.T, binary, dir, mode, endpoint string, token windows.Token) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestNamedPipeProcessHelper$", "-test.v")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "YAK_NPIPE_HELPER_MODE="+mode, "YAK_NPIPE_HELPER_ENDPOINT="+endpoint)
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(token), HideWindow: true}
	return cmd
}

func testPipeChildServer(t *testing.T, binary, dir string, token windows.Token) {
	t.Helper()
	_, endpoint := testEndpoint(t)
	cmd := pipeHelperCommand(t, binary, dir, "listen", endpoint, token)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		t.Log(scanner.Text())
		if scanner.Text() == "pipe helper ready" {
			ready = true
			break
		}
	}
	if ready {
		exchangePipe(t, endpoint)
	}
	rest, _ := io.ReadAll(stdout)
	if err := cmd.Wait(); err != nil || !ready {
		t.Fatalf("child server: ready=%v, err=%v\n%s\n%s", ready, err, rest, stderr.String())
	}
	l, err := Listen("npipe", endpoint)
	if err != nil {
		t.Fatalf("child left a stale pipe: %v", err)
	}
	l.Close()
}

func TestNamedPipeDifferentWorkingDirectory(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "存量用户 data & spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	testPipeChildServer(t, binary, dir, 0)
}

func TestNamedPipeDifferentDrive(t *testing.T) {
	root := os.Getenv("YAK_NPIPE_TEST_SECOND_ROOT")
	if root == "" {
		t.Skip("set YAK_NPIPE_TEST_SECOND_ROOT to a writable directory on another drive")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) || strings.EqualFold(filepath.VolumeName(binary), filepath.VolumeName(root)) {
		t.Fatal("second root must use a different absolute drive letter from the test executable")
	}
	dir, err := os.MkdirTemp(root, "npipe-跨盘 & ")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	source, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	childBinary := filepath.Join(dir, "引擎 helper.exe")
	dest, err := os.Create(childBinary)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy child: %v, close: %v", copyErr, closeErr)
	}
	testPipeChildServer(t, childBinary, t.TempDir(), 0)
}

func TestNamedPipeMixedElevation(t *testing.T) {
	current := windows.GetCurrentProcessToken()
	if !current.IsElevated() {
		t.Skip("run the test binary elevated to exercise both UAC tokens")
	}
	ordinary := ordinaryTestToken(t)
	defer ordinary.Close()
	if ordinary.IsElevated() {
		t.Fatal("restricted token is unexpectedly elevated")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("ordinary_server_elevated_client", func(t *testing.T) {
		testPipeChildServer(t, binary, t.TempDir(), ordinary)
	})
	t.Run("elevated_server_ordinary_client", func(t *testing.T) {
		_, endpoint := testEndpoint(t)
		l, err := Listen("npipe", endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		done := make(chan error, 1)
		go func() { done <- servePipeEcho(l) }()
		cmd := pipeHelperCommand(t, binary, t.TempDir(), "dial", endpoint, ordinary)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ordinary client: %v\n%s", err, output)
		}
		if !bytes.Contains(output, []byte("helper elevated=false")) {
			t.Fatalf("child was not verified non-elevated: %s", output)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// Remove administrator groups and all optional privileges, then lower the child
// to medium integrity. This creates a restricted primary token of the caller;
// it needs no new account, credentials, or SeAssignPrimaryTokenPrivilege.
func ordinaryTestToken(t *testing.T) windows.Token {
	t.Helper()
	var current windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ALL_ACCESS, &current); err != nil {
		t.Fatalf("open current token: %v", err)
	}
	defer current.Close()
	admin, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		t.Fatal(err)
	}
	localAdmin, err := windows.StringToSid("S-1-5-114")
	if err != nil {
		t.Fatal(err)
	}
	disabled := []windows.SIDAndAttributes{{Sid: admin}, {Sid: localAdmin}}
	var token windows.Token
	ok, _, callErr := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken").Call(
		uintptr(current), 1, uintptr(len(disabled)), uintptr(unsafe.Pointer(&disabled[0])),
		0, 0, 0, 0, uintptr(unsafe.Pointer(&token)))
	if ok == 0 {
		t.Fatalf("create restricted token: %v", callErr)
	}
	medium, err := windows.StringToSid("S-1-16-8192")
	if err != nil {
		token.Close()
		t.Fatal(err)
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: medium, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(token, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		token.Close()
		t.Fatalf("set medium integrity: %v", err)
	}
	return token
}

func TestNamedPipeRestrictedToken(t *testing.T) {
	token := ordinaryTestToken(t)
	defer token.Close()
	if token.IsElevated() {
		t.Fatal("restricted token is elevated")
	}
}
