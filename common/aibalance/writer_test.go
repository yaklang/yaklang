package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
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
	fmt.Println("Multiple writes output:")
	fmt.Println(output)

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
	fmt.Println("Concurrent writes output:")
	fmt.Println(output)

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

		choices := result["choices"].([]interface{})
		choice := choices[0].(map[string]interface{})
		delta := choice["delta"].(map[string]interface{})

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
	fmt.Println("Concurrent safety test output (first 1000 chars):")
	if len(output) > 1000 {
		fmt.Println(output[:1000] + "...")
	} else {
		fmt.Println(output)
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

		choices := result["choices"].([]interface{})
		choice := choices[0].(map[string]interface{})
		delta := choice["delta"].(map[string]interface{})

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
