package loop_vuln_verify

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reactive_data.txt
var reactiveDataTpl string

//go:embed prompts/reflection_output_example.txt
var outputExample string

// Loop state keys.
const (
	keyFindingDescription     = "finding_description"
	keyTargetInfo             = "target_info"
	keyReproducibilityVerdict = "reproducibility_verdict"
	keyReachabilityStatus     = "reachability_status"
	keyVerificationPhase      = "verification_phase"
	keyEvidenceCount          = "evidence_count"
	keyEvidenceJSON           = "collected_evidence_json"
	keyRecentEvidence         = "recent_evidence"
	keyFinalVerdict           = "final_verdict"
	keyVerdictDelivered       = "verdict_delivered"

	// SSA Risk: when the user references an SSA Risk, only the numeric ID is stored.
	// All further details (program, file path, risk type, etc.) are fetched via the
	// ssa-risk tool by the AI in Phase 0.
	keySSARiskIDOnly = "ssa_risk_id_only" // "true" when a Risk ID was identified
	keySSARiskID     = "ssa_risk_id"      // numeric Risk ID
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_VULN_VERIFY,
		func(invoker aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				// Allow tool calls so the LLM can send HTTP requests, run code, etc. during verification.
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowUserInteract(invoker.GetConfig().GetAllowUserInteraction()),

				reactloops.WithMaxIterations(100),

				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(buildReactiveData),

				// Override directly_answer to enforce structured verdict output.
				reactloops.WithOverrideLoopAction(loopActionConclude),

				buildAssessReproducibilityAction(invoker),
				buildCheckReachabilityAction(invoker),
				buildRecordEvidenceAction(invoker),

				reactloops.WithInitTask(buildInitTask(invoker)),
				buildOnPostIterationHook(invoker),

				reactloops.WithSameActionTypeSpinThreshold(2),
				reactloops.WithEnableSelfReflection(true),
				reactloops.WithPeriodicVerificationInterval(4),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_VULN_VERIFY, invoker, preset...)
		},
		reactloops.WithLoopDescription("Security vulnerability verification: assess reproducibility, check target reachability, execute verification steps with available tools, and deliver a CONFIRMED / NOT_CONFIRMED / INCONCLUSIVE verdict."),
		reactloops.WithLoopDescriptionZh("漏洞验证模式：评估可复现性→检查目标可达性→利用可用工具执行验证→输出 CONFIRMED/NOT_CONFIRMED/INCONCLUSIVE 结论。适用于验证静态分析结果（ssaRisk）、假设性风险、PoC/Exploit 测试、漏洞复现等场景。"),
		reactloops.WithVerboseName("Vuln Verify"),
		reactloops.WithVerboseNameZh("漏洞验证"),
		reactloops.WithLoopUsagePrompt("Use when the user wants to verify whether a security finding (ssaRisk, hypothesis, PoC, exploit) can be reproduced against a specific target environment (local or remote). The loop filters out non-reproducible code-quality issues, checks target reachability, executes verification steps using available tools, records evidence, and delivers a structured verdict."),
		reactloops.WithLoopOutputExample(`
* When user wants to verify a security risk/vulnerability:
  {"@action": "vuln_verify", "human_readable_thought": "I need to verify whether this SQL injection risk can be reproduced in the target environment"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_VULN_VERIFY, err)
	}
}

func buildReactiveData(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
	evidenceCount := loop.Get(keyEvidenceCount)
	if evidenceCount == "" {
		evidenceCount = "0"
	}
	return utils.RenderTemplate(reactiveDataTpl, map[string]any{
		"Nonce":                  nonce,
		"FeedbackMessages":       feedbacker.String(),
		"FindingDescription":     loop.Get(keyFindingDescription),
		"TargetInfo":             loop.Get(keyTargetInfo),
		"ReproducibilityVerdict": loop.Get(keyReproducibilityVerdict),
		"ReachabilityStatus":     loop.Get(keyReachabilityStatus),
		"VerificationPhase":      loop.Get(keyVerificationPhase),
		"EvidenceCount":          evidenceCount,
		"RecentEvidence":         loop.Get(keyRecentEvidence),
		"SSARiskIDOnly":          loop.Get(keySSARiskIDOnly) == "true",
		"SSARiskID":              loop.Get(keySSARiskID),
	})
}

// buildInitTask uses LiteForge for intent recognition to extract the finding
// description, target info, and optional SSA Risk ID from the user's input.
func buildInitTask(invoker aicommon.AIInvokeRuntime) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := strings.TrimSpace(task.GetUserInput())
		emitter := invoker.GetConfig().GetEmitter()

		if userInput == "" {
			emitter.EmitThoughtStream(task.GetIndex(),
				"No input provided. Please describe the vulnerability or risk to verify and specify the target environment.")
			op.Done()
			return
		}

		// Initialise counters.
		loop.Set(keyEvidenceCount, "0")
		loop.Set(keyEvidenceJSON, "[]")
		loop.Set(keyVerificationPhase, "phase1_assess")

		promptTpl := `从用户输入中提取漏洞验证所需的关键信息：

1. finding_description（待验证的漏洞/风险描述）：
   可能是 ssaRisk 内容、CVE 描述、PoC 说明、漏洞假设、静态分析发现等。
   保留原始上下文，不要裁剪重要细节。

2. target_info（目标环境信息）：
   目标 URL、IP 地址、服务端口、本地/远程环境描述等。
   若用户未提供明确目标，填写 "not_provided"。

3. ssa_risk_id（SSA Risk 数字编号）：
   如果用户提到了 SSA Risk 的数字编号（例如"SSA Risk 3450"、"风险 ID 3450"、"risk #3450"、"第3450条风险"、"verify risk 3450"等各种表述），提取该数字。
   仅当用户的意图是针对某个已存在的 SSA Risk 编号进行验证时才填写。
   若用户没有提到任何 SSA Risk 编号，填空字符串。

4. initial_analysis（初步分析）：
   简要说明这个验证任务的性质、可能的漏洞类型和验证思路。

<|USER_INPUT_{{ .nonce }}|>
{{ .userInput }}
<|USER_INPUT_END_{{ .nonce }}|>`

		renderedPrompt := utils.MustRenderTemplate(promptTpl, map[string]any{
			"nonce":     utils.RandStringBytes(4),
			"userInput": userInput,
		})

		extracted, err := invoker.InvokeSpeedPriorityLiteForge(
			task.GetContext(),
			"vuln-verify-input-parse",
			renderedPrompt,
			[]aitool.ToolOption{
				aitool.WithStringParam("finding_description",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("漏洞/风险的描述，包括漏洞类型、位置、触发条件等")),
				aitool.WithStringParam("target_info",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("目标环境信息，如 URL、IP、端口；若未提供则填 not_provided")),
				aitool.WithStringParam("ssa_risk_id",
					aitool.WithParam_Description("用户提及的 SSA Risk 数字编号，未提及则为空")),
				aitool.WithStringParam("initial_analysis",
					aitool.WithParam_Description("对此验证任务的初步分析和思路")),
			},
			aicommon.WithGeneralConfigStreamableFieldWithNodeId("re-act-loop-thought", "initial_analysis"),
		)
		if err != nil {
			log.Warnf("[VulnVerify] init LiteForge parse failed: %v, using raw input as finding", err)
			loop.Set(keyFindingDescription, userInput)
			loop.Set(keyTargetInfo, "not_provided")
		} else {
			findingDesc := strings.TrimSpace(extracted.GetString("finding_description"))
			targetInfo := strings.TrimSpace(extracted.GetString("target_info"))
			initialAnalysis := strings.TrimSpace(extracted.GetString("initial_analysis"))
			ssaRiskID := strings.TrimSpace(extracted.GetString("ssa_risk_id"))

			if findingDesc == "" {
				findingDesc = userInput
			}
			loop.Set(keyFindingDescription, findingDesc)
			loop.Set(keyTargetInfo, targetInfo)

			if ssaRiskID != "" {
				// The input references an SSA Risk by ID. All details (program name,
				// file path, vulnerability type, etc.) will be fetched by the AI via
				// the ssa-risk tool in Phase 0.
				loop.Set(keySSARiskIDOnly, "true")
				loop.Set(keySSARiskID, ssaRiskID)
				loop.Set(keyVerificationPhase, "phase0_ssa_inspect")

				invoker.AddToTimeline("vuln_verify_ssa_id",
					"SSA Risk ID detected: "+ssaRiskID+". AI must call ssa-risk tool first.")
				if emitter != nil {
					emitter.EmitThoughtStream(task.GetIndex(),
						"SSA Risk ID detected: "+ssaRiskID+". Will call ssa-risk tool to fetch full risk details before verification.")
				}
			}

			if initialAnalysis != "" {
				invoker.AddToTimeline("vuln_verify_init", "Initial analysis: "+initialAnalysis)
			}
		}

		invoker.AddToTimeline("vuln_verify_start", "Vulnerability verification session started. Finding: "+
			utils.ShrinkTextBlock(loop.Get(keyFindingDescription), 200))

		op.Continue()
	}
}
