package bruteutils_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real VNC servers via Docker (local daemon or DOCKER_HOST / Windows docker context).
//
//	YAK_VNC_DOCKER=1 go test ./common/utils/bruteutils/ -run TestVNCDocker -count=1 -timeout 20m -v
//
// Covers TigerVNC (VncAuth, None, default TLSVnc+VncAuth, TLSVnc-only negative),
// x11vnc (default + RFB 3.3), LibVNCServer, and TightVNC. Unset env → skip.
func TestVNCDocker(t *testing.T) {
	if os.Getenv("YAK_VNC_DOCKER") != "1" && os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_VNC_DOCKER=1 (or YAK_BRUTE_REAL=1) to start VNC fixtures via docker")
	}

	if addr := os.Getenv("YAK_VNC_TEST_ADDRESS"); addr != "" {
		pass := os.Getenv("YAK_VNC_TEST_PASSWORD")
		if pass == "" {
			pass = "VncPass123!"
		}
		assertVNCLogin(t, "prestarted", addr, pass)
		return
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI not found: %v", err)
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker daemon not reachable (check DOCKER_HOST / context): %v\n%s", err, out)
	}

	dir := vncTestdataDir(t)

	t.Run("tigervnc-vncauth", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tigervnc", "yak-vnc-tigervnc:test")
		addr := runVNCContainer(t, "yak-vnc-tigervnc:test", []string{"VNC_PASSWORD=VncPass123!", "VNC_SECURITY=VncAuth"})
		assertVNCLogin(t, "tigervnc", addr, "VncPass123!")
	})
	t.Run("tigervnc-none", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tigervnc", "yak-vnc-tigervnc:test")
		addr := runVNCContainer(t, "yak-vnc-tigervnc:test", []string{"VNC_SECURITY=None"})
		waitVNCAuth(t, addr, "")
		assertProbe(t, "none-empty", mockProbe(t, "vnc", addr, "", ""), true, false)
		assertProbe(t, "none-any", mockProbe(t, "vnc", addr, "", "ignored"), true, false)
	})
	t.Run("tigervnc-default-tlsvnc-plus-vncauth", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tigervnc", "yak-vnc-tigervnc:test")
		addr := runVNCContainer(t, "yak-vnc-tigervnc:test", []string{"VNC_PASSWORD=VncPass123!", "VNC_SECURITY=Default"})
		assertVNCLogin(t, "tiger-default", addr, "VncPass123!")
	})
	t.Run("tigervnc-tlsvnc-only-unsupported", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tigervnc", "yak-vnc-tigervnc:test")
		addr := runVNCContainer(t, "yak-vnc-tigervnc:test", []string{"VNC_PASSWORD=VncPass123!", "VNC_SECURITY=TLSVnc"})
		waitVNCPort(t, addr)
		res := mockProbe(t, "vnc", addr, "", "VncPass123!")
		if res.Ok {
			t.Fatal("TLSVnc-only must not look like VNC-Auth success")
		}
		if !res.Finished {
			t.Fatal("unsupported security type should finish the target")
		}
	})
	t.Run("tigervnc-eight-byte-pass", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tigervnc", "yak-vnc-tigervnc:test")
		addr := runVNCContainer(t, "yak-vnc-tigervnc:test", []string{"VNC_PASSWORD=12345678", "VNC_SECURITY=VncAuth"})
		assertVNCLogin(t, "eight", addr, "12345678")
	})
	t.Run("x11vnc-rfbauth", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.x11vnc", "yak-vnc-x11vnc:test")
		addr := runVNCContainer(t, "yak-vnc-x11vnc:test", []string{"VNC_PASSWORD=X11VncPass!"})
		assertVNCLogin(t, "x11vnc", addr, "X11VncPass!")
	})
	t.Run("x11vnc-rfb33", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.x11vnc", "yak-vnc-x11vnc:test")
		addr := runVNCContainer(t, "yak-vnc-x11vnc:test", []string{"VNC_PASSWORD=X11VncPass!", "RFB_VERSION=3.3"})
		assertVNCLogin(t, "x11vnc33", addr, "X11VncPass!")
	})
	t.Run("libvncserver", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.libvnc", "yak-vnc-libvnc:test")
		addr := runVNCContainer(t, "yak-vnc-libvnc:test", []string{"VNC_PASSWORD=LibVncPass!"})
		assertVNCLogin(t, "libvnc", addr, "LibVncPass!")
	})
	t.Run("libvncserver-none", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.libvnc", "yak-vnc-libvnc:test")
		addr := runVNCContainer(t, "yak-vnc-libvnc:test", []string{"VNC_SECURITY=None"})
		waitVNCAuth(t, addr, "")
		assertProbe(t, "libvnc-none", mockProbe(t, "vnc", addr, "", ""), true, false)
	})
	t.Run("tightvnc", func(t *testing.T) {
		buildVNCImage(t, dir, "Dockerfile.tightvnc", "yak-vnc-tightvnc:test")
		addr := runVNCContainer(t, "yak-vnc-tightvnc:test", nil)
		assertVNCLogin(t, "tightvnc", addr, "TightPass")
	})
}

func assertVNCLogin(t *testing.T, name, addr, pass string) {
	t.Helper()
	waitVNCAuth(t, addr, pass)
	assertProbe(t, name+"-correct", mockProbe(t, "vnc", addr, "", pass), true, false)
	assertProbe(t, name+"-wrong", mockProbe(t, "vnc", addr, "", "WRONG-PASSWORD"), false, false)
	assertProbe(t, name+"-empty", mockProbe(t, "vnc", addr, "", ""), false, false)
	assertProbe(t, name+"-user-ignored", mockProbe(t, "vnc", addr, "administrator", pass), true, false)
	if len(pass) > 8 {
		assertProbe(t, name+"-first8", mockProbe(t, "vnc", addr, "", pass[:8]), true, false)
	}
	const n = 4
	var wg sync.WaitGroup
	errCh := make(chan string, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			res := mockProbe(t, "vnc", addr, "", pass)
			if !res.Ok {
				errCh <- "concurrent correct failed"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
	wrong := mockProbe(t, "vnc", addr, "", "WRONG-PASSWORD")
	if wrong.Ok {
		t.Fatal("wrong password succeeded after concurrent hits")
	}
	again := mockProbe(t, "vnc", addr, "", pass)
	if !again.Ok {
		t.Fatal("correct password failed after concurrent + wrong")
	}
}

func vncTestdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(file), "testdata", "vnc")
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func buildVNCImage(t *testing.T, dir, dockerfile, tag string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", filepath.Join(dir, dockerfile), dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker build %s: %v\n%s", tag, err, out)
	}
}

func runVNCContainer(t *testing.T, image string, env []string) string {
	t.Helper()
	args := []string{"run", "-d", "-P"}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, image)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	id := strings.TrimSpace(string(out))
	if err != nil || id == "" {
		t.Fatalf("docker run %s: %v\n%s", image, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", id).Run()
	})
	inspect, _ := exec.Command("docker", "inspect", "-f", "{{.State.Status}} {{.State.ExitCode}} {{.State.Error}}", id).CombinedOutput()
	portOut, err := exec.Command("docker", "port", id, "5900/tcp").CombinedOutput()
	if err != nil {
		logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
		t.Fatalf("docker port: %v\nport:%s\ninspect:%s\nlogs:\n%s", err, portOut, inspect, logs)
	}
	hostPort, err := parseDockerPublishedPort(string(portOut))
	if err != nil {
		t.Fatal(err)
	}
	return net.JoinHostPort("127.0.0.1", hostPort)
}

func parseDockerPublishedPort(out string) (string, error) {
	line := strings.TrimSpace(strings.Split(out, "\n")[0])
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return strings.TrimSpace(line[i+1:]), nil
	}
	return "", fmt.Errorf("unrecognized docker port output %q", out)
}

func waitVNCAuth(t *testing.T, addr, pass string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var lastOK, lastFinished bool
	for time.Now().Before(deadline) {
		res := mockProbe(t, "vnc", addr, "", pass)
		lastOK, lastFinished = res.Ok, res.Finished
		if res.Ok {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s VNC login not ready (pass set=%v): last ok=%v finished=%v", addr, pass != "", lastOK, lastFinished)
}

func waitVNCPort(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		last = err
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s not listening: %v", addr, last)
}
