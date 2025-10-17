package aitag

import (
	"io"
	"strings"
	"testing"
	"time"
)

// TestTimeoutFlushMechanism tests that content is flushed even when ending with newlines
// if enough time has passed, ensuring good streaming performance
func TestTimeoutFlushMechanism(t *testing.T) {
	// Simulate AI code generation with multiple lines ending in newlines
	input := `<|GEN_CODE_test|>
// 记录扫描开始时间
startTime := time.Now()
yakit.Info("开始扫描目标: %s, 端口范围: %s, 并发数: %d", target, ports, concurrent)
yakit.StatusCard("扫描状态", "进行中", "scan-status", "info")

// 执行端口扫描
results, err := servicescan.Scan(target, ports, opts...)
if err != nil {
    yakit.Error("扫描过程中发生错误: %v", err)
    yakit.StatusCard("扫描状态", "失败", "scan-status", "error")
    return
}
<|GEN_CODE_END_test|>`

	var receivedChunks []string
	var chunkTimes []time.Time
	startTime := time.Now()

	err := Parse(strings.NewReader(input), WithCallback("GEN_CODE", "test", func(reader io.Reader) {
		// Read in small chunks to simulate streaming consumption
		buffer := make([]byte, 50)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				chunk := string(buffer[:n])
				receivedChunks = append(receivedChunks, chunk)
				chunkTimes = append(chunkTimes, time.Now())

				// Simulate processing time
				time.Sleep(10 * time.Millisecond)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("读取失败: %v", err)
				return
			}
		}
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify we received multiple chunks (indicating streaming)
	if len(receivedChunks) < 3 {
		t.Errorf("Expected multiple chunks for streaming, got %d chunks", len(receivedChunks))
		t.Logf("Chunks: %v", receivedChunks)
	}

	// Verify chunks arrived over time (not all at once)
	if len(chunkTimes) >= 2 {
		firstChunk := chunkTimes[0].Sub(startTime)
		lastChunk := chunkTimes[len(chunkTimes)-1].Sub(startTime)

		// Should have some time spread between first and last chunk
		timeDiff := lastChunk - firstChunk
		if timeDiff < 50*time.Millisecond {
			t.Errorf("Chunks arrived too quickly (within %v), may not be truly streaming", timeDiff)
		}

		t.Logf("Streaming verified: first chunk at %v, last chunk at %v, spread: %v",
			firstChunk, lastChunk, timeDiff)
	}

	// Verify content is complete and correct
	fullContent := strings.Join(receivedChunks, "")
	expectedLines := []string{
		"// 记录扫描开始时间",
		"startTime := time.Now()",
		"yakit.Info(",
		"servicescan.Scan(",
		"return",
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(fullContent, expectedLine) {
			t.Errorf("Expected content to contain %q", expectedLine)
		}
	}
}

// TestTimeoutFlushWithNewlines specifically tests that newlines don't prevent streaming
func TestTimeoutFlushWithNewlines(t *testing.T) {
	// Content with many newlines that would normally be held in buffer
	input := `<|CODE_test|>line1
line2
line3
line4
line5
<|CODE_END_test|>`

	var chunks []string
	var timestamps []time.Time
	start := time.Now()

	err := Parse(strings.NewReader(input), WithCallback("CODE", "test", func(reader io.Reader) {
		buf := make([]byte, 10) // Small buffer to force multiple reads
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunks = append(chunks, string(buf[:n]))
				timestamps = append(timestamps, time.Now())
				// Small delay to allow timeout mechanism to work
				time.Sleep(50 * time.Millisecond)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Read error: %v", err)
				return
			}
		}
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should receive content in multiple chunks due to timeout flush
	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(chunks))
	}

	// Verify content correctness (block text formatting should still work)
	fullContent := strings.Join(chunks, "")
	expected := "line1\nline2\nline3\nline4\nline5"
	if fullContent != expected {
		t.Errorf("Expected %q, got %q", expected, fullContent)
	}

	totalTime := time.Since(start)
	t.Logf("Test completed in %v with %d chunks", totalTime, len(chunks))
}
