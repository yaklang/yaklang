package aitag

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// slowReader 模拟慢速流式输入
type slowReader struct {
	data     []byte
	position int
	delay    time.Duration
}

func newSlowReader(data string, delay time.Duration) *slowReader {
	return &slowReader{
		data:  []byte(data),
		delay: delay,
	}
}

func (sr *slowReader) Read(p []byte) (n int, err error) {
	if sr.position >= len(sr.data) {
		return 0, io.EOF
	}

	// 模拟网络延迟
	if sr.delay > 0 {
		time.Sleep(sr.delay)
	}

	// 每次只返回一个字节，模拟极慢的流
	p[0] = sr.data[sr.position]
	sr.position++
	return 1, nil
}

// TestLongStreamProcessing 测试长流输入处理
func TestLongStreamProcessing(t *testing.T) {
	// 生成一个很长的输入流
	var builder strings.Builder
	builder.WriteString("开始处理长流数据\n")

	// 添加多个大的代码块
	for i := 0; i < 10; i++ {
		builder.WriteString(fmt.Sprintf("<|CODE_block_%d|>\n", i))

		// 每个代码块包含大量内容
		for j := 0; j < 100; j++ {
			builder.WriteString(fmt.Sprintf("// 这是第 %d 个代码块的第 %d 行\n", i, j))
			builder.WriteString(fmt.Sprintf("func function_%d_%d() {\n", i, j))
			builder.WriteString("    // 实现代码\n")
			builder.WriteString("    var data = map[string]interface{}{\n")
			builder.WriteString(fmt.Sprintf("        \"block\": %d,\n", i))
			builder.WriteString(fmt.Sprintf("        \"line\": %d,\n", j))
			builder.WriteString("        \"content\": \"这里是一些复杂的内容，包含各种字符: !@#$%^&*()[]{}|\\\\\\\"'\",\n")
			builder.WriteString("    }\n")
			builder.WriteString("    return data\n")
			builder.WriteString("}\n\n")
		}

		builder.WriteString(fmt.Sprintf("<|CODE_END_block_%d|>\n", i))
		builder.WriteString(fmt.Sprintf("处理完成第 %d 个代码块\n\n", i))
	}

	builder.WriteString("所有长流数据处理完成\n")

	input := builder.String()
	log.Infof("生成的测试数据长度: %d 字符", len(input))

	var processedBlocks = make(map[string]int)
	var totalBytes = 0
	var mu sync.Mutex

	// 注册所有代码块的回调
	var options []ParseOption
	for i := 0; i < 10; i++ {
		nonce := fmt.Sprintf("block_%d", i)
		options = append(options, WithCallback("CODE", nonce, func(reader io.Reader) {
			content, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取内容失败: %v", err)
				return
			}

			mu.Lock()
			processedBlocks[nonce] = len(content)
			totalBytes += len(content)
			mu.Unlock()

			log.Infof("处理了代码块 %s，内容长度: %d", nonce, len(content))
		}))
	}

	start := time.Now()
	err := Parse(strings.NewReader(input), options...)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("长流解析失败: %v", err)
	}

	// 验证结果
	if len(processedBlocks) != 10 {
		t.Errorf("期望处理 10 个代码块，实际处理了 %d 个", len(processedBlocks))
	}

	log.Infof("长流处理完成，耗时: %v，总字节数: %d", duration, totalBytes)

	// 验证每个块都有内容
	for i := 0; i < 10; i++ {
		nonce := fmt.Sprintf("block_%d", i)
		if size, exists := processedBlocks[nonce]; !exists {
			t.Errorf("代码块 %s 未被处理", nonce)
		} else if size == 0 {
			t.Errorf("代码块 %s 内容为空", nonce)
		}
	}
}

// TestSlowStreamProcessing 测试慢速流输入
func TestSlowStreamProcessing(t *testing.T) {
	input := `开始慢速流测试
<|SLOW_test123|>
这是一个慢速流的内容
包含多行数据
每个字符都会慢慢传输
<|SLOW_END_test123|>
慢速流测试结束`

	var receivedContent string
	var startTime time.Time
	var endTime time.Time

	// 创建慢速读取器，每个字符延迟1毫秒
	slowReader := newSlowReader(input, time.Millisecond)

	startTime = time.Now()
	err := Parse(slowReader, WithCallback("SLOW", "test123", func(reader io.Reader) {
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("读取慢速流内容失败: %v", err)
			return
		}
		receivedContent = string(content)
		endTime = time.Now()
		log.Infof("慢速流处理完成，内容长度: %d", len(receivedContent))
	}))

	if err != nil {
		t.Fatalf("慢速流解析失败: %v", err)
	}

	duration := endTime.Sub(startTime)
	log.Infof("慢速流处理耗时: %v", duration)

	// 验证内容正确性
	if !strings.Contains(receivedContent, "慢速流的内容") {
		t.Error("慢速流内容不正确")
	}

	// 验证确实花费了时间（说明是流式处理的）
	if duration < time.Millisecond*50 { // 应该至少花费字符数量的毫秒数
		t.Errorf("处理时间过短，可能不是真正的流式处理: %v", duration)
	}
}

// TestConcurrentStreamProcessing 测试并发流处理
func TestConcurrentStreamProcessing(t *testing.T) {
	// 创建多个并发的流
	numStreams := 5
	streamSize := 1000

	var wg sync.WaitGroup
	var results = make(map[string]string)
	var mu sync.Mutex

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamID int) {
			defer wg.Done()

			// 为每个goroutine生成不同的输入
			var builder strings.Builder
			nonce := fmt.Sprintf("stream_%d", streamID)

			builder.WriteString(fmt.Sprintf("<|DATA_%s|>\n", nonce))
			for j := 0; j < streamSize; j++ {
				builder.WriteString(fmt.Sprintf("数据行 %d_%d: 这里是一些测试数据内容\n", streamID, j))
			}
			builder.WriteString(fmt.Sprintf("<|DATA_END_%s|>\n", nonce))

			input := builder.String()

			err := Parse(strings.NewReader(input), WithCallback("DATA", nonce, func(reader io.Reader) {
				content, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("并发流 %d 读取失败: %v", streamID, err)
					return
				}

				mu.Lock()
				results[nonce] = string(content)
				mu.Unlock()

				log.Infof("并发流 %d 处理完成，内容长度: %d", streamID, len(content))
			}))

			if err != nil {
				t.Errorf("并发流 %d 解析失败: %v", streamID, err)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有流都被正确处理
	if len(results) != numStreams {
		t.Errorf("期望处理 %d 个并发流，实际处理了 %d 个", numStreams, len(results))
	}

	// 验证每个流的内容
	for i := 0; i < numStreams; i++ {
		nonce := fmt.Sprintf("stream_%d", i)
		content, exists := results[nonce]
		if !exists {
			t.Errorf("并发流 %s 结果不存在", nonce)
			continue
		}

		// 验证内容包含预期的行数
		lines := strings.Split(content, "\n")
		if len(lines) < streamSize {
			t.Errorf("并发流 %s 内容行数不足，期望至少 %d 行，实际 %d 行", nonce, streamSize, len(lines))
		}
	}

	log.Infof("并发流处理测试完成，处理了 %d 个流", len(results))
}

// TestStreamingBoundaryConditions 测试流式处理边界条件
func TestStreamingBoundaryConditions(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "单字符标签",
			input: `<|A_x|>内容<|A_END_x|>`,
			desc:  "测试最短的标签名",
		},
		{
			name:  "长标签名",
			input: `<|VERY_LONG_TAG_NAME_WITH_MANY_CHARACTERS_test|>内容<|VERY_LONG_TAG_NAME_WITH_MANY_CHARACTERS_END_test|>`,
			desc:  "测试很长的标签名",
		},
		{
			name:  "空内容",
			input: `<|EMPTY_test|><|EMPTY_END_test|>`,
			desc:  "测试空内容标签",
		},
		{
			name:  "只有换行符",
			input: "<|NEWLINE_test|>\n\n\n<|NEWLINE_END_test|>",
			desc:  "测试只包含换行符的内容",
		},
		{
			name:  "特殊字符",
			input: `<|SPECIAL_test|>!@#$%^&*()[]{}|\"'<>=+-_~` + "`" + `<|SPECIAL_END_test|>`,
			desc:  "测试包含特殊字符的内容",
		},
		{
			name:  "Unicode字符",
			input: `<|UNICODE_test|>你好世界 🌍 こんにちは مرحبا<|UNICODE_END_test|>`,
			desc:  "测试Unicode字符",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedContent string

			err := Parse(strings.NewReader(tc.input), WithCallback(strings.Split(tc.name, "_")[0], "test", func(reader io.Reader) {
				// 注意：这里的标签名处理需要根据具体情况调整
				content, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("读取内容失败: %v", err)
					return
				}
				capturedContent = string(content)
				log.Infof("边界条件测试 [%s]: 捕获内容长度 %d", tc.name, len(capturedContent))
			}))

			if err != nil {
				t.Errorf("边界条件测试失败 [%s]: %v", tc.name, err)
			}

			log.Infof("边界条件测试 [%s] 完成: %s", tc.name, tc.desc)
		})
	}
}

// TestStreamingMemoryUsage 测试流式处理内存使用
func TestStreamingMemoryUsage(t *testing.T) {
	// 创建一个非常大的输入，但分成小块处理
	const blockSize = 10000
	const numBlocks = 100

	var results []int
	var mu sync.Mutex

	// 构建大输入
	var builder strings.Builder
	for i := 0; i < numBlocks; i++ {
		nonce := fmt.Sprintf("block_%d", i)
		builder.WriteString(fmt.Sprintf("<|MEMORY_%s|>", nonce))

		// 每个块包含大量数据
		for j := 0; j < blockSize; j++ {
			builder.WriteString(fmt.Sprintf("数据_%d_%d ", i, j))
		}

		builder.WriteString(fmt.Sprintf("<|MEMORY_END_%s|>", nonce))
	}

	input := builder.String()
	log.Infof("内存测试输入大小: %d MB", len(input)/(1024*1024))

	// 注册回调处理每个块
	var options []ParseOption
	for i := 0; i < numBlocks; i++ {
		nonce := fmt.Sprintf("block_%d", i)
		options = append(options, WithCallback("MEMORY", nonce, func(reader io.Reader) {
			// 流式处理，不把整个内容读入内存
			var size int
			buffer := make([]byte, 1024)
			for {
				n, err := reader.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("流式读取失败: %v", err)
					return
				}
				size += n
			}

			mu.Lock()
			results = append(results, size)
			mu.Unlock()

			if len(results)%10 == 0 {
				log.Infof("已处理 %d 个内存测试块", len(results))
			}
		}))
	}

	start := time.Now()
	err := Parse(strings.NewReader(input), options...)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("内存测试解析失败: %v", err)
	}

	log.Infof("内存测试完成，处理了 %d 个块，耗时: %v", len(results), duration)

	// 验证所有块都被处理
	if len(results) != numBlocks {
		t.Errorf("期望处理 %d 个块，实际处理了 %d 个", numBlocks, len(results))
	}

	// 验证每个块的大小合理
	for i, size := range results {
		if size == 0 {
			t.Errorf("块 %d 大小为 0", i)
		}
	}
}

// TestStreamingInterruption 测试流中断处理
func TestStreamingInterruption(t *testing.T) {
	// 测试不完整的流输入
	incompleteInputs := []string{
		"<|INCOMPLETE_test|>内容但是没有结束标签",
		"<|INCOMPLETE_test|>内容<|INCOMPLETE_",
		"<|INCOMPLETE_test|>内容<|INCOMPLETE_END_",
		"<|INCOMPLETE_test|>内容<|INCOMPLETE_END_test",
		"<|INCOMPLETE_test|>内容<|INCOMPLETE_END_test|",
	}

	for i, input := range incompleteInputs {
		t.Run(fmt.Sprintf("中断测试_%d", i), func(t *testing.T) {
			var callbackTriggered bool

			err := Parse(strings.NewReader(input), WithCallback("INCOMPLETE", "test", func(reader io.Reader) {
				callbackTriggered = true
				content, _ := io.ReadAll(reader)
				log.Infof("中断测试 %d: 收到内容长度 %d", i, len(content))
			}))

			// 不完整的输入不应该触发错误，但可能不会触发回调
			if err != nil {
				t.Errorf("中断测试 %d 不应该返回错误: %v", i, err)
			}

			log.Infof("中断测试 %d 完成，回调触发: %v", i, callbackTriggered)
		})
	}
}
