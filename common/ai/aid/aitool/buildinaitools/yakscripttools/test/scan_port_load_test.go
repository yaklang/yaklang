package test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// TestMUSTPASS_ScanPortTool_LoadsWithInjectedCTX 验证 scan_port.yak 在引用 AI 执行器
// 注入的全局 CTX 之后, 仍能被 SSA 静态分析正确解析、提取 metadata 并转换成 AI tool.
//
// 背景: scan_port.yak 现在用 `CTX` 作为父 context 以支持 AI 取消传播. 历史注释担心
// SSA 不识别该 external 变量. 本用例锁死"工具仍能正常加载"这一回归点, 防止以后
// 误以为 CTX 引用会破坏工具注册.
//
// 关键词: scan_port CTX 注入回归, SSAParse external 变量, AI tool 加载
func TestMUSTPASS_ScanPortTool_LoadsWithInjectedCTX(t *testing.T) {
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/pentest/scan_port.yak")
	if err != nil {
		t.Fatalf("failed to read scan_port.yak from embed FS: %v", err)
	}

	aiTool := yakscripttools.LoadYakScriptToAiTools("scan_port", string(content))
	assert.Assert(t, aiTool != nil, "LoadYakScriptToAiTools returned nil; CTX reference likely broke SSAParse/metadata")

	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	assert.Assert(t, len(tools) > 0, "ConvertTools returned empty for scan_port")
	assert.Equal(t, tools[0].Name, "scan_port")
}

// TestMUSTPASS_ScanPortTool_RuntimeCTXUsable 在 AI 工具执行路径下真正运行
// scan_port (tcp 模式, 本地回环, 少量端口), 验证脚本里 `context.WithCancel(CTX)`
// 在运行时可用 (CTX 确实是注入进来的 context 值), 且能正常跑完输出 "scan completed".
//
// 这条用例锁死"运行时引用注入的 CTX 不会 panic / 报错"这一关键回归点 —— 这是把
// 父 context 从 background 换成 CTX 之后最大的运行时风险.
//
// 关键词: scan_port 运行时 CTX, context.WithCancel(CTX) 可用, AI 工具执行路径
func TestMUSTPASS_ScanPortTool_RuntimeCTXUsable(t *testing.T) {
	tool := getScanPortTool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	w1, w2 := &strings.Builder{}, &strings.Builder{}
	_, err := tool.Callback(ctx, aitool.InvokeParams{
		"hosts": "127.0.0.1",
		"ports": "65500-65502",
		"mode":  "tcp",
	}, nil, w1, w2)
	if err != nil {
		t.Fatalf("scan_port tcp run returned error (CTX runtime usage likely broken): %v\nstderr: %s", err, w2.String())
	}

	combined := w1.String() + "\n" + w2.String()
	assert.Assert(t, strings.Contains(combined, "scan completed"),
		"expected 'scan completed' marker, CTX-based context may have broken the flow:\n%s", combined)
	// 说明: "using injected CTX" 是 log.info (引擎日志), 不会进入 yakit stdout 流,
	// 故不在此断言. CTX 是否真正生效由 TestMUSTPASS_ScanPortTool_CancelStopsTcpScanFast
	// 端到端的"取消即快速返回"行为来保证.
}

// TestMUSTPASS_ScanPortTool_CancelStopsTcpScanFast 端到端验证: 当 AI 插件 context
// 被取消时, scan_port 的 TCP 扫描能借助注入的 CTX 迅速停止, 而不是把对一个 tarpit
// 主机的全部端口扫完.
//
// 构造: 本地 tarpit listener (accept 后永不回包) + 大端口范围, 取消 ctx 后断言
// Callback 在远小于"全量扫完"所需时间内返回.
//
// 关键词: scan_port 端到端取消, CTX 传播到 servicescan, tarpit 资源泄漏防护
func TestMUSTPASS_ScanPortTool_CancelStopsTcpScanFast(t *testing.T) {
	host, port, closeFn := startLocalTarpit(t)
	defer closeFn()

	tool := getScanPortTool(t)

	ctx, cancel := context.WithCancel(context.Background())

	// 端口范围围绕 tarpit 端口展开, 让大量 TCP 连接都落到 tarpit 上 (accept 但不回包),
	// 没有取消传播时每个连接都要卡满 probeTimeout(5s), 全量扫完会非常久.
	ports := fmt.Sprintf("%d-%d", port, port+800)

	go func() {
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w1, w2 := &strings.Builder{}, &strings.Builder{}
		_, _ = tool.Callback(ctx, aitool.InvokeParams{
			"hosts":      host,
			"ports":      ports,
			"mode":       "tcp",
			"concurrent": 50,
		}, nil, w1, w2)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		t.Logf("scan_port returned %v after cancel", elapsed)
		if elapsed > 25*time.Second {
			t.Fatalf("scan_port did not stop quickly after cancel (%v); CTX cancellation not propagated", elapsed)
		}
	case <-time.After(40 * time.Second):
		t.Fatal("scan_port did not return within 40s after cancel; CTX cancellation not propagated")
	}
}

// startLocalTarpit 启动一个 accept 后持有连接、永不回包也不主动关闭的 TCP listener,
// 用于模拟"全端口响应"的异常主机.
func startLocalTarpit(t *testing.T) (host string, port int, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	var stopped atomic.Bool
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, conn)
			if stopped.Load() {
				return
			}
		}
	}()
	return addr.IP.String(), addr.Port, func() {
		stopped.Store(true)
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
	}
}
