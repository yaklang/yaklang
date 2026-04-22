package loop_syntaxflow_scan

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(outputExample + sfu.ReflectionOutputSharedAppendix),
				sfu.WithReloadSyntaxFlowScanSessionAction(r),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					fb := strings.TrimSpace(feedbacker.String())
					return utils.RenderTemplate(reactiveData, map[string]any{
						"Preface":          loop.Get("sf_scan_review_preface"),
						"TaskID":           loop.Get("sf_scan_task_id"),
						"SessionMode":      loop.Get("sf_scan_session_mode"),
						"FeedbackMessages": fb,
						"Nonce":            nonce,
					})
				}),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SyntaxFlow Scan"),
		reactloops.WithVerboseNameZh("IRify · SyntaxFlow 扫描"),
		reactloops.WithLoopDescription("IRify SyntaxFlow scan session: bind a scan by task_id to review progress, linked SSA risks for the runtime, and rule results from the project DB. Use session_mode=start for how to launch a new scan in Yakit; the actual scan still runs in Yakit/IRify."),
		reactloops.WithLoopDescriptionZh("IRify SyntaxFlow 扫描会话：通过 task_id 附着已有扫描任务，结合数据库查看进度、同 runtime 的 SSA 风险列表与规则命中摘要；也可使用 session_mode=start 获取在 Yakit 中发起新扫描的指引。完整扫描与引擎执行仍在 Yakit/IRify。"),
		reactloops.WithLoopUsagePrompt("Use when interpreting a SyntaxFlow scan: task progress, SSA risks, and results. Attach irify_syntaxflow/task_id (UUID), or session_mode=start for how to start a scan in Yakit. Orchestrators may inject WithVar(syntaxflow_task_id). Call reload_syntaxflow_scan_session when task_id changes. Scan execution remains in Yakit/IRify."),
		reactloops.WithLoopOutputExample(`
* SyntaxFlow 扫描会话：
  {"@action": "syntaxflow_scan", "human_readable_thought": "需要解读已附着的 SyntaxFlow 扫描任务（task_id 由附件或 Loop 变量提供）"}
* 重新加载另一扫描任务摘要：
  {"@action": "reload_syntaxflow_scan_session", "human_readable_thought": "用户提供了新的 task_id", "task_id": "550e8400-e29b-41d4-a716-446655440000"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, err)
	}
}
