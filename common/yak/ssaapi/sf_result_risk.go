package ssaapi

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CodeRange struct {
	URL            string `json:"url"`
	StartLine      int64  `json:"start_line"`
	StartColumn    int64  `json:"start_column"`
	EndLine        int64  `json:"end_line"`
	EndColumn      int64  `json:"end_column"`
	SourceCodeLine int64  `json:"source_code_line"`
}

func (c *CodeRange) GetPath() string {
	urlIns := utils.ParseStringToUrl(c.URL)
	return urlIns.Path
}

func (c *CodeRange) JsonString() string {
	jsonString, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(jsonString)
}

const CodeContextLine = 3

func CoverCodeRange(programName string, codeSourceProto string, r memedit.RangeIf) (*CodeRange, string) {
	// url := ""
	source := ""
	ret := &CodeRange{
		URL:            "",
		StartLine:      0,
		StartColumn:    0,
		EndLine:        0,
		EndColumn:      0,
		SourceCodeLine: 0,
	}
	if r == nil {
		return ret, source
	}
	if codeSourceProto == "" {
		codeSourceProto = "ssadb"
	}

	if editor := r.GetEditor(); editor != nil {
		ret.URL = fmt.Sprintf("%s:///%s/%s", codeSourceProto, programName, editor.GetFilename())
		source = editor.GetTextFromRangeContext(r, CodeContextLine)
	}
	if start := r.GetStart(); start != nil {
		ret.StartLine = int64(start.GetLine())
		ret.StartColumn = int64(start.GetColumn())
	}
	if end := r.GetEnd(); end != nil {
		ret.EndLine = int64(end.GetLine())
		ret.EndColumn = int64(end.GetColumn())
	}
	if start := ret.StartLine - CodeContextLine - 1; start > 0 {
		ret.SourceCodeLine = start
	}
	return ret, source
}

func buildSSARisk(
	result *SyntaxFlowResult,
	variable string, index int,
	resultID uint64, runtimeId string,
) *schema.SSARisk {
	progName := result.GetProgramName()
	if progName == "" {
		return nil
	}
	var value *Value
	if vs := result.GetValues(variable); len(vs) <= index {
		return nil
	} else {
		value = vs[index]
	}
	riskCodeRange, CodeFragment := CoverCodeRange(progName, "", value.GetRange())
	rule := result.rule
	newSSARisk := &schema.SSARisk{
		CodeSourceUrl: riskCodeRange.URL,
		CodeRange:     riskCodeRange.JsonString(),
		CodeFragment:  CodeFragment,
		Title:         rule.Title,
		TitleVerbose:  rule.TitleZh,
		Description:   rule.Description,
		RiskType:      rule.RiskType,
		Severity:      string(rule.Severity),
		CVE:           rule.CVE,

		FromRule:    rule.RuleName,
		RuntimeId:   runtimeId,
		IsPotential: false,
		ProgramName: progName,
		// result
		ResultID: resultID,
		Variable: variable,
		Index:    0,

		FunctionName: value.GetFunction().GetName(),
		Line:         riskCodeRange.StartLine,
	}

	// modify info by alertMsg
	alertInfo, _ := result.GetAlertInfo(variable)
	if alertInfo.OnlyMsg {
		if alertInfo.Msg != "" {
			newSSARisk.Details = alertInfo.Msg
		}
	} else {
		// cover info from alertMsg
		if alertInfo.Severity != "" {
			newSSARisk.Severity = string(alertInfo.Severity)
		}
		if alertInfo.CVE != "" {
			newSSARisk.CVE = alertInfo.CVE
		}
		if alertInfo.RiskType != "" {
			newSSARisk.RiskType = alertInfo.RiskType
		}
		if alertInfo.Title != "" {
			newSSARisk.Title = alertInfo.Title
		}
		if alertInfo.Description != "" {
			newSSARisk.TitleVerbose = alertInfo.TitleZh
		}
		if alertInfo.Solution != "" {
			newSSARisk.Solution = alertInfo.Solution
		}
		if alertInfo.Msg != "" {
			newSSARisk.Details = alertInfo.Msg
		}
	}
	return newSSARisk
}

func (r *SyntaxFlowResult) SaveRisk(variable string, result *ssadb.AuditResult) {
	_, ok := r.GetAlertInfo(variable)
	if !ok {
		log.Infof("no alert msg for %s; skip", variable)
		return
	}
	for i := range r.GetValues(variable) {
		ssaRisk := buildSSARisk(r, variable, i, uint64(result.ID), result.TaskID)
		if ssaRisk == nil {
			continue
		}
		err := yakit.CreateSSARisk(consts.GetGormDefaultSSADataBase(), ssaRisk)
		if err != nil {
			log.Errorf("save risk failed: %s", err)
		}
		r.riskMap[fmt.Sprintf("%s-%d", variable, i)] = ssaRisk
	}
}

func (r *SyntaxFlowResult) GetGRPCModelRisk() []*ypb.SSARisk {
	if r == nil {
		return nil
	}

	// load risk from database
	if r.dbResult != nil && len(r.riskMap) != len(r.dbResult.RiskHashs) {
		for name, hash := range r.dbResult.RiskHashs {
			risk, err := yakit.GetSSARiskByHash(ssadb.GetDB(), hash)
			if err != nil {
				log.Errorf("get risk by hash failed: %s", err)
				continue
			}
			r.riskMap[name] = risk
		}
	}
	// transform to grpc model
	if len(r.riskGRPCCache) != len(r.riskMap) {
		r.riskGRPCCache = lo.MapToSlice(r.riskMap, func(name string, risk *schema.SSARisk) *ypb.SSARisk {
			return risk.ToGRPCModel()
		})
	}
	// return
	return r.riskGRPCCache
}

func (r *SyntaxFlowResult) GetRiskByValue(variable string, i int) *schema.SSARisk {
	if r == nil {
		return nil
	}
	return r.GetRisk(fmt.Sprintf("%s-%d", variable, i))
}

func (r *SyntaxFlowResult) GetRisk(name string) *schema.SSARisk {
	if r == nil {
		return nil
	}
	if r, ok := r.riskMap[name]; ok {
		return r
	}
	// from db
	if r.dbResult != nil {
		if hash, ok := r.dbResult.RiskHashs[name]; ok {
			risk, err := yakit.GetSSARiskByHash(ssadb.GetDB(), hash)
			if err != nil {
				log.Errorf("get risk by hash failed: %s", err)
				return nil
			}
			r.riskMap[name] = risk
			return risk
		}
	}
	return nil
}
