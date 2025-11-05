package aiengine

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
)

func TestNewAIEngine(t *testing.T) {
	engine, err := NewAIEngine(
		WithMaxIteration(5),
		WithSessionID("test-session"),
		WithYOLOMode(),
	)
	if err != nil {
		t.Fatalf("failed to create AI engine: %v", err)
	}
	defer engine.Close()

	if engine.config.MaxIteration != 5 {
		t.Errorf("expected MaxIteration to be 5, got %d", engine.config.MaxIteration)
	}

	if engine.config.SessionID != "test-session" {
		t.Errorf("expected SessionID to be 'test-session', got %s", engine.config.SessionID)
	}

	if engine.config.ReviewPolicy != "yolo" {
		t.Errorf("expected ReviewPolicy to be 'yolo', got %s", engine.config.ReviewPolicy)
	}
}

func TestAIEngineConfigOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  []AIEngineConfigOption
		validate func(*AIEngineConfig) error
	}{
		{
			name: "WithMaxIteration",
			options: []AIEngineConfigOption{
				WithMaxIteration(20),
			},
			validate: func(c *AIEngineConfig) error {
				if c.MaxIteration != 20 {
					return fmt.Errorf("expected MaxIteration=20, got %d", c.MaxIteration)
				}
				return nil
			},
		},
		{
			name: "WithYOLOMode",
			options: []AIEngineConfigOption{
				WithYOLOMode(),
			},
			validate: func(c *AIEngineConfig) error {
				if c.ReviewPolicy != "yolo" {
					return fmt.Errorf("expected ReviewPolicy=yolo, got %s", c.ReviewPolicy)
				}
				if c.AllowUserInteract != false {
					return fmt.Errorf("expected AllowUserInteract=false, got %v", c.AllowUserInteract)
				}
				return nil
			},
		},
		{
			name: "WithManualMode",
			options: []AIEngineConfigOption{
				WithManualMode(),
			},
			validate: func(c *AIEngineConfig) error {
				if c.ReviewPolicy != "manual" {
					return fmt.Errorf("expected ReviewPolicy=manual, got %s", c.ReviewPolicy)
				}
				if c.AllowUserInteract != true {
					return fmt.Errorf("expected AllowUserInteract=true, got %v", c.AllowUserInteract)
				}
				return nil
			},
		},
		{
			name: "WithKeywords",
			options: []AIEngineConfigOption{
				WithKeywords("test", "example"),
			},
			validate: func(c *AIEngineConfig) error {
				if len(c.Keywords) != 2 {
					return fmt.Errorf("expected 2 keywords, got %d", len(c.Keywords))
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewAIEngineConfig(tt.options...)
			if err := tt.validate(config); err != nil {
				t.Errorf("validation failed: %v", err)
			}
		})
	}
}

func TestAIEngineEventHandling(t *testing.T) {
	engine, err := NewAIEngine(
		WithMaxIteration(1),
		WithYOLOMode(),
		WithOnEvent(func(event *schema.AiOutputEvent) {
			t.Logf("Received event: type=%s, isStream=%v", event.Type, event.IsStream)
		}),
	)
	if err != nil {
		t.Fatalf("failed to create AI engine: %v", err)
	}
	defer engine.Close()

	// 这个测试不会真正执行 AI 任务，只是验证引擎创建和事件处理设置
	if engine.config.OnEvent == nil {
		t.Error("expected OnEvent callback to be set")
	}
}

func TestAIEngineWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engine, err := NewAIEngine(
		WithContext(ctx),
		WithMaxIteration(1),
		WithYOLOMode(),
	)
	if err != nil {
		t.Fatalf("failed to create AI engine: %v", err)
	}
	defer engine.Close()

	if engine.ctx != ctx {
		t.Error("expected context to be set")
	}
}

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
}

// ExampleNewAIEngine 展示如何创建和使用 AI 引擎
func ExampleNewAIEngine() {
	engine, err := NewAIEngine(
		WithMaxIteration(5),
		WithYOLOMode(),
		WithDebugMode(false),
		WithOnEvent(func(event *schema.AiOutputEvent) {
			fmt.Printf("Event: %s\n", event.Type)
		}),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer engine.Close()

	// 使用引擎...
	fmt.Println("AI Engine created successfully")
	// Output: AI Engine created successfully
}

// ExampleInvokeReAct 展示如何使用便捷函数
func ExampleInvokeReAct() {
	// 注意：这个例子不会真正运行，因为需要 AI 服务配置
	err := InvokeReAct("帮我分析一下代码",
		WithMaxIteration(5),
		WithYOLOMode(),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// 基准测试
func BenchmarkNewAIEngine(b *testing.B) {
	for i := 0; i < b.N; i++ {
		engine, err := NewAIEngine(
			WithMaxIteration(5),
			WithYOLOMode(),
		)
		if err != nil {
			b.Fatalf("failed to create AI engine: %v", err)
		}
		engine.Close()
	}
}

// TestMain 用于设置测试环境
func TestMain(m *testing.M) {
	// 设置测试环境
	os.Exit(m.Run())
}

// 伪代码
// result = {
//     "ok": false,
//     "reason": "",
//     "result": "",
// }
// err = aim.InvokeReAct(
//     "你的任务是编写一个 Yaklang 代码，进行端口扫描",
//     aim.focus("yaklang_code"), // default general
//     aim.iteratiomMax(10),
//     aim.onOutputStream((react, loop, isSystem, isReason, reader) => {
//         // handle event.
//         yakit.Stream(reader)
//     }),
//     aim.onOutput((react, loop, event) => {
//         // 隐藏逻辑，把 event 转换成合理的 ypb.ExecResult 或者想办法让前端触发普通插件下的 AI 输出
//         yakit.Output(event)
//     }),
//     aim.onFinished(loop => {
//         code = loop.Get("full_code")
//         if code != "" {
//             result.ok = true
//             result.result = ""
//         }
//     })
// )
// if err != nil {
//     result.reason = "%v" % err
// }

func TestInvokeReAct(t *testing.T) {
	err := InvokeReAct("你好",
		WithMaxIteration(5),
		WithYOLOMode(),

		WithAIService("aibalance"),
		WithOnStream(func(react *aireact.ReAct, data []byte) {
			t.Logf("Received stream: %s", string(data))
		}),
		WithOnData(func(react *aireact.ReAct, data []byte) {
			t.Logf("Received data: %s", string(data))
		}),
		WithOnFinished(func(react *aireact.ReAct, success bool, result map[string]any) {
			t.Logf("Received finished: success=%v, result=%v", success, result)
		}),
	)
	if err != nil {
		t.Fatalf("failed to invoke ReAct: %v", err)
	}
	time.Sleep(10 * time.Hour)
}
