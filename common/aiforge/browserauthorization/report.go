package browserauthorization

import "github.com/yaklang/yaklang/common/browser"

type authorizationReportCase struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Outcome string `json:"outcome,omitempty"`
	Status  int    `json:"status,omitempty"`
}

type authorizationReport struct {
	Version           int                               `json:"version"`
	WorkspaceID       string                            `json:"workspaceId"`
	PlanID            string                            `json:"planId"`
	ExecutionID       string                            `json:"executionId"`
	Mode              string                            `json:"mode"`
	Summary           string                            `json:"summary"`
	RequestCount      int                               `json:"requestCount"`
	Cases             []authorizationReportCase         `json:"cases"`
	Evidence          []authorizationReviewEvidence     `json:"evidence"`
	IndependentReview authorizationReviewCommit         `json:"independentReview"`
	Deterministic     authorizationDeterministicFinding `json:"deterministic"`
	FactAgreement     string                            `json:"factAgreement"`
	PolicyBoundary    string                            `json:"policyBoundary"`
	Limitations       []string                          `json:"limitations"`
	NextActions       []string                          `json:"nextActions"`
	EvidenceScope     string                            `json:"evidenceScope"`
}

func buildAuthorizationReport(
	workspace browser.ExtensionAuthorizationWorkspace,
	reconciliation authorizationVerdictReconciliation,
) authorizationReport {
	report := authorizationReport{
		Version:           2,
		WorkspaceID:       workspace.ID,
		Mode:              workspace.Mode,
		Cases:             []authorizationReportCase{},
		Evidence:          append([]authorizationReviewEvidence(nil), reconciliation.Deterministic.Evidence...),
		IndependentReview: reconciliation.IndependentReview,
		Deterministic:     reconciliation.Deterministic,
		FactAgreement:     reconciliation.FactAgreement,
		PolicyBoundary:    reconciliation.PolicyBoundary,
		Limitations:       append([]string(nil), reconciliation.IndependentReview.Assessment.Limitations...),
		NextActions:       []string{},
		EvidenceScope:     "仅适用于本工作区绑定的身份、页面文档、请求基线与执行时刻",
	}
	execution := workspace.Execution
	if execution == nil {
		report.Summary = "当前工作区尚无可报告的执行事实"
		report.Limitations = append(report.Limitations, "尚未执行经过验证的固定测试计划")
		report.NextActions = append(report.NextActions, "验证并执行当前计划后重新开始独立盲审")
		return report
	}

	report.PlanID = execution.PlanID
	report.ExecutionID = execution.ID
	report.RequestCount = execution.RequestCount
	for _, current := range execution.Cases {
		item := authorizationReportCase{
			ID:    current.ID,
			State: current.State,
		}
		if current.Result != nil {
			item.Outcome = current.Result.Outcome
			item.Status = current.Result.Status
		}
		report.Cases = append(report.Cases, item)
	}

	switch reconciliation.Deterministic.Observation {
	case "cross-identity-resource-reproduction-confirmed":
		report.Summary = "确定性证据确认至少一个交叉请求复现了目标身份的业务数据；是否违反授权策略由独立审查结论单独表达"
	case "cross-identity-response-match-observed":
		report.Summary = "确定性比较观察到交叉响应与目标身份基线吻合，但没有独立稳定字段证明资源归属"
	case "cross-identity-probes-blocked":
		report.Summary = "本次固定矩阵中的两个交叉身份请求均被服务端拒绝"
	case "low-identity-operation-state-change-confirmed":
		report.Summary = "确定性后置状态证据确认低权限身份发起操作后业务状态发生变化；策略意图仍由独立审查解释"
	case "low-identity-operation-response-accepted":
		report.Summary = "低权限操作请求被接受或复现高权限响应，但缺少独立后置状态证明"
	case "low-identity-operation-blocked":
		report.Summary = "本次低权限特权操作请求被服务端拒绝"
	case "control-baselines-invalid":
		report.Summary = "正常对照未建立有效基线，本次交叉结果没有解释基础"
	case "execution-partial":
		report.Summary = "固定测试矩阵未完整执行，本次事实证据不足"
	default:
		report.Summary = "当前确定性请求与响应事实不足以形成稳定观察"
	}

	assessment := reconciliation.IndependentReview.Assessment
	if workspace.Mode == "horizontal" && assessment.IdentityRelationship != "same-privilege-proven" {
		report.Limitations = append(
			report.Limitations,
			"尚未证明两个身份权限等价，跨身份访问事实不能自动命名为水平授权缺陷",
		)
	}
	if reconciliation.FactAgreement == "disagree" {
		report.Limitations = append(
			report.Limitations,
			"独立审查与确定性事实分级不一致，应复核差异路径、报文和身份关系",
		)
	}
	switch assessment.PolicyAssessment {
	case "violation-supported":
		report.NextActions = append(
			report.NextActions,
			"核对明确的角色、租户或资源归属策略后修复服务端鉴权",
			"修复后用新工作区重新建立身份基线并复测",
		)
	case "expected-access-plausible":
		report.NextActions = append(
			report.NextActions,
			"确认角色层级或管理权限是否明确允许本次跨身份访问",
		)
	case "protected":
		report.NextActions = append(
			report.NextActions,
			"如需扩大覆盖范围，为另一个业务资源或操作建立独立工作区",
		)
	case "requires-policy":
		report.NextActions = append(
			report.NextActions,
			"补充两个账户的角色、租户、资源归属或明确授权策略后再下业务结论",
		)
	default:
		report.NextActions = append(
			report.NextActions,
			"复核最小差异证据与身份关系，保留当前不确定性",
		)
	}
	return report
}
