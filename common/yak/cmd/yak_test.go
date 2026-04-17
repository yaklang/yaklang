package main

import (
	"sync/atomic"
	"testing"
	"time"
)

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
