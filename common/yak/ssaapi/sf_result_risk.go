package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *SyntaxFlowResult) SaveRisk(variable string, result *ssadb.AuditResult) {
	alertInfo, ok := r.GetAlertInfo(variable)
	if !ok {
		log.Infof("no alert msg for %s; skip", variable)
		return
	}

	rule := r.rule
	// risk := yakit.CreateRisk("",
	opts := []yakit.RiskParamsOpt{
		yakit.WithRiskParam_RuntimeId(result.TaskID),
		yakit.WithRiskParam_FromScript(rule.RuleName),
		yakit.WithRiskParam_Title(rule.Title),
		yakit.WithRiskParam_TitleVerbose(rule.TitleZh),
		yakit.WithRiskParam_Description(rule.Description),
		yakit.WithRiskParam_Severity(string(rule.Severity)),
		yakit.WithRiskParam_CVE(rule.CVE),
		yakit.WithRiskParam_RiskType(string(rule.RiskType)),
	}

	// modify info by alertMsg
	if alertInfo.OnlyMsg {
		if alertInfo.Msg != "" {
			opts = append(opts, yakit.WithRiskParam_Details(alertInfo.Msg))
		}
	} else {
		// cover info from alertMsg
		if alertInfo.Severity != "" {
			opts = append(opts, yakit.WithRiskParam_Severity(string(alertInfo.Severity)))
		}
		if alertInfo.CVE != "" {
			opts = append(opts, yakit.WithRiskParam_CVE(alertInfo.CVE))
		}
		if alertInfo.Purpose != "" {
			opts = append(opts, yakit.WithRiskParam_RiskType(string(rule.RiskType)))
		}
		if alertInfo.Title != "" {
			opts = append(opts, yakit.WithRiskParam_Title(alertInfo.Title))
		}
		if alertInfo.Description != "" {
			opts = append(opts, yakit.WithRiskParam_TitleVerbose(alertInfo.TitleZh))
		}
		if alertInfo.Solution != "" {
			opts = append(opts, yakit.WithRiskParam_Solution(alertInfo.Solution))
		}
		if alertInfo.Msg != "" {
			opts = append(opts, yakit.WithRiskParam_Details(map[string]string{
				"message": alertInfo.Msg,
			}))
		}
	}

	risk := yakit.CreateRisk("", opts...)
	risk.ResultID = result.ID
	risk.Variable = variable
	risk.ProgramName = result.ProgramName
	if err := yakit.SaveRisk(risk); err != nil {
		log.Errorf("save risk failed: %s", err)
		return
	}
	r.riskMap[variable] = risk
}

func (r *SyntaxFlowResult) GetGRPCModelRisk() []*ypb.Risk {
	if r == nil || len(r.riskMap) == 0 {
		return nil
	}
	if len(r.riskGRPCCache) != len(r.riskMap) {
		r.riskGRPCCache = lo.MapToSlice(r.riskMap, func(name string, risk *schema.Risk) *ypb.Risk {
			return risk.ToGRPCModel()
		})
	}
	return r.riskGRPCCache
}

func (r *SyntaxFlowResult) GetRisk(name string) *schema.Risk {
	if r == nil || r.riskMap == nil {
		return nil
	}
	if r, ok := r.riskMap[name]; ok {
		return r
	}
	return nil
}
