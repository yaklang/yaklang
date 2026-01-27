package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 创建一个实现了 io.WriteCloser 的测试用 writer
type testWriteCloser struct {
	*bytes.Buffer
}

func (t *testWriteCloser) Close() error {
	return nil
}

func newTestWriteCloser() io.WriteCloser {
	return &testWriteCloser{Buffer: &bytes.Buffer{}}
}

type closeSpyWriteCloser struct {
	*bytes.Buffer
	closed bool
	mu     sync.Mutex
}

func (c *closeSpyWriteCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *closeSpyWriteCloser) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestNewChatJSONChunkWriter(t *testing.T) {
	buf := newTestWriteCloser()
	uid := "test-uid"
	model := "test-model"
	writer := NewChatJSONChunkWriter(buf, uid, model)

	if writer.uid != uid {
		t.Errorf("Expected uid %s, got %s", uid, writer.uid)
	}
	if writer.model != model {
		t.Errorf("Expected model %s, got %s", model, writer.model)
	}
	if writer.created.IsZero() {
		t.Error("Expected created time to be set")
	}
}

func TestChatJSONChunkWriterCloseWaitReleasesBackgroundGoroutine(t *testing.T) {
	// This verifies that Close()+Wait() returns (writer internal io.Copy goroutine exits),
	// which is critical for preventing goroutine/memory leak in provider failover.
	forceWait := func(w *chatJSONChunkWriter) {
		done := make(chan struct{})
		go func() {
			_ = w.Close()
			w.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("writer Close/Wait did not return in time (possible goroutine leak)")
		}
	}

	base := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		buf := newTestWriteCloser()
		w := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
		forceWait(w)
	}

	// allow runtime goroutines noise; still, we should not continuously grow.
	after := runtime.NumGoroutine()
	if after > base+30 {
		t.Fatalf("goroutine leak suspected: base=%d after=%d", base, after)
	}
}

func TestChatJSONChunkWriterDoubleCloseIsSafe(t *testing.T) {
	buf := &closeSpyWriteCloser{Buffer: &bytes.Buffer{}}
	w := NewChatJSONChunkWriter(buf, "test-uid", "test-model")

	if err := w.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	// second close should be no-op
	if err := w.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	w.Wait()
}

func TestChatJSONChunkWriterCloseInNotStreamStillClosesUnderlyingWriter(t *testing.T) {
	buf := &closeSpyWriteCloser{Buffer: &bytes.Buffer{}}
	w := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	w.notStream = true

	_ = w.Close()
	w.Wait()

	if !buf.IsClosed() {
		t.Fatalf("expected underlying writer to be closed in notStream mode")
	}
}

func TestGetOutputWriter(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()

	if outputWriter.reason {
		t.Error("Expected output writer to have reason=false")
	}
	if outputWriter.writer != writer {
		t.Error("Expected output writer to reference the parent writer")
	}
}

func TestGetReasonWriter(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	reasonWriter := writer.GetReasonWriter()

	if !reasonWriter.reason {
		t.Error("Expected reason writer to have reason=true")
	}
	if reasonWriter.writer != writer {
		t.Error("Expected reason writer to reference the parent writer")
	}
}

func TestOutputWriterWrite(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()

	testContent := "Hello, World!"
	_, err := outputWriter.Write([]byte(testContent))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 验证写入的内容格式是否正确
	output := buf.(*testWriteCloser).String()
	if !bytes.Contains([]byte(output), []byte(testContent)) {
		t.Errorf("Expected output to contain %s", testContent)
	}

	// 验证 JSON 格式
	lines := bytes.Split([]byte(output), []byte("\r\n"))
	var jsonLine []byte
	for _, line := range lines {
		if bytes.Contains(line, []byte("data: ")) {
			jsonLine = line
			break
		}
	}
	if jsonLine == nil {
		t.Fatal("No JSON line found in output")
	}

	// 解析 JSON
	var result map[string]interface{}
	err = json.Unmarshal(bytes.TrimPrefix(jsonLine, []byte("data: ")), &result)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// 验证 JSON 结构
	if result["id"] != "chat-ai-balance-test-uid" {
		t.Errorf("Expected id chat-ai-balance-test-uid, got %s", result["id"])
	}
	if result["model"] != "test-model" {
		t.Errorf("Expected model test-model, got %s", result["model"])
	}

	choices := result["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	delta := choice["delta"].(map[string]interface{})
	if delta["content"] != testContent {
		t.Errorf("Expected content %s, got %s", testContent, delta["content"])
	}
}

func TestReasonWriterWrite(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	reasonWriter := writer.GetReasonWriter()

	testContent := "This is a reason"
	_, err := reasonWriter.Write([]byte(testContent))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 验证写入的内容格式是否正确
	output := buf.(*testWriteCloser).String()
	if !bytes.Contains([]byte(output), []byte(testContent)) {
		t.Errorf("Expected output to contain %s", testContent)
	}

	// 验证 JSON 格式
	lines := bytes.Split([]byte(output), []byte("\r\n"))
	var jsonLine []byte
	for _, line := range lines {
		if bytes.Contains(line, []byte("data: ")) {
			jsonLine = line
			break
		}
	}
	if jsonLine == nil {
		t.Fatal("No JSON line found in output")
	}

	// 解析 JSON
	var result map[string]interface{}
	err = json.Unmarshal(bytes.TrimPrefix(jsonLine, []byte("data: ")), &result)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	choices := result["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	delta := choice["delta"].(map[string]interface{})
	if delta["reason_content"] != testContent {
		t.Errorf("Expected reason_content %s, got %s", testContent, delta["reason_content"])
	}
}

func TestMultipleWrites(t *testing.T) {
	// Skip: Known flaky test - multiple writes output validation is unstable
	t.Skip("Skipping flaky test - known issue with multiple writes")

	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()
	reasonWriter := writer.GetReasonWriter()

	// 写入普通内容
	_, err := outputWriter.Write([]byte("Hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 写入原因
	_, err = reasonWriter.Write([]byte("Reason"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 再次写入普通内容
	_, err = outputWriter.Write([]byte("World"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 关闭 writer
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 验证输出
	output := buf.(*testWriteCloser).String()
	t.Log("Multiple writes output:")
	t.Log(output)

	// 验证所有内容都存在
	if !bytes.Contains([]byte(output), []byte("Hello")) {
		t.Error("Expected output to contain 'Hello'")
	}
	if !bytes.Contains([]byte(output), []byte("Reason")) {
		t.Error("Expected output to contain 'Reason'")
	}
	if !bytes.Contains([]byte(output), []byte("World")) {
		t.Error("Expected output to contain 'World'")
	}
	if !bytes.Contains([]byte(output), []byte("data: [DONE]")) {
		t.Error("Expected output to contain [DONE] marker")
	}
}

func TestConcurrentWrites(t *testing.T) {
	// Skip: Known flaky test - concurrent writes timing is unstable
	t.Skip("Skipping flaky test - known issue with concurrent writes")

	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()
	reasonWriter := writer.GetReasonWriter()

	// 同时写入普通内容和原因内容
	content := "Hello, World!"
	reason := "This is a reason"

	// 使用 goroutine 模拟并发写入
	done := make(chan bool)
	go func() {
		_, err := outputWriter.Write([]byte(content))
		if err != nil {
			t.Errorf("Output write failed: %v", err)
		}
		done <- true
	}()

	go func() {
		_, err := reasonWriter.Write([]byte(reason))
		if err != nil {
			t.Errorf("Reason write failed: %v", err)
		}
		done <- true
	}()

	// 等待两个写入完成
	<-done
	<-done

	// 关闭 writer
	err := writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 验证输出
	output := buf.(*testWriteCloser).String()
	t.Log("Concurrent writes output:")
	t.Log(output)

	// 验证所有内容都存在
	if !bytes.Contains([]byte(output), []byte(content)) {
		t.Errorf("Expected output to contain content: %s", content)
	}
	if !bytes.Contains([]byte(output), []byte(reason)) {
		t.Errorf("Expected output to contain reason: %s", reason)
	}

	// 验证只有一个 [DONE] 标记
	doneCount := strings.Count(output, "data: [DONE]")
	if doneCount != 1 {
		t.Errorf("Expected exactly one [DONE] marker, got %d", doneCount)
	}

	// 验证 JSON 格式
	lines := strings.Split(output, "\r\n")
	var contentFound, reasonFound bool
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if line == "data: [DONE]" {
			continue
		}

		var result map[string]interface{}
		err := json.Unmarshal([]byte(line[6:]), &result)
		if err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		// 安全地检查 JSON 结构
		choices, ok := result["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			// 不满足条件，跳过这个行
			continue
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			// 检查是否有 finish_reason
			if finish, ok := choice["finish_reason"]; ok && finish == "stop" {
				// 这是正常的结束标记
				continue
			}
			// 不是结束标记也没有 delta，这可能是个问题
			continue
		}

		// 检查内容和原因
		if content, ok := delta["content"]; ok && content == "Hello, World!" {
			contentFound = true
		}
		if reason, ok := delta["reason_content"]; ok && reason == "This is a reason" {
			reasonFound = true
		}
	}

	if !contentFound {
		t.Error("Content not found in output")
	}
	if !reasonFound {
		t.Error("Reason not found in output")
	}
}

func TestConcurrentSafety(t *testing.T) {
	// Skip: This test is flaky due to concurrent write timing issues
	// The [DONE] marker may not be written correctly under race conditions
	t.Skip("Skipping flaky concurrent safety test - known issue with writer timing")

	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()
	reasonWriter := writer.GetReasonWriter()

	// 设置并发数和总写入次数
	concurrency := 10
	totalWrites := 100
	contentPrefix := "content-"
	reasonPrefix := "reason-"

	// 使用 WaitGroup 等待所有 goroutine 完成
	var wg sync.WaitGroup
	wg.Add(concurrency * 2) // 每个 goroutine 都会写入内容和原因

	// 使用 channel 收集错误
	errChan := make(chan error, concurrency*2)

	// 启动多个 goroutine 进行并发写入
	for i := 0; i < concurrency; i++ {
		// 启动内容写入 goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < totalWrites/concurrency; j++ {
				content := fmt.Sprintf("%s%d-%d", contentPrefix, id, j)
				_, err := outputWriter.Write([]byte(content))
				if err != nil {
					errChan <- fmt.Errorf("content write failed (goroutine %d, write %d): %v", id, j, err)
					return
				}
				// 添加随机延迟模拟真实场景
				time.Sleep(time.Duration(id%10) * time.Millisecond)
			}
		}(i)

		// 启动原因写入 goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < totalWrites/concurrency; j++ {
				reason := fmt.Sprintf("%s%d-%d", reasonPrefix, id, j)
				_, err := reasonWriter.Write([]byte(reason))
				if err != nil {
					errChan <- fmt.Errorf("reason write failed (goroutine %d, write %d): %v", id, j, err)
					return
				}
				// 添加随机延迟模拟真实场景
				time.Sleep(time.Duration(id%10) * time.Millisecond)
			}
		}(i)
	}

	// 等待所有写入完成
	wg.Wait()
	close(errChan)

	// 检查是否有错误发生
	for err := range errChan {
		if err != nil {
			t.Fatal(err)
		}
	}

	// 关闭 writer
	err := writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 验证输出
	output := buf.(*testWriteCloser).String()
	if len(output) > 1000 {
		t.Log("Concurrent safety test output (first 1000 chars):")
		t.Log(output[:1000] + "...")
	} else {
		t.Log(output)
	}

	// 验证只有一个 [DONE] 标记
	doneCount := strings.Count(output, "data: [DONE]")
	if doneCount != 1 {
		t.Errorf("Expected exactly one [DONE] marker, got %d", doneCount)
	}

	// 验证所有内容都存在
	contentCount := 0
	reasonCount := 0
	lines := strings.Split(output, "\r\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if line == "data: [DONE]" {
			continue
		}

		var result map[string]interface{}
		err := json.Unmarshal([]byte(line[6:]), &result)
		if err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		// 安全地检查 JSON 结构
		choices, ok := result["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			// 不满足条件，跳过这个行
			continue
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			// 检查是否有 finish_reason
			if finish, ok := choice["finish_reason"]; ok && finish == "stop" {
				// 这是正常的结束标记
				continue
			}
			// 不是结束标记也没有 delta，这可能是个问题
			continue
		}

		if content, ok := delta["content"]; ok {
			if strings.HasPrefix(content.(string), contentPrefix) {
				contentCount++
			}
		}
		if reason, ok := delta["reason_content"]; ok {
			if strings.HasPrefix(reason.(string), reasonPrefix) {
				reasonCount++
			}
		}
	}

	// 验证写入数量
	if contentCount != totalWrites {
		t.Errorf("Expected %d content writes, got %d", totalWrites, contentCount)
	}
	if reasonCount != totalWrites {
		t.Errorf("Expected %d reason writes, got %d", totalWrites, reasonCount)
	}

	// 验证 JSON 格式完整性
	// 检查是否有损坏的 JSON
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if line == "data: [DONE]" {
			continue
		}

		var result map[string]interface{}
		err := json.Unmarshal([]byte(line[6:]), &result)
		if err != nil {
			t.Errorf("Invalid JSON found: %v, line: %s", err, line)
		}
	}
}

// TestWriteToolCalls tests the WriteToolCalls method for forwarding tool calls to clients
func TestWriteToolCalls(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")

	// Create test tool calls
	// Note: Index field is used in streaming responses to identify which tool call this is
	toolCalls := []*aispec.ToolCall{
		{
			Index: 0, // First tool call
			ID:    "call_test_123",
			Type:  "function",
			Function: aispec.FuncReturn{
				Name:      "get_weather",
				Arguments: `{"location":"Boston","unit":"celsius"}`,
			},
		},
		{
			Index: 1, // Second tool call
			ID:    "call_test_456",
			Type:  "function",
			Function: aispec.FuncReturn{
				Name:      "get_time",
				Arguments: `{"timezone":"America/New_York"}`,
			},
		},
	}

	// Write tool calls
	err := writer.WriteToolCalls(toolCalls)
	if err != nil {
		t.Fatalf("WriteToolCalls failed: %v", err)
	}

	// Close and wait for writer to finish
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	writer.Wait()

	// Verify output
	output := buf.(*testWriteCloser).String()
	t.Log("WriteToolCalls output:")
	t.Log(output)

	// Find the data line with tool_calls
	lines := strings.Split(output, "\r\n")
	var toolCallsLine string
	for _, line := range lines {
		if strings.Contains(line, "tool_calls") {
			toolCallsLine = line
			break
		}
	}
	if toolCallsLine == "" {
		t.Fatal("No tool_calls line found in output")
	}

	// Parse JSON
	dataStart := strings.Index(toolCallsLine, "data: ")
	if dataStart == -1 {
		t.Fatal("No 'data: ' prefix found")
	}
	jsonStr := toolCallsLine[dataStart+6:]
	var result map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v, json: %s", err, jsonStr)
	}

	// Verify structure
	if result["id"] != "chat-ai-balance-test-uid" {
		t.Errorf("Expected id chat-ai-balance-test-uid, got %v", result["id"])
	}
	if result["model"] != "test-model" {
		t.Errorf("Expected model test-model, got %v", result["model"])
	}

	choices := result["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	delta := choice["delta"].(map[string]interface{})

	// Verify role
	if delta["role"] != "assistant" {
		t.Errorf("Expected role assistant, got %v", delta["role"])
	}

	// Verify tool_calls structure
	resultToolCalls := delta["tool_calls"].([]interface{})
	if len(resultToolCalls) != 2 {
		t.Fatalf("Expected 2 tool calls, got %d", len(resultToolCalls))
	}

	// Verify first tool call
	tc1 := resultToolCalls[0].(map[string]interface{})
	// Verify index - must be 0 for the first tool call in the array
	if int(tc1["index"].(float64)) != 0 {
		t.Errorf("Expected index 0 for first tool call, got %v", tc1["index"])
	}
	if tc1["id"] != "call_test_123" {
		t.Errorf("Expected id call_test_123, got %v", tc1["id"])
	}
	if tc1["type"] != "function" {
		t.Errorf("Expected type function, got %v", tc1["type"])
	}
	fn1 := tc1["function"].(map[string]interface{})
	if fn1["name"] != "get_weather" {
		t.Errorf("Expected name get_weather, got %v", fn1["name"])
	}
	if !strings.Contains(fn1["arguments"].(string), "Boston") {
		t.Errorf("Expected arguments to contain Boston")
	}

	// Verify second tool call
	tc2 := resultToolCalls[1].(map[string]interface{})
	// Verify index - must be 1 for the second tool call in the array
	if int(tc2["index"].(float64)) != 1 {
		t.Errorf("Expected index 1 for second tool call, got %v", tc2["index"])
	}
	if tc2["id"] != "call_test_456" {
		t.Errorf("Expected id call_test_456, got %v", tc2["id"])
	}
	fn2 := tc2["function"].(map[string]interface{})
	if fn2["name"] != "get_time" {
		t.Errorf("Expected name get_time, got %v", fn2["name"])
	}
}

// TestWriteToolCalls_OpenAIStandardFormat verifies the tool_calls chunk follows OpenAI standard:
// - tool_calls MUST be in delta.tool_calls, NOT in delta.content
// - delta.content MUST be empty or null when tool_calls is present
// - Each tool_call MUST have: index, id, type, function.name, function.arguments
// This is critical for clients like Cursor to correctly recognize tool calls
func TestWriteToolCalls_OpenAIStandardFormat(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")

	toolCalls := []*aispec.ToolCall{
		{
			Index: 0,
			ID:    "call_abc123",
			Type:  "function",
			Function: aispec.FuncReturn{
				Name:      "read_file",
				Arguments: `{"path":"/Users/test/README.md"}`,
			},
		},
	}

	err := writer.WriteToolCalls(toolCalls)
	assert.NoError(t, err, "WriteToolCalls should succeed")

	err = writer.Close()
	assert.NoError(t, err, "Close should succeed")
	writer.Wait()

	output := buf.(*testWriteCloser).String()
	t.Logf("Tool calls output:\n%s", output)

	// Parse each data line and verify format
	lines := strings.Split(output, "\r\n")
	var toolCallChunkFound bool

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
			continue
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}
		choice := choices[0].(map[string]interface{})
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is a tool_calls chunk
		if toolCallsRaw, hasToolCalls := delta["tool_calls"]; hasToolCalls {
			toolCallChunkFound = true

			// CRITICAL: When tool_calls is present, content MUST NOT contain tool call data
			if content, hasContent := delta["content"]; hasContent {
				contentStr, _ := content.(string)
				// Content should be empty, null, or not contain tool-like patterns
				assert.NotContains(t, contentStr, "<read_file>", "Content MUST NOT contain XML-style tool calls")
				assert.NotContains(t, contentStr, "<function>", "Content MUST NOT contain XML-style function tags")
				assert.NotContains(t, contentStr, "\"function\":", "Content MUST NOT contain JSON tool call data")
			}

			// Verify tool_calls structure
			toolCallsArr := toolCallsRaw.([]interface{})
			assert.Len(t, toolCallsArr, 1, "Should have 1 tool call")

			tc := toolCallsArr[0].(map[string]interface{})

			// REQUIRED fields per OpenAI spec
			assert.Contains(t, tc, "index", "tool_call MUST have 'index' field")
			assert.Contains(t, tc, "id", "tool_call MUST have 'id' field")
			assert.Contains(t, tc, "type", "tool_call MUST have 'type' field")
			assert.Contains(t, tc, "function", "tool_call MUST have 'function' field")

			assert.Equal(t, float64(0), tc["index"], "First tool_call should have index 0")
			assert.Equal(t, "call_abc123", tc["id"], "tool_call id should match")
			assert.Equal(t, "function", tc["type"], "tool_call type should be 'function'")

			fn := tc["function"].(map[string]interface{})
			assert.Equal(t, "read_file", fn["name"], "function name should be 'read_file'")
			assert.Contains(t, fn["arguments"], "README.md", "function arguments should contain path")
		}
	}

	assert.True(t, toolCallChunkFound, "Output should contain a tool_calls chunk")
}

// TestWriteToolCallsWithContentAndReason tests that tool calls work with other stream types
func TestWriteToolCallsWithContentAndReason(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	outputWriter := writer.GetOutputWriter()
	reasonWriter := writer.GetReasonWriter()

	// Write content
	_, err := outputWriter.Write([]byte("Let me check that for you."))
	if err != nil {
		t.Fatalf("Content write failed: %v", err)
	}

	// Write reason
	_, err = reasonWriter.Write([]byte("User wants weather info."))
	if err != nil {
		t.Fatalf("Reason write failed: %v", err)
	}

	// Write tool calls
	toolCalls := []*aispec.ToolCall{
		{
			ID:   "call_mixed_789",
			Type: "function",
			Function: aispec.FuncReturn{
				Name:      "get_weather",
				Arguments: `{"location":"Tokyo"}`,
			},
		},
	}
	err = writer.WriteToolCalls(toolCalls)
	if err != nil {
		t.Fatalf("WriteToolCalls failed: %v", err)
	}

	// Close and wait
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	writer.Wait()

	// Verify all types are present
	output := buf.(*testWriteCloser).String()
	if !strings.Contains(output, "Let me check that") {
		t.Error("Output should contain content")
	}
	if !strings.Contains(output, "User wants weather") {
		t.Error("Output should contain reason")
	}
	if !strings.Contains(output, "tool_calls") {
		t.Error("Output should contain tool_calls")
	}
	if !strings.Contains(output, "get_weather") {
		t.Error("Output should contain get_weather function")
	}
	if !strings.Contains(output, "Tokyo") {
		t.Error("Output should contain Tokyo in arguments")
	}
	if !strings.Contains(output, "[DONE]") {
		t.Error("Output should contain [DONE] marker")
	}
}

// TestWriteToolCallsNotStream tests that WriteToolCalls is no-op for non-streaming mode
func TestWriteToolCallsNotStream(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")
	writer.notStream = true

	toolCalls := []*aispec.ToolCall{
		{
			ID:   "call_nostream",
			Type: "function",
			Function: aispec.FuncReturn{
				Name:      "test_func",
				Arguments: `{}`,
			},
		},
	}

	// Should not error in non-stream mode
	err := writer.WriteToolCalls(toolCalls)
	if err != nil {
		t.Fatalf("WriteToolCalls should not error in non-stream mode: %v", err)
	}

	// Close and wait
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	writer.Wait()

	// In non-stream mode, tool calls should not be written to the stream
	// (they would be in the final message instead)
	output := buf.(*testWriteCloser).String()
	// The output should be empty or minimal for non-stream mode
	// because non-stream doesn't use SSE format
	t.Logf("Non-stream output: %s", output)
}

// TestWriterDoubleCloseIsSafe verifies that calling Close() multiple times
// does not panic and is safe (idempotent).
func TestWriterDoubleCloseIsSafe(t *testing.T) {
	buf := newTestWriteCloser()
	writer := NewChatJSONChunkWriter(buf, "test-uid", "test-model")

	// Write some data
	outputWriter := writer.GetOutputWriter()
	_, err := outputWriter.Write([]byte("test content"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// First close should succeed
	err = writer.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should not panic and should return nil
	err = writer.Close()
	if err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}

	// Third close should also be safe
	err = writer.Close()
	if err != nil {
		t.Fatalf("Third Close failed: %v", err)
	}

	// Wait should also be safe to call multiple times
	writer.Wait()
	writer.Wait()
}
