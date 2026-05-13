package loop_vuln_verify

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// Allowed reproducibility categories.
const (
	categoryReproducible      = "reproducible"
	categoryNotReproducible   = "not_reproducible"
	categoryRequiresEnvAccess = "requires_env_access"
)

func buildAssessReproducibilityAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"assess_reproducibility",
		"评估安全发现能否在实际环境中被复现。对于 SSA Risk，须在完成阶段 0 源代码检查后再调用本动作；对于假设/PoC 输入，在阶段 1 直接调用即可。",
		[]aitool.ToolOption{
			aitool.WithStringParam("category",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"reproducible | not_reproducible | requires_env_access"),
			),
			aitool.WithStringParam("finding_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"漏洞类型，例如 sql_injection / xss / rce / ssrf / auth_bypass / "+
						"idor / path_traversal / deserialization / info_disclosure / code_quality / "+
						"logic_flaw / memory_corruption / other"),
			),
			aitool.WithStringParam("reasoning",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"说明为何将该发现归入所选类别。"+
						"若为 not_reproducible，须明确指出为何不存在运行时安全影响。"),
			),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "reasoning", AINodeId: "re-act-loop-thought", ContentType: aicommon.TypeTextMarkdown},
		},
		verifyAssessReproducibility,
		handleAssessReproducibility,
	)
}

func verifyAssessReproducibility(_ *reactloops.ReActLoop, action *aicommon.Action) error {
	category := action.GetString("category")
	switch category {
	case categoryReproducible, categoryNotReproducible, categoryRequiresEnvAccess:
		// valid
	default:
		return fmt.Errorf("category must be one of: %s | %s | %s, got %q",
			categoryReproducible, categoryNotReproducible, categoryRequiresEnvAccess, category)
	}
	if action.GetString("finding_type") == "" {
		return utils.Error("finding_type is required")
	}
	if action.GetString("reasoning") == "" {
		return utils.Error("reasoning is required")
	}
	return nil
}

func handleAssessReproducibility(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
	category := action.GetString("category")
	findingType := action.GetString("finding_type")
	reasoning := action.GetString("reasoning")

	loop.Set(keyReproducibilityVerdict, category)
	loop.GetInvoker().AddToTimeline("assess_reproducibility", fmt.Sprintf(
		"category=%s finding_type=%s reasoning=%s", category, findingType,
		utils.ShrinkTextBlock(reasoning, 150)))

	switch category {
	case categoryNotReproducible:
		loop.Set(keyVerificationPhase, "concluded_not_reproducible")
		op.Feedback(fmt.Sprintf(
			"评估结果：不可复现（finding_type=%s）。\n"+
				"原因：%s\n"+
				"下一步：调用 directly_answer，说明该发现无法在实际环境中演示。",
			findingType, reasoning))

	case categoryRequiresEnvAccess:
		loop.Set(keyVerificationPhase, "phase2_reachability")
		op.Feedback(fmt.Sprintf(
			"评估结果：需要环境访问（finding_type=%s）。\n"+
				"原因：%s\n"+
				"下一步：调用 check_target_reachability，传入目标 URL。",
			findingType, reasoning))

	case categoryReproducible:
		loop.Set(keyVerificationPhase, "phase2_reachability")
		op.Feedback(fmt.Sprintf(
			"评估结果：可复现（finding_type=%s）。\n"+
				"原因：%s\n"+
				"下一步：调用 check_target_reachability，传入目标 URL。",
			findingType, reasoning))
	}

	op.Continue()
}
