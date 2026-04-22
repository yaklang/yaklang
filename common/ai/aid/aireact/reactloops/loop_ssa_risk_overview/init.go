package loop_ssa_risk_overview

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
		schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW,
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
				sfu.WithReloadSSARiskOverviewAction(r),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					fb := strings.TrimSpace(feedbacker.String())
					return utils.RenderTemplate(reactiveData, map[string]any{
						"Preface":          loop.Get("ssa_risk_overview_preface"),
						"TotalHint":        loop.Get("ssa_risk_total_hint"),
						"FeedbackMessages": fb,
						"Nonce":            nonce,
					})
				}),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SSA Risk Overview"),
		reactloops.WithVerboseNameZh("IRify · SSA 风险总览"),
		reactloops.WithLoopDescription("IRify SSA risk triage: query, filter, and summarize many static-analysis findings across programs/runtimes; use for bucketing, prioritization, and next steps. For a single-finding walkthrough, switch to the SSA risk review focus mode instead."),
		reactloops.WithLoopDescriptionZh("IRify SSA 风险总览：在大量 SSA 风险记录上做检索、过滤与聚类，支撑优先级与后续处置；需对单条做证据级解读时，请使用「SSA 风险解读」模式。"),
		reactloops.WithLoopUsagePrompt("Use when triaging many SSA risks in IRify: filter, bucket, and prioritize. For one risk at a time use ssa_risk_review. Prefer attachment irify_ssa_risks_filter (filter_json / runtime_id / program_name) or loop var ssa_risks_filter_json. Use reload_ssa_risk_overview to refresh by limit, search, filters."),
		reactloops.WithLoopOutputExample(`
* 总览当前项目风险：
  {"@action": "ssa_risk_overview", "human_readable_thought": "需要归纳并筛选 SSA 风险列表"}
* 多轮中按条件刷新数据库列表：
  {"@action": "reload_ssa_risk_overview", "human_readable_thought": "用户收紧了 program 过滤", "limit": 40, "program_name": "demo"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW, err)
	}
}
