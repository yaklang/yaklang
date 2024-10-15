package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (r *SyntaxFlowResult) SaveRisk(variable string, resultID uint, taskID string) {
	alertMsg, ok := r.GetAlertEx(variable)
	if !ok {
		log.Infof("no alert msg for %s; skip", variable)
		return
	}

	rule := r.rule
	// risk := yakit.CreateRisk("",
	opts := []yakit.RiskParamsOpt{
		yakit.WithRiskParam_RuntimeId(taskID),
		yakit.WithRiskParam_FromScript(rule.RuleName),
		yakit.WithRiskParam_Title(rule.Title),
		yakit.WithRiskParam_TitleVerbose(rule.TitleZh),
		yakit.WithRiskParam_Description(rule.Description),
		yakit.WithRiskParam_Severity(string(rule.Severity)),
		yakit.WithRiskParam_CVE(rule.CVE),

		// TODO: syntaxflow rule set  risk type
		// yakit.WithRiskParam_RiskType(string(rule.Purpose)),
	}

	// modify info by alertMsg
	if alertMsg.OnlyMsg {
		if alertMsg.Msg != "" {
			opts = append(opts, yakit.WithRiskParam_Details(alertMsg.Msg))
		}
	} else {
		// cover info from alertMsg
		if alertMsg.Severity != "" {
			opts = append(opts, yakit.WithRiskParam_Severity(string(alertMsg.Severity)))
		}
		if alertMsg.CVE != "" {
			opts = append(opts, yakit.WithRiskParam_CVE(alertMsg.CVE))
		}
		if alertMsg.Purpose != "" {
			// TODO: syntaxflow rule set  risk type
			// risk.RiskType = string(alertMsg.Purpose)
		}
		if alertMsg.Title != "" {
			opts = append(opts, yakit.WithRiskParam_Title(alertMsg.Title))
		}
		if alertMsg.Description != "" {
			opts = append(opts, yakit.WithRiskParam_TitleVerbose(alertMsg.TitleZh))
		}
		if alertMsg.Solution != "" {
			opts = append(opts, yakit.WithRiskParam_Solution(alertMsg.Solution))
		}
		if alertMsg.Msg != "" {
			opts = append(opts, yakit.WithRiskParam_Details(alertMsg.Msg))
		}
	}

	risk := yakit.CreateRisk("", opts...)
	risk.ResultID = resultID
	risk.Variable = variable
	if err := yakit.SaveRisk(risk); err != nil {
		log.Errorf("save risk failed: %s", err)
	}
}
