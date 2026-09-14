package subprocess

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// threadSafeBuffer is a strings.Builder guarded by a mutex so that
// the drain goroutine and the test can safely access it concurrently.
type threadSafeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *threadSafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "subprocess-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	helperPath := tmp + "/helper"
	if err := exec.Command("go", "build", "-o", helperPath, "testdata/helper.go").Run(); err != nil {
		panic(err)
	}
	os.Setenv("SUBPROCESS_TEST_HELPER", helperPath)

	os.Exit(m.Run())
}

func helperBin(t *testing.T) string {
	t.Helper()
	p := os.Getenv("SUBPROCESS_TEST_HELPER")
	if p == "" {
		t.Fatal("SUBPROCESS_TEST_HELPER not set")
	}
	return p
}

func TestLaunch_BasicExit(t *testing.T) {
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "exit-immediately"),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	<-p.Done()
}

func TestLaunch_StdoutWriterOnly(t *testing.T) {
	var buf threadSafeBuffer
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stdout", "hello"),
		Stdio: StdioConfig{
			Stdout: &buf,
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	<-p.Done()
	p.drainWg.Wait()
	got := strings.TrimSpace(buf.String())
	if got != "hello" {
		t.Fatalf("stdout = %q, want %q", got, "hello")
	}
}

func TestLaunch_StderrWriterOnly(t *testing.T) {
	var buf threadSafeBuffer
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stderr", "boom"),
		Stdio: StdioConfig{
			Stderr: &buf,
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	<-p.Done()
	p.drainWg.Wait()
	got := strings.TrimSpace(buf.String())
	if got != "boom" {
		t.Fatalf("stderr = %q, want %q", got, "boom")
	}
}

func TestLaunch_StdoutTapReader(t *testing.T) {
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stdout", "tapped"),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	r := p.Stdout()
	if r == nil {
		t.Fatal("Stdout() returned nil")
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		t.Fatalf("scan: %v", scanner.Err())
	}
	got := strings.TrimSpace(scanner.Text())
	if got != "tapped" {
		t.Fatalf("tap stdout = %q, want %q", got, "tapped")
	}
	<-p.Done()
}

func TestLaunch_StdoutWriterAndTap(t *testing.T) {
	var buf threadSafeBuffer
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stdout", "dual"),
		Stdio: StdioConfig{
			Stdout: &buf,
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	r := p.Stdout()
	if r == nil {
		t.Fatal("Stdout() returned nil")
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		t.Fatalf("scan: %v", scanner.Err())
	}
	tapGot := strings.TrimSpace(scanner.Text())
	<-p.Done()
	p.drainWg.Wait()

	writerGot := strings.TrimSpace(buf.String())
	if tapGot != "dual" {
		t.Fatalf("tap = %q, want %q", tapGot, "dual")
	}
	if writerGot != "dual" {
		t.Fatalf("writer = %q, want %q", writerGot, "dual")
	}
}

func TestLaunch_StdinFromPipe(t *testing.T) {
	pr, pw := io.Pipe()
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "cat-stdin"),
		Stdio: StdioConfig{
			Stdin: pr,
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	go func() {
		_, _ = pw.Write([]byte("piped-input\n"))
		_ = pw.Close()
	}()

	r := p.Stdout()
	if r == nil {
		t.Fatal("Stdout() returned nil")
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		t.Fatalf("scan: %v", scanner.Err())
	}
	got := strings.TrimSpace(scanner.Text())
	if got != "piped-input" {
		t.Fatalf("stdin roundtrip = %q, want %q", got, "piped-input")
	}
	<-p.Done()
}

func TestLaunch_StdoutFileInheritance(t *testing.T) {
	tmp, err := os.CreateTemp("", "subprocess-stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stdout", "inherited"),
		Stdio: StdioConfig{
			Stdout: tmp,
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	<-p.Done()

	if r := p.Stdout(); r != nil {
		t.Fatal("Stdout() should return nil for *os.File")
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "inherited" {
		t.Fatalf("file stdout = %q, want %q", got, "inherited")
	}
}

func TestLaunch_Ready(t *testing.T) {
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "echo-stdout", "ready-test"),
		Ready: func(ctx context.Context, mp *ManagedProcess) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if p.PID() == 0 {
		t.Fatal("PID is 0 after ready")
	}
	<-p.Done()
}

func TestLaunch_ReadyTimeout(t *testing.T) {
	ctx := context.Background()
	_, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "run-forever"),
		Ready: func(ctx context.Context, mp *ManagedProcess) error {
			<-ctx.Done()
			return ctx.Err()
		},
		StartupTimeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestLaunch_GracefulShutdown(t *testing.T) {
	shutdownCalled := false
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "run-forever"),
		GracefulShutdown: func(mp *ManagedProcess) error {
			shutdownCalled = true
			return nil
		},
		ShutdownTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	p.Close()
	if !shutdownCalled {
		t.Fatal("GracefulShutdown was not called")
	}
}

func TestLaunch_KillProcessGroup(t *testing.T) {
	ctx := context.Background()
	p, err := Launch(ctx, &LaunchConfig{
		Cmd: exec.Command(helperBin(t), "run-forever"),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	pid := p.PID()
	if pid == 0 {
		t.Fatal("PID is 0")
	}
	p.Kill()
	<-p.Done()
	if p.cmd.ProcessState == nil {
		t.Fatal("ProcessState is nil after Kill")
	}
}

func TestBuildChildEnvironment(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"HOME=/root",
		"SECRET=value",
		"secret=lowercase",
	}
	got := BuildChildEnvironment(parent, []string{"SECRET"}, []string{"NEW=extra"})
	for _, entry := range got {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "SECRET") {
			t.Fatalf("SECRET should be excluded, found: %s", entry)
		}
	}
	found := false
	for _, entry := range got {
		if entry == "NEW=extra" {
			found = true
		}
	}
	if !found {
		t.Fatal("NEW=extra not found in result")
	}
}
