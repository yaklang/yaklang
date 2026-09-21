package dockerhttp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestDialMissingSocket(t *testing.T) {
	c, err := dockerhttp.New(
		dockerhttp.WithHost("unix:///no/such/docker.sock"),
		dockerhttp.WithVersion("1.44"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestDialPermission(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "locked")
	if err := os.Mkdir(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	bad := filepath.Join(parent, "docker.sock")
	c, err := dockerhttp.New(dockerhttp.WithHost("unix://"+bad), dockerhttp.WithVersion("1.44"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected permission/dial error")
	}
}

func TestPingCancel(t *testing.T) {
	m := newMockEngine()
	m.PingDelay = 2 * time.Second
	c := startUnixMock(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.Ping(ctx)
	if err == nil {
		t.Fatal("expected cancel error")
	}
}

func TestPingTimeout(t *testing.T) {
	m := newMockEngine()
	m.PingDelay = 2 * time.Second
	c := startUnixMock(t, m)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Ping(ctx)
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestVersionNegotiation(t *testing.T) {
	m := newMockEngine()
	m.APIVersion = "1.40"
	c := startUnixMock(t, m)
	if err := c.NegotiateAPIVersion(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.ClientVersion() != "1.40" {
		t.Fatalf("negotiated=%s want 1.40", c.ClientVersion())
	}
}

func TestVersionNegotiationEmpty(t *testing.T) {
	m := newMockEngine()
	m.RejectNegotiate = true
	c := startUnixMock(t, m)
	err := c.NegotiateAPIVersion(context.Background())
	if err == nil {
		t.Fatal("expected incompatible/empty version error")
	}
	var ve *dockerhttp.VersionIncompatibleError
	if !errors.As(err, &ve) {
		t.Fatalf("want VersionIncompatibleError, got %T %v", err, err)
	}
}

func TestImageInspectOKAnd404(t *testing.T) {
	m := newMockEngine()
	m.Images["alpine:latest"] = dockerhttp.ImageInspect{
		ID:       "sha256:abc",
		RepoTags: []string{"alpine:latest"},
	}
	m.Images["sha256:abc"] = m.Images["alpine:latest"]
	m.Images["alpine@sha256:abc"] = m.Images["alpine:latest"]
	c := startUnixMock(t, m)

	img, err := c.ImageInspect(context.Background(), "alpine:latest")
	if err != nil || img.ID != "sha256:abc" {
		t.Fatalf("inspect: %+v %v", img, err)
	}

	_, err = c.ImageInspect(context.Background(), "missing:tag")
	if !dockerhttp.IsNotFound(err) {
		t.Fatalf("want 404, got %v", err)
	}

	id, ok, err := c.ResolveImageID(context.Background(), "alpine:latest")
	if err != nil || !ok || id != "sha256:abc" {
		t.Fatalf("resolve: %s %v %v", id, ok, err)
	}
	_, ok, err = c.ResolveImageID(context.Background(), "nope")
	if err != nil || ok {
		t.Fatalf("resolve missing: ok=%v err=%v", ok, err)
	}
	id, ok, err = c.ResolveImageID(context.Background(), "alpine@sha256:abc")
	if err != nil || !ok {
		t.Fatalf("digest resolve: %v %v", ok, err)
	}
}

func TestImageInspectEmptyID(t *testing.T) {
	m := newMockEngine()
	m.Images["empty"] = dockerhttp.ImageInspect{ID: ""}
	c := startUnixMock(t, m)
	id, ok, err := c.ResolveImageID(context.Background(), "empty")
	if err != nil || ok || id != "" {
		t.Fatalf("empty id: %q ok=%v err=%v", id, ok, err)
	}
}

func TestImageLoadStream(t *testing.T) {
	m := newMockEngine()
	c := startUnixMock(t, m)

	big := bytes.Repeat([]byte("X"), 256*1024)
	if err := c.ImageLoad(context.Background(), bytes.NewReader(big)); err != nil {
		t.Fatalf("load big: %v", err)
	}
	if m.LoadCalls != 1 {
		t.Fatalf("load calls=%d", m.LoadCalls)
	}

	m.LoadHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		writeErr(w, 500, "load boom")
	}
	err := c.ImageLoad(context.Background(), bytes.NewReader([]byte("tar")))
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("want http error, got %v", err)
	}

	m.LoadHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"Loading layer"}` + "\n"))
		_, _ = w.Write([]byte(`{"errorDetail":{"message":"invalid tar"},"error":"invalid tar"}` + "\n"))
	}
	err = c.ImageLoad(context.Background(), bytes.NewReader([]byte("tar")))
	if err == nil || !strings.Contains(err.Error(), "invalid tar") {
		t.Fatalf("want stream error, got %v", err)
	}

	m.LoadHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	if err := c.ImageLoad(ctx, foreverReader{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("load cancellation: %v", err)
	}
}

type foreverReader struct{}

func (foreverReader) Read(p []byte) (int, error) {
	time.Sleep(10 * time.Millisecond)
	for i := range p {
		p[i] = 'A'
	}
	return len(p), nil
}

var _ = net.Listen

func TestImageLoadTruncatedOK(t *testing.T) {
	m := newMockEngine()
	m.LoadHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		// single JSON object without trailing newline
		_, _ = w.Write([]byte(`{"status":"Loaded image"}`))
	}
	c := startUnixMock(t, m)
	if err := c.ImageLoad(context.Background(), bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
}

func TestPingAndClose(t *testing.T) {
	m := newMockEngine()
	c := startUnixMock(t, m)
	info, err := c.PingInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.APIVersion == "" {
		t.Fatal("empty api version")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestManualVersionSkipsNegotiate(t *testing.T) {
	m := newMockEngine()
	m.APIVersion = "1.30"
	dir, err := os.MkdirTemp("", "dh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "docker.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: m.Handler()}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close(); ln.Close() })
	c, err := dockerhttp.New(
		dockerhttp.WithHost("unix://"+sock),
		dockerhttp.WithVersion("1.44"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Should still talk /v1.44/... even if daemon reports 1.30
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.ClientVersion() != "1.44" {
		t.Fatalf("version=%s", c.ClientVersion())
	}
}
