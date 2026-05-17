package aicommon

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// newTestPrinter 为测试创建一个不启动后台 flusher 的 printer,
// 这样断言只看 FlushIfActive 触发的输出, 结果可重复。
func newTestPrinter(out *bytes.Buffer) *DebugStreamPrinter {
	p := NewDebugStreamPrinter(out)
	p.flushInterval = 0
	p.idleEvictAfter = 0
	return p
}

// 关键词: DEBUG stream printer test, 同流多次 delta 合并到单行
func TestDebugStreamPrinter_SameStreamSingleLine(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	deltas := []string{"Hello", " ", "world", "!"}
	for _, d := range deltas {
		p.PrintStreamDelta(&schema.AiOutputEvent{
			IsStream:    true,
			EventUUID:   "uuid-aaa",
			NodeId:      "default",
			StreamDelta: []byte(d),
		})
	}
	p.FlushIfActive()

	got := buf.String()
	if strings.Count(got, "[stream|default|uuid-aaa] ") != 1 {
		t.Fatalf("expected exactly one header, got: %q", got)
	}
	if !strings.Contains(got, "Hello world!") {
		t.Fatalf("expected coalesced content, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline after flush, got: %q", got)
	}
}

// 关键词: DEBUG stream printer test, 两条并发流各自合并不再每 token 一行
func TestDebugStreamPrinter_TwoConcurrentStreamsEachCoalesced(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	a := []string{"我", "需要", "构造", "payload"}
	b := []string{"url:", "https://", "example", ".com"}
	// 模拟两条流交错 token, 这是用户上报的场景核心
	for i := 0; i < 4; i++ {
		p.PrintStreamDelta(&schema.AiOutputEvent{
			IsStream:    true,
			EventUUID:   "uuid-aaa",
			NodeId:      "thought",
			StreamDelta: []byte(a[i]),
		})
		p.PrintStreamDelta(&schema.AiOutputEvent{
			IsStream:    true,
			EventUUID:   "uuid-bbb",
			NodeId:      "tool",
			StreamDelta: []byte(b[i]),
		})
	}
	p.FlushIfActive()

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 output lines (one per stream), got %d:\n%s", len(lines), got)
	}
	// 两条流各自的内容必须完整, 且不再被对方切割。
	wantA := "我需要构造payload"
	wantB := "url:https://example.com"
	foundA, foundB := false, false
	for _, line := range lines {
		if strings.HasPrefix(line, "[stream|thought|uuid-aaa] ") && strings.HasSuffix(line, wantA) {
			foundA = true
		}
		if strings.HasPrefix(line, "[stream|tool|uuid-bbb] ") && strings.HasSuffix(line, wantB) {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("each stream should be one full line.\nfoundA=%v foundB=%v\nout:\n%s", foundA, foundB, got)
	}
}

// 关键词: DEBUG stream printer test, 内嵌换行被转义不破坏行
func TestDebugStreamPrinter_EscapeEmbeddedNewlines(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	p.PrintStreamDelta(&schema.AiOutputEvent{
		IsStream:    true,
		EventUUID:   "uuid-aaa",
		NodeId:      "x",
		StreamDelta: []byte("line1\nline2\r\nline3"),
	})
	p.FlushIfActive()

	got := buf.String()
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("expected single trailing newline, got: %q", got)
	}
	if !strings.Contains(got, "line1\\nline2\\nline3") {
		t.Fatalf("embedded newlines should be escaped, got: %q", got)
	}
}

// 关键词: DEBUG stream printer test, 非流事件 / 空 delta 被忽略
func TestDebugStreamPrinter_SkipNonStream(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	p.PrintStreamDelta(&schema.AiOutputEvent{
		IsStream:    false,
		EventUUID:   "uuid-aaa",
		StreamDelta: []byte("should-not-print"),
	})
	p.PrintStreamDelta(&schema.AiOutputEvent{
		IsStream:    true,
		EventUUID:   "uuid-aaa",
		StreamDelta: []byte{},
	})
	p.FlushIfActive()

	if buf.Len() != 0 {
		t.Fatalf("expected no output for non-stream / empty delta, got: %q", buf.String())
	}
}

// 关键词: DEBUG stream printer test, 并发流写入串行化无字节交错
func TestDebugStreamPrinter_ConcurrentSerialized(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			uuid := "uuid-" + string(rune('a'+idx))
			for j := 0; j < 50; j++ {
				p.PrintStreamDelta(&schema.AiOutputEvent{
					IsStream:    true,
					EventUUID:   uuid,
					NodeId:      "default",
					StreamDelta: []byte("x"),
				})
			}
		}()
	}
	wg.Wait()
	p.FlushIfActive()

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// 4 条流, 每条独占一行
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (one per stream) got %d:\n%s", len(lines), got)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "[stream|") {
			t.Fatalf("line missing header (byte-level interleave?): %q\n%s", line, got)
		}
		// 每条流应该恰好 50 个 'x'
		idx := strings.LastIndex(line, "] ")
		if idx < 0 {
			t.Fatalf("malformed header on line: %q", line)
		}
		content := line[idx+2:]
		if content != strings.Repeat("x", 50) {
			t.Fatalf("stream content tampered, got %q in line %q", content, line)
		}
	}
}

// 关键词: DEBUG stream printer test, 后台 flusher 自动 flush
func TestDebugStreamPrinter_BackgroundFlusher(t *testing.T) {
	buf := new(bytes.Buffer)
	p := NewDebugStreamPrinter(buf)
	p.flushInterval = 30 * time.Millisecond
	defer p.Stop()

	// 写一次, 然后等待后台 flusher 主动 flush
	p.PrintStreamDelta(&schema.AiOutputEvent{
		IsStream:    true,
		EventUUID:   "uuid-aaa",
		NodeId:      "n",
		StreamDelta: []byte("AAA"),
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if buf.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := buf.String()
	if !strings.Contains(got, "[stream|n|uuid-aaa] AAA") {
		t.Fatalf("background flusher should have written content, got: %q", got)
	}
}

// TestDebugStreamPrinter_WrapLogWriterFlushesStreamFirst 验证
// WrapLogWriter 包装后的 io.Writer 会先 flush 流再写日志, 消除"夹心"。
// 关键词: WrapLogWriter 防夹心 日志先 flush 流
func TestDebugStreamPrinter_WrapLogWriterFlushesStreamFirst(t *testing.T) {
	buf := new(bytes.Buffer)
	p := newTestPrinter(buf)

	// 给流写入一些内容, 不主动 flush
	p.PrintStreamDelta(&schema.AiOutputEvent{
		IsStream:    true,
		EventUUID:   "uuid-aaa",
		NodeId:      "n",
		StreamDelta: []byte("stream-content"),
	})
	// 此时 buf 应该还是空的（flushInterval=0 不会自动 flush）
	if buf.Len() != 0 {
		t.Fatalf("buf should be empty before log write, got: %q", buf.String())
	}

	// 模拟日志写入
	wrapped := p.WrapLogWriter(buf)
	_, _ = wrapped.Write([]byte("[INFO] log line\n"))

	got := buf.String()
	// 流先 flush, 日志后写入
	streamIdx := strings.Index(got, "stream-content")
	logIdx := strings.Index(got, "[INFO] log line")
	if streamIdx < 0 || logIdx < 0 {
		t.Fatalf("missing expected content: %q", got)
	}
	if streamIdx > logIdx {
		t.Fatalf("stream should be flushed BEFORE log line, got: %q", got)
	}
	// 验证流是完整一行（包含换行）然后日志在新行
	if !strings.Contains(got, "stream-content\n[INFO] log line") {
		t.Fatalf("stream and log should be on separate lines: %q", got)
	}
}
