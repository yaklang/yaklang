package ssaapi

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
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

func CoverCodeRange(r *memedit.Range) (*CodeRange, string) {
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

	if editor := r.GetEditor(); editor != nil {
		// if codeSourceProto is empty, url is pure path
		ret.URL = editor.GetUrl()
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
	variable string, index int, value *Value,
) *schema.SSARisk {
	progName := result.GetProgramName()
	if progName == "" {
		return nil
	}
	riskCodeRange, CodeFragment := CoverCodeRange(value.GetRange())
	rule := result.rule
	newSSARisk := &schema.SSARisk{
		CodeSourceUrl: utils.EscapeInvalidUTF8Byte([]byte(riskCodeRange.URL)),
		CodeRange:     riskCodeRange.JsonString(),
		CodeFragment:  utils.EscapeInvalidUTF8Byte([]byte(CodeFragment)),
		Title:         rule.Title,
		TitleVerbose:  rule.TitleZh,
		Description:   rule.Description,
		Solution:      rule.Solution,
		RiskType:      rule.RiskType,
		Severity:      rule.Severity,
		CVE:           rule.CVE,
		CWE:           rule.CWE,

		FromRule:    rule.RuleName,
		IsPotential: false,
		ProgramName: progName,
		// result
		Variable: variable,
		Index:    int64(index),

		Line:     riskCodeRange.StartLine,
		Language: result.program.GetLanguage(),
	}
	if fun := value.GetFunction(); fun != nil {
		newSSARisk.FunctionName = utils.EscapeInvalidUTF8Byte([]byte(fun.GetName()))
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
			newSSARisk.Severity = alertInfo.Severity
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
		if alertInfo.TitleZh != "" {
			newSSARisk.TitleVerbose = alertInfo.TitleZh
		}
		if alertInfo.Description != "" {
			newSSARisk.Description = alertInfo.Description
		}
		if alertInfo.Solution != "" {
			newSSARisk.Solution = alertInfo.Solution
		}
		if alertInfo.Msg != "" {
			newSSARisk.Details = alertInfo.Msg
		}
	}

	newSSARisk.RiskFeatureHash = utils.CalcSha1(
		// SSA Feature
		newSSARisk.FunctionName,
		value.String(),
		// SyntaxFlow Rule Feature
		newSSARisk.FromRule,
		newSSARisk.Variable,
		string(newSSARisk.Severity),
		newSSARisk.RiskType,
		newSSARisk.Language,
		newSSARisk.Title,
	)
	newSSARisk.Hash = newSSARisk.CalcHash()
	return newSSARisk
}

func ssaRiskName(variable string, index int) string {
	return fmt.Sprintf("%s-%d", variable, index)
}

func (r *SyntaxFlowResult) SaveRisk(variable string, index int, value *Value, save bool) string {
	_, ok := r.GetAlertInfo(variable)
	if !ok {
		return ""
	}
	ssaRisk := buildSSARisk(r, variable, index, value)
	if ssaRisk == nil {
		return ""
	}

	ssaRisk.RuntimeId = r.TaskID
	ssaRisk.ResultID = uint64(r.GetResultID())
	if save {
		err := yakit.CreateSSARisk(consts.GetGormDefaultSSADataBase(), ssaRisk)
		if err != nil {
			log.Errorf("save risk failed: %s", err)
			return ""
		}
	}
	r.riskMap[ssaRiskName(variable, index)] = ssaRisk
	return ssaRisk.Hash
}

func (r *SyntaxFlowResult) GetGRPCModelRisk() []*ypb.SSARisk {
	if r == nil {
		return nil
	}

	// load risk from database
	if r.dbResult != nil && len(r.riskMap) != len(r.dbResult.RiskHashs) {
		for name := range r.dbResult.RiskHashs {
			r.getRisk(name)
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
	name := ssaRiskName(variable, i)
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

func (r *SyntaxFlowResult) GetRiskHash(variable string, i int) string {
	name := ssaRiskName(variable, i)
	if r == nil {
		return ""
	}
	if r, ok := r.riskMap[name]; ok {
		return r.Hash
	}
	if r.dbResult != nil {
		if hash, ok := r.dbResult.RiskHashs[name]; ok {
			return hash
		}
	}
	return ""
}

func (r *SyntaxFlowResult) getRisk(name string) *schema.SSARisk {
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

func (r *SyntaxFlowResult) YieldRisk() chan *schema.SSARisk {
	ch := make(chan *schema.SSARisk)
	go func() {
		defer close(ch)
		r.GetAlertVariables()
		r.GetAlertValues().ForEach(func(variable string, v Values) bool {
			for index := range v {
				risk := r.GetRiskByValue(variable, index)
				if risk != nil {
					ch <- risk
				}
			}
			return true
		})
	}()
	return ch
}

func (r *SyntaxFlowResult) RiskCount() int {
	return len(r.riskMap)
}
