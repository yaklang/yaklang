package loop_syntaxflow_audit_analyst

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	ssaovw "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_risk_overview"
	ssarev "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_risk_review"
	sfscan "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_syntaxflow_scan"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	persistentInstruction = `You are the SyntaxFlow audit analyst sub-loop. Use SSA project file tools and SSA risk data; avoid blind filesystem grep unless ssa-grep is justified.
Each iteration: refresh risk context with reload_ssa_risk_overview or reload_ssa_risk, drill into code with read_ssa_project_file (then actually invoke ssa-read-file tool from the tool panel), and record concrete findings.`

	reactiveData = `### Audit analyst state
- risk overview preface: {{ .Overview }}
- last feedback: {{ .FeedbackMessages }}
- nonce: {{ .Nonce }}
`

	outputExample = `{"@action":"reload_ssa_risk","risk_id":123,"get_full_code":1,"human_readable_thought":"need evidence"}`
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_AUDIT_ANALYST,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(outputExample + sfu.ReflectionOutputSharedAppendix),
				ssaovw.WithReloadSSARiskOverviewAction(r),
				ssarev.WithReloadSSARiskAction(r),
				sfscan.WithReadSSAProjectFileAction(r),
				ssarev.WithDeriveRuleSeedFromRiskAction(r),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					return utils.RenderTemplate(reactiveData, map[string]any{
						"Overview":         loop.Get("ssa_risk_overview_preface"),
						"FeedbackMessages": strings.TrimSpace(feedbacker.String()),
						"Nonce":            nonce,
					})
				}),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_AUDIT_ANALYST, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SyntaxFlow Audit Analyst"),
		reactloops.WithVerboseNameZh("IRify · SyntaxFlow 审计分析师"),
		reactloops.WithLoopDescription("Sub-loop focused on SSA-backed evidence reading and-risk triage during SyntaxFlow audits."),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_AUDIT_ANALYST, err)
	}
}
