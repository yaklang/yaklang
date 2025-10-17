package tools

import (
	"context"
	"testing"
	"time"
)

func Test__pingScan(t *testing.T) {
	t.Skip("跳过测试：依赖外部网络 1.1.1.1/24，不符合测试不外连的原则")

	ctx, cancel := context.WithCancel(context.Background())
	res := _pingScan(
		"1.1.1.1/24",
		_pingConfigOpt_tcpPingPorts("80,443,22"),
		_pingConfigOpt_withTimeout(5),
		_pingConfigOpt_concurrent(20),
		WithPingCtx(ctx),
	)
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()
	for r := range res {
		if r.Ok {
			t.Log(r.IP)
		}
	}
	time.Sleep(5 * time.Second)
}

func Test__skipPingScan2(t *testing.T) {
	t.Skip("跳过测试：依赖外部网络 1.1.1.1/24，不符合测试不外连的原则")

	ctx, cancel := context.WithCancel(context.Background())
	res := _pingScan(
		"1.1.1.1/24",
		_pingConfigOpt_tcpPingPorts("80,443,22"),
		_pingConfigOpt_withTimeout(5),
		_pingConfigOpt_concurrent(20),
		WithPingCtx(ctx),
		_pingConfigOpt_skipped(true),
	)
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()
	for r := range res {
		if r.Ok {
			time.Sleep(100 * time.Millisecond)
			t.Log(r.IP)
		}
	}
	time.Sleep(5 * time.Second)
}
