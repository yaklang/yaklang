package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteGRPCReadyEvent(t *testing.T) {
	var output bytes.Buffer
	if err := writeGRPCReadyEvent(&output, "127.0.0.1:54321"); err != nil {
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
	if event.SchemaVersion != 1 || event.Address != "127.0.0.1:54321" {
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
