// Package tests 包含AI聊天功能的集成测试
// 主要测试流式和非流式聊天响应的处理逻辑
package tests

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// mockAiRsp 模拟的非流式AI响应数据
// 这是标准的OpenAI API JSON响应格式，包含：
// - id: 请求唯一标识符
// - object: 响应对象类型 "chat.completion"
// - model: 使用的AI模型名称
// - choices: 响应选择数组，包含助手的回复内容
// - usage: token使用统计信息
const mockAiRsp = `HTTP/1.1 200 OK
Connection: close
Content-Type: application/json; charset=utf-8

{
  "id": "01983a7496e24930e8de7952fd33c19c",
  "object": "chat.completion",
  "created": 1753327376,
  "model": "deepseek-ai/DeepSeek-V3",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！😊 很高兴见到你～有什么我可以帮你的吗？"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": { "prompt_tokens": 4, "completion_tokens": 15, "total_tokens": 19 },
  "system_fingerprint": ""
}
`

// mockAiStreamRsp 模拟的流式AI响应数据
// 这是Server-Sent Events (SSE) 格式的流式响应，包含：
// - Content-Type: text/event-stream 表示这是流式数据
// - data: 前缀的JSON chunk，每个chunk包含部分响应内容
// - delta: 增量内容，包含content和reasoning_content字段
// - [DONE]: 流式响应结束标识符
const mockAiStreamRsp = `HTTP/1.1 200 OK
Connection: close
Content-Type: text/event-stream

data: {"id":"01983a922eff7e1cee0f9a1cdbbd74f4","object":"chat.completion.chunk","created":1753329315,"model":"deepseek-ai/DeepSeek-V3","choices":[{"index":0,"delta":{"content":"","reasoning_content":null,"role":"assistant"},"finish_reason":null}],"system_fingerprint":"","usage":{"prompt_tokens":4,"completion_tokens":0,"total_tokens":4}}

data: {"id":"01983a922eff7e1cee0f9a1cdbbd74f4","object":"chat.completion.chunk","created":1753329315,"model":"deepseek-ai/DeepSeek-V3","choices":[{"index":0,"delta":{"content":"你好","reasoning_content":null},"finish_reason":null}],"system_fingerprint":"","usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}

data: [DONE]
`

// mockAiReasoningRsp 模拟包含推理内容的AI响应
// 用于测试推理内容(reasoning_content)的处理逻辑
const mockAiReasoningRsp = `HTTP/1.1 200 OK
Connection: close
Content-Type: application/json; charset=utf-8

{
  "id": "reasoning-test-123",
  "object": "chat.completion",
  "created": 1753327376,
  "model": "deepseek-ai/DeepSeek-V3",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "基于我的分析，答案是42。",
        "reasoning_content": "用户询问了生命、宇宙和一切的终极答案。根据《银河系漫游指南》，这个答案是42。"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": { "prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30 }
}
`

// TestNonStreamChat 测试非流式聊天功能
// 验证：
// 1. 非流式响应的正确解析
// 2. message.content字段的提取
// 3. 最终返回内容的正确性
func TestNonStreamChat(t *testing.T) {
	// 创建模拟HTTP服务器，返回预定义的非流式响应
	host, port := utils.DebugMockHTTP([]byte(mockAiRsp))

	// 调用ChatBase进行非流式聊天
	// 不设置StreamHandler，默认为非流式处理
	res, err := aispec.ChatBase(
		"http://api.openai.com/v1/chat/completions",
		"gpt-4o-mini",
		"hello",
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return []poc.PocConfigOption{
				poc.WithHost(host),        // 使用模拟服务器的主机
				poc.WithPort(port),        // 使用模拟服务器的端口
				poc.WithForceHTTPS(false), // 禁用HTTPS
				poc.WithTimeout(3),        // 设置3秒超时
			}, nil
		}))

	// 检查是否有错误发生
	if err != nil {
		t.Fatal(err)
	}

	// 验证返回的内容是否符合预期
	// 应该从JSON响应的choices[0].message.content字段中提取内容
	assert.Equal(t, "你好！😊 很高兴见到你～有什么我可以帮你的吗？", res)
}

// TestStreamChat 测试流式聊天功能
// 验证：
// 1. 流式响应的正确解析
// 2. delta.content字段的逐步累积
// 3. 流式处理器的正确调用
func TestStreamChat(t *testing.T) {
	// 创建模拟HTTP服务器，返回预定义的流式响应
	host, port := utils.DebugMockHTTP([]byte(mockAiStreamRsp))

	// 用于捕获流式数据的变量
	var streamContent strings.Builder

	// 调用ChatBase进行流式聊天
	// 设置StreamHandler启用流式处理
	res, err := aispec.ChatBase(
		"http://api.openai.com/v1/chat/completions",
		"gpt-4o-mini",
		"hello",
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return []poc.PocConfigOption{
				poc.WithHost(host),        // 使用模拟服务器的主机
				poc.WithPort(port),        // 使用模拟服务器的端口
				poc.WithForceHTTPS(false), // 禁用HTTPS
				poc.WithTimeout(3),        // 设置3秒超时
			}, nil
		}),
		// 流式处理器：读取并保存流式数据
		aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
			data, _ := io.ReadAll(reader)
			streamContent.Write(data)
		}))

	// 检查是否有错误发生
	if err != nil {
		t.Fatal(err)
	}

	// 验证返回的内容是否符合预期
	// 流式响应应该累积所有delta.content的内容
	assert.Equal(t, "你好", res)

	// 验证流式处理器是否被正确调用并接收到数据
	assert.NotEmpty(t, streamContent.String(), "流式处理器应该接收到数据")
}

// TestNonStreamChatWithReasoning 测试包含推理内容的非流式聊天
// 验证：
// 1. reasoning_content字段的正确处理
// 2. 推理内容和正常内容的分离
func TestNonStreamChatWithReasoning(t *testing.T) {
	t.Skip("wait for fix enable thinking option issue")
	// 创建模拟HTTP服务器，返回包含推理内容的响应
	host, port := utils.DebugMockHTTP([]byte(mockAiReasoningRsp))

	// 用于捕获推理内容的变量
	var reasonContent strings.Builder

	// 调用ChatBase，同时处理推理内容
	res, err := aispec.ChatBase(
		"http://api.openai.com/v1/chat/completions",
		"gpt-4o-mini",
		"什么是生命、宇宙和一切的终极答案？",
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return []poc.PocConfigOption{
				poc.WithHost(host),
				poc.WithPort(port),
				poc.WithForceHTTPS(false),
				poc.WithTimeout(3),
			}, nil
		}),
		// 推理内容处理器：专门处理reasoning_content
		aispec.WithChatBase_ReasonStreamHandler(func(reader io.Reader) {
			data, _ := io.ReadAll(reader)
			reasonContent.Write(data)
		}))

	// 检查是否有错误发生
	if err != nil {
		t.Fatal(err)
	}

	// 验证正常回复内容
	assert.Equal(t, "基于我的分析，答案是42。", res)

	// 验证推理内容是否被正确处理
	expectedReasoning := "用户询问了生命、宇宙和一切的终极答案。根据《银河系漫游指南》，这个答案是42。"
	assert.Contains(t, reasonContent.String(), expectedReasoning, "推理内容应该被正确提取")
}

// TestChatBaseErrorHandling 测试错误处理机制
// 验证：
// 1. HTTP错误的正确处理
// 2. 错误回调函数的调用
func TestChatBaseErrorHandling(t *testing.T) {
	// 用于捕获错误的变量
	var capturedError error

	// 调用ChatBase，使用不存在的服务器地址来触发错误
	_, err := aispec.ChatBase(
		"http://nonexistent-server.com/v1/chat/completions",
		"gpt-4o-mini",
		"hello",
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return []poc.PocConfigOption{
				poc.WithTimeout(1), // 短超时确保快速失败
			}, nil
		}),
		// 错误处理器：捕获HTTP错误
		aispec.WithChatBase_ErrHandler(func(httpErr error) {
			capturedError = httpErr
		}))

	// 应该返回错误
	assert.Error(t, err, "应该返回连接错误")

	// 错误处理器应该被调用
	assert.Error(t, capturedError, "错误处理器应该捕获到HTTP错误")
}
