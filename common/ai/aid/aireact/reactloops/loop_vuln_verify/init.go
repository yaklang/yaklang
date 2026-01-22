package loop_vuln_verify

import (
	"bytes"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveDataTemplate string

//go:embed prompts/output_example.txt
var outputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_VULN_VERIFY,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			// 创建验证状态
			state := NewVerifyState()

			preset := []reactloops.ReActLoopOption{
				// 基础配置
				reactloops.WithMaxIterations(30),
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(true),      // 允许使用 bash/grep 等工具
				reactloops.WithAllowUserInteract(false), // 不允许人工协助

				// Spin 检测配置
				reactloops.WithSameActionTypeSpinThreshold(4),
				reactloops.WithSameLogicSpinThreshold(3),
				reactloops.WithEnableSelfReflection(true),

				// 初始化
				reactloops.WithInitTask(buildInitTask(r, state)),

				// Prompt 配置
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(buildReactiveData(state)),

				// 注册 Actions
				setVulnContextAction(r, state),
				readCodeAction(r, state),
				traceBackwardAction(r, state),
				recordFilterAction(r, state),
				concludeAction(r, state),
			}

			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_VULN_VERIFY, r, preset...)
		},
		// 注册元数据
		reactloops.WithLoopDescription("漏洞验证模式：验证潜在漏洞点是否真实可利用，追踪 Source→Sink 数据流，分析过滤有效性。"),
		reactloops.WithLoopUsagePrompt(`当需要验证一个潜在漏洞是否真实存在时使用此流程。
AI 会追踪数据流、分析过滤函数、判断可利用性，最终给出确认/安全/需人工确认的结论。`),
		reactloops.WithLoopOutputExample(`
* 当需要验证潜在漏洞时:
  {"@action": "vuln_verify", "human_readable_thought": "需要验证这个SQL注入点是否真实可利用"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_VULN_VERIFY, err)
	}
}

// buildInitTask 创建初始化任务
func buildInitTask(r aicommon.AIInvokeRuntime, state *VerifyState) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		log.Infof("[*] VulnVerify loop initialized, waiting for vulnerability context")

		// 解析用户输入，尝试提取漏洞信息
		userInput := task.GetUserInput()
		if userInput != "" {
			r.AddToTimeline("init", "漏洞验证任务开始，用户输入: "+utils.ShrinkTextBlock(userInput, 200))
		}

		return nil
	}
}

// buildReactiveData 构建动态数据
func buildReactiveData(state *VerifyState) func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
	return func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
		renderMap := state.ToMap()
		renderMap["Nonce"] = nonce
		renderMap["FeedbackMessages"] = feedbacker.String()

		return utils.RenderTemplate(reactiveDataTemplate, renderMap)
	}
}
