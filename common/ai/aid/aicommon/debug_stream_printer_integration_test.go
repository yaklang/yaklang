package aicommon

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

// TestDebugStreamPrinter_RealLifeInterleaveScenario 复现用户上报的"并发两条流 +
// 日志交织"的真实场景:
//   - 两条流的 token 交替到达
//   - 中间还有 log.Info 输出
//
// 期望：每条流各占一整行, 日志独占一行, 不再出现"一行一个 token"刷屏。
//
// 关键词: DEBUG=1 流式打印体验, 并发流交织, FlushIfActive 防止夹心
func TestDebugStreamPrinter_RealLifeInterleaveScenario(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)
	logWriter := p.WrapLogWriter(buf)

	// 两条流交替写, token 粒度细到 1~5 字节, 模拟真实 AI 流。
	thoughtDeltas := []string{
		"我", "需要", "构造", " AND ", "'1'='2'", " 的", "布尔", "盲注", "payload",
	}
	toolDeltas := []string{
		"{\"url\":", "\"https://", "id.redh", "aze.top/api", "/portal/auth/", "enterprise/login\"}",
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, d := range thoughtDeltas {
			p.PrintStreamDelta(&schema.AiOutputEvent{
				IsStream:    true,
				EventUUID:   "uuid-thought",
				NodeId:      "re-act-loop-thought",
				StreamDelta: []byte(d),
			})
		}
	}()
	go func() {
		defer wg.Done()
		for _, d := range toolDeltas {
			p.PrintStreamDelta(&schema.AiOutputEvent{
				IsStream:    true,
				EventUUID:   "uuid-tool",
				NodeId:      "directly_call_tool",
				StreamDelta: []byte(d),
			})
		}
	}()
	wg.Wait()

	// 现在写一条"日志"——会自动 flush 流缓冲, 让日志独占行。
	_, _ = logWriter.Write([]byte("[INFO] some log line\n"))

	// 再来第二轮 token, 应该开新的流行（流尾被 flush 后再次累积）。
	for _, d := range []string{"continued ", "after ", "log"} {
		p.PrintStreamDelta(&schema.AiOutputEvent{
			IsStream:    true,
			EventUUID:   "uuid-thought",
			NodeId:      "re-act-loop-thought",
			StreamDelta: []byte(d),
		})
	}
	p.FlushIfActive()

	out := buf.String()
	t.Logf("captured output:\n%s", out)

	// 校验：所有非空行必须是合法 stream 头 或 [INFO] 日志, 不应有"裸 token 行"
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		isStreamLine := strings.HasPrefix(line, "[stream|") ||
			strings.HasPrefix(line, "[reason|") ||
			strings.HasPrefix(line, "[system|")
		isLogLine := strings.HasPrefix(line, "[INFO] ")
		if !isStreamLine && !isLogLine {
			t.Fatalf("found orphan / interleaved line: %q\nfull:\n%s", line, out)
		}
	}

	// 第一轮的 thought 必须是连续一段完整文本（不被 tool 流切碎）
	wantThought := "我需要构造 AND '1'='2' 的布尔盲注payload"
	if !strings.Contains(out, wantThought) {
		t.Fatalf("thought stream content was split, expected %q in:\n%s", wantThought, out)
	}
	wantTool := "{\"url\":\"https://id.redhaze.top/api/portal/auth/enterprise/login\"}"
	if !strings.Contains(out, wantTool) {
		t.Fatalf("tool stream content was split, expected %q in:\n%s", wantTool, out)
	}
	// 日志独占一行（前后必须是换行）
	if !strings.Contains(out, "\n[INFO] some log line\n") {
		t.Fatalf("log line should be sandwiched by newlines:\n%s", out)
	}
	// log 之前必须先把两条流 flush 掉, 也就是日志必须在两条流之后
	logIdx := strings.Index(out, "[INFO] some log line")
	thoughtIdx := strings.Index(out, wantThought)
	toolIdx := strings.Index(out, wantTool)
	if logIdx < thoughtIdx || logIdx < toolIdx {
		t.Fatalf("log should appear after both streams have been flushed:\n%s", out)
	}
	// log 之后第二轮 thought 应该出现新的 thought 行
	tailIdx := strings.Index(out, "continued after log")
	if tailIdx < logIdx {
		t.Fatalf("second-round thought should appear after log:\n%s", out)
	}
}
