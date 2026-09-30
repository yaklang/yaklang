package test

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"io"
	"net"
	"strings"
	"sync"
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

// TestMUSTPASS_ScanPortTool_ActivatesTunSafeMode 验证 TUN/Fake-IP 的
// 198.18.0.0/15 目标不会被整体拒绝，而是把大范围 SYN 计划收缩为
// 少量 TCP connect 候选验证，并输出 AI TODO 与后续应用层处置指引.
//
// 关键词: scan_port TUN safe mode 回归, SCAN_TUN_SAFE_MODE,
// 198.18.0.0/15 大规模端口误报, AI_TODO_REQUIRED
func TestMUSTPASS_ScanPortTool_ActivatesTunSafeMode(t *testing.T) {
	exports := map[string]any{}
	for key, value := range tools.FingerprintScanExports {
		exports[key] = value
	}
	calls := 0
	exports["Scan"] = func(hosts, ports string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
		calls++
		assert.Equal(t, hosts, "198.18.215.229")
		assert.Equal(t, ports, "22,80,443,445,3306,3389,5432,6379,8000,8080,8443,9000")
		results := make(chan *fp.MatchResult)
		close(results)
		return results, nil
	}
	_, combined := executeFixtureScript(t, "yakscriptforai/pentest/scan_port.yak", aitool.InvokeParams{"hosts": "198.18.215.229", "ports": "1-65535", "mode": "auto"}, map[string]any{"servicescan": exports})
	assert.Equal(t, calls, 1, "planning must start exactly one compact TCP scan")

	assert.Assert(t, strings.Contains(combined, "[SCAN_TUN_SAFE_MODE]"),
		"expected machine-readable TUN-safe-mode marker:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "effective-mode=tcp"),
		"expected automatic TCP-connect fallback:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "effective-ports=22,80,443,445,3306,3389,5432,6379,8000,8080,8443,9000"),
		"expected broad request to be reduced to compact operational ports:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "[AI_TODO_REQUIRED]"),
		"expected explicit AI TODO instruction:\n%s", combined)
	assert.Assert(t, !strings.Contains(combined, "starting SYN scan"),
		"SYN scan must not start for a TUN/Fake-IP target:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "starting TCP connect scan"),
		"compact TCP scan should replace the broad SYN plan:\n%s", combined)
}

// TestMUSTPASS_ScanPortTool_CancelStopsTcpScanFast 端到端验证: 当 AI 插件 context
// 被取消时, scan_port 的 TCP 扫描能借助注入的 CTX 迅速停止, 而不是把对一个 tarpit
// 主机的全部端口扫完.
//
// 构造: 本地 tarpit 在收到实际 probe 字节后通知测试取消，断言
// Callback 在 3s 内结束；不依赖解析/调度速度或随机固定 sleep。
//
// 关键词: scan_port 端到端取消, CTX 传播到 servicescan, tarpit 资源泄漏防护
func TestMUSTPASS_ScanPortTool_CancelStopsTcpScanFast(t *testing.T) {
	host, port, probing := startLocalTarpit(t)
	tool := getScanPortTool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var out, stderr strings.Builder
		_, _ = tool.Callback(ctx, aitool.InvokeParams{"hosts": host, "ports": fmt.Sprint(port), "mode": "tcp", "concurrent": 1, "active": true, "web": true}, nil, &out, &stderr)
	}()
	select {
	case <-probing:
	case <-done:
		t.Fatal("scan finished without starting an application probe")
	case <-time.After(3 * time.Second):
		t.Fatal("local application probe never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop the in-flight probe")
	}
}

// Read the first probe byte so cancellation happens during the real scan, not
// during script parsing or a TCP readiness connection. Cleanup joins all workers.
func startLocalTarpit(t *testing.T) (host string, port int, probing <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	addr := ln.Addr().(*net.TCPAddr)
	started := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var workers sync.WaitGroup
	connections := map[net.Conn]struct{}{}
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[conn] = struct{}{}
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				b := make([]byte, 1)
				if _, err := io.ReadFull(conn, b); err == nil {
					once.Do(func() { close(started) })
				}
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-acceptDone
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		workers.Wait()
	})
	return addr.IP.String(), addr.Port, started
}
