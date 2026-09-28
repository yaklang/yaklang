//go:build integration
// +build integration

package sgnds

import (
	"context"
	"testing"
	"time"
)

// TestEnsureBridgeRunning 验证从 Go 二进制内部释放 jar 并启动代理。
func TestEnsureBridgeRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	addr, err := EnsureBridgeRunning(ctx)
	if err != nil {
		t.Fatalf("ensure bridge failed: %v", err)
	}
	t.Logf("bridge running at %s", addr)

	c := NewBridgeClient(addr, 5*time.Second)
	ok, err := c.Ping(ctx)
	if err != nil || !ok {
		t.Fatalf("ping bridge failed: ok=%v err=%v", ok, err)
	}

	// 本地没有隔离装置，open 应该失败，但证明驱动已加载。
	connID, err := c.Open(ctx, "jdbc:nds://127.0.0.1:18600/v_test?appname=yakit_embedded_test", "user", "pass")
	if err == nil {
		_ = c.Close(ctx, connID)
		t.Fatal("expected open to fail without isolation device")
	}
	t.Logf("expected open failure: %v", err)

	if bridgeLauncher != nil {
		_ = bridgeLauncher.Stop()
	}
}
