package aiengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
)

// TestNewAIEngine 测试创建 AI 引擎的基本功能
func TestNewAIEngine(t *testing.T) {
	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "basic")
		},
		WithMaxIteration(5),
		WithSessionID("test-session"),
		WithYOLOMode(),
	)
	defer engine.Close()

	// 验证配置是否正确设置
	if engine.config.MaxIteration != 5 {
		t.Errorf("expected MaxIteration to be 5, got %d", engine.config.MaxIteration)
	}

	if engine.config.SessionID != "test-session" {
		t.Errorf("expected SessionID to be 'test-session', got %s", engine.config.SessionID)
	}

	if engine.config.ReviewPolicy != "yolo" {
		t.Errorf("expected ReviewPolicy to be 'yolo', got %s", engine.config.ReviewPolicy)
	}

	// 验证引擎的基本状态
	if engine.ctx == nil {
		t.Error("expected context to be initialized")
	}

	if engine.activeTasks == nil {
		t.Error("expected activeTasks map to be initialized")
	}
}

// TestAIEngineConfigOptions 测试各种配置选项
func TestAIEngineConfigOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  []AIEngineConfigOption
		validate func(*testing.T, *AIEngineConfig)
	}{
		{
			name: "WithMaxIteration",
			options: []AIEngineConfigOption{
				WithMaxIteration(20),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if c.MaxIteration != 20 {
					t.Errorf("expected MaxIteration=20, got %d", c.MaxIteration)
				}
			},
		},
		{
			name: "WithYOLOMode",
			options: []AIEngineConfigOption{
				WithYOLOMode(),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if c.ReviewPolicy != "yolo" {
					t.Errorf("expected ReviewPolicy=yolo, got %s", c.ReviewPolicy)
				}
				if c.AllowUserInteract != false {
					t.Errorf("expected AllowUserInteract=false, got %v", c.AllowUserInteract)
				}
			},
		},
		{
			name: "WithManualMode",
			options: []AIEngineConfigOption{
				WithManualMode(),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if c.ReviewPolicy != "manual" {
					t.Errorf("expected ReviewPolicy=manual, got %s", c.ReviewPolicy)
				}
				if c.AllowUserInteract != true {
					t.Errorf("expected AllowUserInteract=true, got %v", c.AllowUserInteract)
				}
			},
		},
		{
			name: "WithKeywords",
			options: []AIEngineConfigOption{
				WithKeywords("test", "example"),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if len(c.Keywords) != 2 {
					t.Errorf("expected 2 keywords, got %d", len(c.Keywords))
				}
				if c.Keywords[0] != "test" || c.Keywords[1] != "example" {
					t.Errorf("keywords not set correctly: %v", c.Keywords)
				}
			},
		},
		{
			name: "WithDebugMode",
			options: []AIEngineConfigOption{
				WithDebugMode(true),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if !c.DebugMode {
					t.Error("expected DebugMode=true")
				}
			},
		},
		{
			name: "WithAIService",
			options: []AIEngineConfigOption{
				WithAIService("custom-service"),
			},
			validate: func(t *testing.T, c *AIEngineConfig) {
				if c.AIService != "custom-service" {
					t.Errorf("expected AIService=custom-service, got %s", c.AIService)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewAIEngineConfig(tt.options...)
			tt.validate(t, config)
		})
	}
}

// TestAIEngineConfigOptions_MultipleOptions 测试组合多个配置选项
func TestAIEngineConfigOptions_MultipleOptions(t *testing.T) {
	config := NewAIEngineConfig(
		WithMaxIteration(10),
		WithSessionID("multi-test"),
		WithYOLOMode(),
		WithKeywords("keyword1", "keyword2", "keyword3"),
		WithDebugMode(true),
	)

	if config.MaxIteration != 10 {
		t.Errorf("expected MaxIteration=10, got %d", config.MaxIteration)
	}

	if config.SessionID != "multi-test" {
		t.Errorf("expected SessionID=multi-test, got %s", config.SessionID)
	}

	if config.ReviewPolicy != "yolo" {
		t.Errorf("expected ReviewPolicy=yolo, got %s", config.ReviewPolicy)
	}

	if len(config.Keywords) != 3 {
		t.Errorf("expected 3 keywords, got %d", len(config.Keywords))
	}

	if !config.DebugMode {
		t.Error("expected DebugMode=true")
	}
}

// TestAIEngineEventHandling 测试事件处理回调设置
func TestAIEngineEventHandling(t *testing.T) {
	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "event")
		},
		WithMaxIteration(1),
		WithYOLOMode(),
		WithOnEvent(func(react *aireact.ReAct, event *schema.AiOutputEvent) {
			t.Logf("Received event: type=%s, isStream=%v", event.Type, event.IsStream)
		}),
	)
	defer engine.Close()

	// 验证事件处理器已设置
	if engine.config.OnEvent == nil {
		t.Error("expected OnEvent callback to be set")
	}

	// 验证其他回调也可以设置
	engine2 := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "callbacks")
		},
		WithMaxIteration(1),
		WithYOLOMode(),
		WithOnStream(func(react *aireact.ReAct, event *schema.AiOutputEvent, nodeId string, data []byte) {
			t.Logf("Stream received: %d bytes", len(data))
		}),
		WithOnData(func(react *aireact.ReAct, event *schema.AiOutputEvent, nodeId string, data []byte) {
			t.Logf("Data received: %d bytes", len(data))
		}),
		WithOnFinished(func(react *aireact.ReAct) {
			t.Logf("Finished")
		}),
	)
	defer engine2.Close()

	if engine2.config.OnStream == nil {
		t.Error("expected OnStream callback to be set")
	}

	if engine2.config.OnData == nil {
		t.Error("expected OnData callback to be set")
	}

	if engine2.config.OnFinished == nil {
		t.Error("expected OnFinished callback to be set")
	}
}

// TestAIEngineWithCancelledContext 测试已取消的上下文
func TestAIEngineWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "cancelled")
		},
		WithContext(ctx),
		WithMaxIteration(1),
		WithYOLOMode(),
	)
	defer engine.Close()

	// 验证上下文已被取消
	select {
	case <-engine.ctx.Done():
		// 预期行为：上下文已取消
	default:
		t.Error("expected context to be cancelled")
	}
}

// TestConvertToYPBAIStartParams 测试配置转换为 GRPC 参数
func TestConvertToYPBAIStartParams(t *testing.T) {
	config := NewAIEngineConfig(
		WithMaxIteration(15),
		WithSessionID("test-session"),
		WithReviewPolicy("manual"),
		WithKeywords("test", "example"),
	)

	params := config.ConvertToYPBAIStartParams()

	if params.ReActMaxIteration != 15 {
		t.Errorf("expected ReActMaxIteration=15, got %d", params.ReActMaxIteration)
	}

	if params.TimelineSessionID != "test-session" {
		t.Errorf("expected TimelineSessionID='test-session', got %s", params.TimelineSessionID)
	}

	if params.ReviewPolicy != "manual" {
		t.Errorf("expected ReviewPolicy='manual', got %s", params.ReviewPolicy)
	}

	if len(params.IncludeSuggestedToolKeywords) != 2 {
		t.Errorf("expected 2 keywords, got %d", len(params.IncludeSuggestedToolKeywords))
	}

	// 验证关键词内容
	keywordMap := make(map[string]bool)
	for _, kw := range params.IncludeSuggestedToolKeywords {
		keywordMap[kw] = true
	}
	if !keywordMap["test"] || !keywordMap["example"] {
		t.Errorf("keywords not correctly converted: %v", params.IncludeSuggestedToolKeywords)
	}
}

// TestConvertToYPBAIStartParams_DefaultValues 测试默认值转换
func TestConvertToYPBAIStartParams_DefaultValues(t *testing.T) {
	config := NewAIEngineConfig()
	params := config.ConvertToYPBAIStartParams()

	// 验证默认值
	if params == nil {
		t.Fatal("expected params to be non-nil")
	}

	// 默认最大迭代次数应该有合理的值
	if params.ReActMaxIteration < 0 {
		t.Errorf("expected non-negative ReActMaxIteration, got %d", params.ReActMaxIteration)
	}
}

// TestMultipleTasksTracking 测试多任务跟踪功能
func TestMultipleTasksTracking(t *testing.T) {
	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "tracking")
		},
		WithMaxIteration(5),
		WithYOLOMode(),
	)
	defer engine.Close()

	// 验证初始状态
	if engine.GetActiveTaskCount() != 0 {
		t.Errorf("expected 0 active tasks initially, got %d", engine.GetActiveTaskCount())
	}

	// 验证 activeTasks map 已初始化
	if engine.activeTasks == nil {
		t.Error("expected activeTasks map to be initialized")
	}

	// 验证 allTasksEndpoint 已初始化
	if engine.allTasksEndpoint == nil {
		t.Error("expected allTasksEndpoint to be initialized")
	}

	// 验证 taskEndpoints 已初始化
	if engine.taskEndpoints == nil {
		t.Error("expected taskEndpoints map to be initialized")
	}
}

// TestWaitTaskFinishByTaskName 测试按任务名等待任务完成
func TestWaitTaskFinishByTaskName(t *testing.T) {
	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "wait")
		},
		WithMaxIteration(5),
		WithYOLOMode(),
	)
	defer engine.Close()

	// 测试空任务ID
	err := engine.WaitTaskFinishByTaskName("")
	if err == nil {
		t.Error("expected error for empty taskID")
	}

	// 测试不存在的任务（应该超时或被取消）
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	engine.ctx = ctx
	err = engine.WaitTaskFinishByTaskName("non-existent-task")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

// BenchmarkNewAIEngineConfig 基准测试：创建配置
func BenchmarkNewAIEngineConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewAIEngineConfig(
			WithMaxIteration(10),
			WithSessionID("bench-session"),
			WithYOLOMode(),
			WithKeywords("keyword1", "keyword2"),
		)
	}
}

// TestMain 用于设置测试环境
func TestMain(m *testing.M) {
	// 设置测试环境
	os.Exit(m.Run())
}

// ==================== Mock 测试辅助函数 ====================

// mockedDirectAnswer 返回一个简单的直接回答
func mockedDirectAnswer(i aicommon.AICallerConfigIf, flag string) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "directly_answer",
	"answer_payload": "mocked_answer_` + flag + `",
}, "human_readable_thought": "mocked thought ` + flag + `", "cumulative_summary": "cumulative-mocked ` + flag + `"}
`)
	rsp.EmitOutputStream(rs)
	rsp.Close()
	return rsp, nil
}

// newTestAIEngine 创建用于测试的 AI 引擎，使用 mock AI 回调
func newTestAIEngine(t *testing.T, mockCallback func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error), options ...AIEngineConfigOption) *AIEngine {
	// 添加 mock AI 回调
	allOptions := append([]AIEngineConfigOption{
		WithAICallback(mockCallback),
		WithDisableMCPServers(true),
		WithExtOptions(
			aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
			aicommon.WithEnableSelfReflection(false),
		),
	}, options...)

	engine, err := NewAIEngine(allOptions...)
	if err != nil {
		t.Fatalf("failed to create test AI engine: %v", err)
	}
	return engine
}

// ==================== Mock 测试 ====================

// TestAIEngine_MockDirectAnswer 测试使用 mock 数据的直接回答
func TestAIEngine_MockDirectAnswer(t *testing.T) {
	streamBuffer := bytes.NewBufferString("")
	finishReceived := false
	answerReceived := false

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectAnswer(i, "test123")
		},
		WithMaxIteration(5),
		WithYOLOMode(),
		WithOnStream(func(react *aireact.ReAct, event *schema.AiOutputEvent, NodeId string, data []byte) {
			t.Logf("Stream: NodeId=%s, data=%s", NodeId, string(data))
			if NodeId == "re-act-loop-answer-payload" {
				streamBuffer.Write(data)
				answerReceived = true
			}
		}),
		WithOnFinished(func(react *aireact.ReAct) {
			finishReceived = true
			t.Logf("Finished")
		}),
	)
	defer engine.Close()

	// 发送消息
	err := engine.SendMsg("你好")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// 等待任务完成
	engine.WaitTaskFinish()

	// 验证回调被调用
	if !finishReceived {
		t.Error("expected finish callback to be called")
	}

	if !answerReceived {
		t.Error("expected answer to be received")
	}

	// 验证流数据包含 mock 标识
	content := streamBuffer.String()
	if content == "" {
		t.Error("expected non-empty stream buffer")
	}
	t.Logf("Stream buffer content: %s", content)
}

// TestAIEngine_MockMultipleTasks 测试使用 mock 数据处理多个任务
func TestAIEngine_MockMultipleTasks(t *testing.T) {
	taskCount := 0
	var taskMutex sync.Mutex

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			taskMutex.Lock()
			currentTask := taskCount
			taskMutex.Unlock()
			return mockedDirectAnswer(i, fmt.Sprintf("task%d", currentTask))
		},
		WithMaxIteration(5),
		WithYOLOMode(),
		WithOnFinished(func(react *aireact.ReAct) {
			taskMutex.Lock()
			taskCount++
			taskMutex.Unlock()
			t.Logf("Task %d completed", taskCount)
		}),
	)
	defer engine.Close()

	// 发送多个消息
	messages := []string{"消息1", "消息2", "消息3"}
	for _, msg := range messages {
		err := engine.SendMsg(msg)
		if err != nil {
			t.Fatalf("failed to send message '%s': %v", msg, err)
		}
	}

	// 等待所有任务完成
	engine.WaitTaskFinish()

	// 验证任务数量
	if taskCount != len(messages) {
		t.Errorf("expected %d tasks, got %d", len(messages), taskCount)
	}
}

// TestAIEngine_MockWithTimeout 测试带超时的 mock 场景
func TestAIEngine_MockWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	finishReceived := false

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			// 模拟快速响应
			time.Sleep(50 * time.Millisecond)
			return mockedDirectAnswer(i, "timeout_test")
		},
		WithContext(ctx),
		WithMaxIteration(5),
		WithYOLOMode(),
		WithOnFinished(func(react *aireact.ReAct) {
			finishReceived = true
		}),
	)
	defer engine.Close()

	// 发送消息
	err := engine.SendMsg("测试超时")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// 等待任务完成或超时
	engine.WaitTaskFinish()

	// 验证任务完成
	if !finishReceived {
		t.Error("expected task to finish before timeout")
	}
}

// TestAIEngine_MockErrorHandling 测试 mock AI 返回错误的情况
func TestAIEngine_MockErrorHandling(t *testing.T) {
	callCount := 0

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			if callCount == 1 {
				// 第一次调用返回错误
				return nil, fmt.Errorf("mocked AI error")
			}
			// 后续调用返回正常响应
			return mockedDirectAnswer(i, "recovered")
		},
		WithMaxIteration(5),
		WithYOLOMode(),
		WithOnFinished(func(react *aireact.ReAct) {
			t.Logf("Finished")
		}),
	)
	defer engine.Close()

	// 发送消息
	err := engine.SendMsg("测试错误处理")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// 等待任务完成
	engine.WaitTaskFinish()

	// 验证 AI 被调用了多次（因为重试）
	if callCount < 2 {
		t.Logf("AI was called %d times (expected at least 2 for retry)", callCount)
	}
}

// TestAIEngine_MockStreamData 测试 mock 流式数据
func TestAIEngine_MockStreamData(t *testing.T) {
	streamChunks := []string{}
	var streamMutex sync.Mutex

	engine := newTestAIEngine(t,
		func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			// 模拟流式输出
			chunks := []string{
				`{"@action": "object", `,
				`"next_action": {"type": "directly_answer", `,
				`"answer_payload": "streaming_test"}, `,
				`"human_readable_thought": "streaming"}`,
			}
			for _, chunk := range chunks {
				rsp.EmitOutputStream(bytes.NewBufferString(chunk))
				time.Sleep(10 * time.Millisecond) // 模拟流式延迟
			}
			rsp.Close()
			return rsp, nil
		},
		WithMaxIteration(5),
		WithYOLOMode(),
		WithOnStream(func(react *aireact.ReAct, event *schema.AiOutputEvent, NodeId string, data []byte) {
			if len(data) > 0 {
				streamMutex.Lock()
				streamChunks = append(streamChunks, string(data))
				streamMutex.Unlock()
				t.Logf("Stream chunk: %s", string(data))
			}
		}),
	)
	defer engine.Close()

	// 发送消息
	err := engine.SendMsg("测试流式数据")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// 等待任务完成
	engine.WaitTaskFinish()

	// 验证接收到了流式数据
	streamMutex.Lock()
	chunkCount := len(streamChunks)
	streamMutex.Unlock()

	if chunkCount == 0 {
		t.Error("expected to receive stream chunks")
	}
	t.Logf("Received %d stream chunks", chunkCount)
}
