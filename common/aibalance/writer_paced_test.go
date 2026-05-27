package aibalance

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// pacedTestConn 是一个轻量 io.WriteCloser，把所有写入累积到 buf 内，
// 用于在不开真实 socket 的情况下验证 writer 行为。
// 关键词: pacedTestConn, 内存 conn 替身
type pacedTestConn struct {
	buf bytes.Buffer
}

func (c *pacedTestConn) Write(p []byte) (int, error) { return c.buf.Write(p) }
func (c *pacedTestConn) Close() error                { return nil }

// parseSSEDeltaContents 从 SSE chunked transfer 字节流里提取所有
// content / reasoning_content delta 文本，按顺序返回。
// 解析时跳过 chunked encoding 的 size 行，按 "data: <json>\n\n" 提取并 unmarshal。
// 关键词: parseSSEDeltaContents, 流式响应解析助手
func parseSSEDeltaContents(t *testing.T, raw []byte) []string {
	t.Helper()
	// 提取所有 "data: <json>" 行
	re := regexp.MustCompile(`(?m)^data: (\{.*\})$`)
	matches := re.FindAllSubmatch(raw, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		var obj struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(m[1], &obj); err != nil {
			continue
		}
		if len(obj.Choices) == 0 {
			continue
		}
		if obj.Choices[0].Delta.Content != "" {
			out = append(out, obj.Choices[0].Delta.Content)
		}
		if obj.Choices[0].Delta.Reasoning != "" {
			out = append(out, obj.Choices[0].Delta.Reasoning)
		}
	}
	return out
}

// TestPacedWrite_NoLimit_SingleFrame 没启用 TPS 限速时，一次 Write 应该只产生一帧 SSE。
// 关键词: pacedWrite no limit, 单帧
func TestPacedWrite_NoLimit_SingleFrame(t *testing.T) {
	conn := &pacedTestConn{}
	w := NewChatJSONChunkWriterEx(conn, "uid-no-limit", "test-model", false)
	defer func() {
		_ = w.Close()
		w.Wait()
	}()

	ow := w.GetOutputWriter()
	text := "你好世界 hello"
	n, err := ow.Write([]byte(text))
	assert.NoError(t, err)
	assert.Equal(t, len(text), n)

	// 等待 close 把内容 flush 出来再校验
	_ = w.Close()
	w.Wait()

	frames := parseSSEDeltaContents(t, conn.buf.Bytes())
	assert.Equal(t, []string{text}, frames, "no-limit path should emit exactly one delta frame")
}

// TestPacedWrite_TPSLimit_MultipleFramesAndDuration 启用 TPS 限速时：
//  1. 客户端应该收到 > 1 个 SSE delta 帧（被涓滴拆分）
//  2. 所有帧拼起来必须 = 原文（一个字节不丢、UTF-8 完整）
//  3. 总耗时大致符合 tokens / TPS（允许较宽松的下界，避免在 CI 抖动下假阳性）
//
// 关键词: pacedWrite 多帧 + 总耗时, UTF-8 完整性集成验证
func TestPacedWrite_TPSLimit_MultipleFramesAndDuration(t *testing.T) {
	conn := &pacedTestConn{}
	w := NewChatJSONChunkWriterEx(conn, "uid-paced", "test-model", false)
	defer func() {
		_ = w.Close()
		w.Wait()
	}()

	const tps int64 = 20
	w.SetOutputTPSLimit(tps)

	// 文本里混合 ASCII 与中文，断言切分严格 rune 对齐
	text := strings.Repeat("你好world", 6) // 6 * (2 中文 + 5 ASCII) = 42 字符
	start := time.Now()
	ow := w.GetOutputWriter()
	n, err := ow.Write([]byte(text))
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Equal(t, len(text), n)

	_ = w.Close()
	w.Wait()

	frames := parseSSEDeltaContents(t, conn.buf.Bytes())
	assert.Greater(t, len(frames), 1,
		"TPS-limited write should produce multiple SSE delta frames, got %d", len(frames))

	joined := strings.Join(frames, "")
	assert.Equal(t, text, joined, "joined frames must equal original text")
	assert.True(t, utf8.Valid([]byte(joined)), "joined text must be valid UTF-8")

	// 至少 1/3 的预期耗时已发生过 sleep；不强测上界，避免 CI 噪声
	// （ytoken 估算 token 数可能比直觉略少，下界给一半保险）
	if elapsed < 200*time.Millisecond {
		t.Logf("paced write elapsed %v (text=%q tps=%d) – validate manually", elapsed, text, tps)
	}
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond,
		"paced write should take noticeable time under TPS limit; got %v", elapsed)
}

// TestPacedWrite_CJKOnly_NeverSplitsCharacters 纯中文输入下，所有帧拼接后必须
// 等于原文，并且每个 frame 的内容本身就是 valid UTF-8（绝不把汉字切碎）。
// 关键词: pacedWrite CJK never split utf8
func TestPacedWrite_CJKOnly_NeverSplitsCharacters(t *testing.T) {
	conn := &pacedTestConn{}
	w := NewChatJSONChunkWriterEx(conn, "uid-cjk", "test-model", false)
	defer func() {
		_ = w.Close()
		w.Wait()
	}()
	w.SetOutputTPSLimit(50)

	text := "天行健君子以自强不息地势坤君子以厚德载物"
	ow := w.GetOutputWriter()
	_, err := ow.Write([]byte(text))
	assert.NoError(t, err)

	_ = w.Close()
	w.Wait()

	frames := parseSSEDeltaContents(t, conn.buf.Bytes())
	assert.NotEmpty(t, frames)
	joined := strings.Join(frames, "")
	assert.Equal(t, text, joined)
	for i, f := range frames {
		assert.True(t, utf8.ValidString(f), "frame %d %q must be valid utf8", i, f)
	}
}

// TestPacedWrite_NotStreamMode 非流式模式下应保持原行为：
// 内容只累积到 buf，不产生任何 SSE 帧。
// 关键词: pacedWrite notStream skip
func TestPacedWrite_NotStreamMode(t *testing.T) {
	conn := &pacedTestConn{}
	w := NewChatJSONChunkWriterEx(conn, "uid-not-stream", "test-model", true)
	w.SetOutputTPSLimit(20)

	ow := w.GetOutputWriter()
	_, err := ow.Write([]byte("hello 世界"))
	assert.NoError(t, err)

	// 不调用 Close（非流式由 server.go 主动 GetNotStreamBody）
	// 验证：buf 里没有任何 SSE 帧
	assert.NotContains(t, conn.buf.String(), "data: ")
	assert.Equal(t, "hello 世界", ow.buf.String(),
		"non-stream mode should accumulate to internal buf only")
}

// pacedTestDevNull 是一个废弃接收方，仅供基准测试使用。
type pacedTestDevNull struct{}

func (pacedTestDevNull) Write(p []byte) (int, error) { return len(p), nil }
func (pacedTestDevNull) Close() error                { return nil }

// 防止 io 包被 "unused" 报错
var _ io.WriteCloser = pacedTestDevNull{}
