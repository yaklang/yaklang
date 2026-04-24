package loop_ssa_risk_review

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_syntaxflow_scan"
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
		schema.AI_REACT_LOOP_NAME_SSA_RISK_REVIEW,
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
				loop_syntaxflow_scan.WithSetSSARiskReviewTargetAction(r),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					fb := strings.TrimSpace(feedbacker.String())
					rid := loop.Get("ssa_risk_id")
					return utils.RenderTemplate(reactiveData, map[string]any{
						"RiskID":           rid,
						"FeedbackMessages": fb,
						"Nonce":            nonce,
					})
				}),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_RISK_REVIEW, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SSA Risk Review"),
		reactloops.WithVerboseNameZh("IRify · SSA 风险解读"),
		reactloops.WithLoopDescription("IRify single SSA risk review: load one risk by risk_id, walk through code evidence and dataflow context, propose remediation, and record suggested disposition. PoC generation is not the default; use batch overview for many risks."),
		reactloops.WithLoopDescriptionZh("IRify 单条 SSA 风险解读：按 risk_id 拉取并解读单条静态分析发现，结合代码证据与数据流说明风险成因、修复建议与处置；默认不用于 PoC 生成。批量风险请用「SSA 风险总览」。"),
		reactloops.WithLoopUsagePrompt("Use when deep-diving one SSA risk: require risk_id. Prefer loading the record via the ssa-risk tool. Call set_ssa_risk_review_target to switch risk_id mid-session. Do not confuse with ssa_risk_overview."),
		reactloops.WithLoopOutputExample(`
* 解读单条 SSA 风险：
  {"@action": "ssa_risk_review", "human_readable_thought": "需要对 risk_id=123 的 SSA 风险做解读与处置建议"}
* 切换到另一条风险再解读：
  {"@action": "set_ssa_risk_review_target", "human_readable_thought": "用户改看另一条", "risk_id": 456}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SSA_RISK_REVIEW, err)
	}
}
