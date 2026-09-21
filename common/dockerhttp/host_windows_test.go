//go:build windows

package dockerhttp_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/Microsoft/go-winio"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestWindowsDefaultDockerHost(t *testing.T) {
	if dockerhttp.DefaultDockerHost != "npipe:////./pipe/docker_engine" {
		t.Fatalf("DefaultDockerHost=%q", dockerhttp.DefaultDockerHost)
	}
}

func TestWindowsHostFromEnvDefault(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "")
	os.Unsetenv(dockerhttp.EnvOverrideHost)
	got := dockerhttp.HostFromEnv()
	if got != "npipe:////./pipe/docker_engine" {
		t.Fatalf("HostFromEnv=%q", got)
	}
}

func TestWindowsNewNpipeClientCompiles(t *testing.T) {
	c, err := dockerhttp.New(dockerhttp.WithHost(dockerhttp.DefaultDockerHost))
	if err != nil {
		t.Fatalf("New npipe: %v", err)
	}
	defer c.Close()
	if c.DaemonHost() != dockerhttp.DefaultDockerHost {
		t.Fatalf("host=%s", c.DaemonHost())
	}
}

func TestWindowsNamedPipeHTTPAndCancellation(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\yak-dockerhttp-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := winio.ListenPipe(pipe, nil)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.44")
			return
		}
		entered <- struct{}{}
		<-r.Context().Done()
	})}
	go server.Serve(listener)
	defer server.Close()
	c, err := dockerhttp.New(dockerhttp.WithHost("npipe://" + strings.ReplaceAll(pipe, `\`, "/")))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.ImageList(canceled, dockerhttp.ImageListOptions{}); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("pipe request never arrived")
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("pipe request did not cancel")
	}
}
