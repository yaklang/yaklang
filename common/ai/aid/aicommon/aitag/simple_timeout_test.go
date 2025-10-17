package aitag

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// TestSimpleTimeoutFlush 测试超时flush机制的基本功能
func TestSimpleTimeoutFlush(t *testing.T) {
	var pr, pw = utils.NewPipe()
	var cb = utils.NewCondBarrier()
	b1 := cb.CreateBarrier("first")
	b2 := cb.CreateBarrier("second")

	go func() {
		defer pw.Close()
		pw.WriteString("<|TEST_0|>")
		pw.WriteString("first_line\n") // 这行以换行符结尾，通常会被保留
		cb.Wait("first")

		// 等待超过超时时间，强制触发flush
		time.Sleep(300 * time.Millisecond)

		pw.WriteString("second_line\n")
		cb.Wait("second")
		pw.WriteString("<|TEST_END_0|>")
	}()

	var chunks []string
	var timestamps []time.Time
	startTime := time.Now()
	finished := false

	_ = Parse(pr, WithCallback("TEST", "0", func(reader io.Reader) {
		// 读取第一个chunk - 应该在超时后收到
		buf := make([]byte, 100)
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("读取第一个chunk失败: %v", err)
		}
		if n > 0 {
			chunk := string(buf[:n])
			chunks = append(chunks, chunk)
			timestamps = append(timestamps, time.Now())
			t.Logf("收到第一个chunk: %q", chunk)
		}
		b1.Done()

		// 读取第二个chunk
		buf = make([]byte, 100)
		n, err = reader.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("读取第二个chunk失败: %v", err)
		}
		if n > 0 {
			chunk := string(buf[:n])
			chunks = append(chunks, chunk)
			timestamps = append(timestamps, time.Now())
			t.Logf("收到第二个chunk: %q", chunk)
		}
		b2.Done()

		// 读取剩余内容
		rest, _ := io.ReadAll(reader)
		if len(rest) > 0 {
			chunks = append(chunks, string(rest))
			timestamps = append(timestamps, time.Now())
			t.Logf("收到剩余内容: %q", string(rest))
		}

		finished = true
	}))

	if !finished {
		t.Fatal("测试未完成")
	}

	// 验证我们收到了多个chunks（表明是流式的）
	if len(chunks) < 2 {
		t.Errorf("期望至少2个chunks，实际收到: %d", len(chunks))
		for i, chunk := range chunks {
			t.Logf("Chunk %d: %q", i, chunk)
		}
	}

	// 验证时间间隔
	if len(timestamps) >= 2 {
		interval := timestamps[1].Sub(timestamps[0])
		if interval < 250*time.Millisecond {
			t.Errorf("Chunks到达间隔太短 (%v)，可能不是真正的流式处理", interval)
		} else {
			t.Logf("流式处理验证成功，间隔: %v", interval)
		}
	}

	// 验证内容完整性
	fullContent := strings.Join(chunks, "")
	if !strings.Contains(fullContent, "first_line") {
		t.Errorf("内容应该包含 'first_line'")
	}
	if !strings.Contains(fullContent, "second_line") {
		t.Errorf("内容应该包含 'second_line'")
	}

	totalTime := time.Since(startTime)
	t.Logf("测试完成，总时间: %v, 收到 %d 个chunks", totalTime, len(chunks))
}

// TestTimeoutFlushWithoutBarrier 不使用barrier的简单超时测试
func TestTimeoutFlushWithoutBarrier(t *testing.T) {
	input := `<|CODE_0|>
line1
line2
line3
<|CODE_END_0|>`

	var chunks []string
	var timestamps []time.Time
	startTime := time.Now()

	// 使用一个慢速的reader来模拟网络延迟
	slowReader := &slowReader{
		data:     []byte(input),
		position: 0,
		delay:    50 * time.Millisecond, // 减少延迟避免测试超时
	}

	err := Parse(slowReader, WithCallback("CODE", "0", func(reader io.Reader) {
		for {
			buf := make([]byte, 50)
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				chunks = append(chunks, chunk)
				timestamps = append(timestamps, time.Now())
				t.Logf("收到chunk: %q", chunk)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("读取错误: %v", err)
				return
			}
		}
	}))

	if err != nil {
		t.Fatalf("Parse失败: %v", err)
	}

	// 验证收到了多个chunks
	if len(chunks) < 2 {
		t.Errorf("期望多个chunks，实际收到: %d", len(chunks))
	}

	totalTime := time.Since(startTime)
	t.Logf("测试完成，总时间: %v, 收到 %d 个chunks", totalTime, len(chunks))
}
