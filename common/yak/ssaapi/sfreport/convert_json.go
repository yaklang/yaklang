package sfreport

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io"
)

var _ IReport = (*Report)(nil)

func (r *Report) PrettyWrite(w io.Writer) error {
	jsonData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(jsonData)
	if err != nil {
		return err
	}
	return nil
}

func (r *Report) AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool {
	for risk := range result.YieldRisk() {
		r.ConvertSSARiskToReport(risk)
	}
	return true
}

func (r *Report) AddSyntaxFlowRisks(risks []*schema.SSARisk) {
	for _, risk := range risks {
		r.ConvertSSARiskToReport(risk)
	}
}

func (r *Report) ConvertSSARiskToReport(ssarisk *schema.SSARisk) {
	if r.GetRisk(ssarisk.Hash) != nil {
		// already exists
		return
	}

	// create risk with detailed structure
	risk := NewRisk(ssarisk)
	r.AddRisks(risk)

	r.RiskNums = len(r.Risks)
	// get result
	result, err := ssaapi.LoadResultByID(uint(ssarisk.ResultID))
	if err != nil {
		log.Errorf("load result by id %d error: %v", ssarisk.ResultID, err)
		return
	}

	// get value
	value, err := result.GetValue(ssarisk.Variable, int(ssarisk.Index))
	if err != nil {
		log.Errorf("get value by variable %s and index %d error: %v", ssarisk.Variable, ssarisk.Index, err)
		return
	}

	// {{ analyze graph
	// Data flow information is now automatically generated in NewRisk() function
	// The DataFlowPaths field will contain the complete data flow analysis
	// }}

	// {{ file
	// editor
	editor := value.GetRange().GetEditor()
	if editor == nil {
		log.Errorf("editor is nil")
		return
	}

	// create report.file
	file := r.FirstOrCreateFile(editor)
	risk.SetFile(file)
	file.AddRisk(risk)
	// }}

	// {{ rule
	// create report.rule
	rule := r.FirstOrCreateRule(result.GetRule())
	risk.SetRule(rule)
	rule.AddRisk(risk)
	// }}
}

func (r *Report) FirstOrCreateRule(rule *schema.SyntaxFlowRule) *Rule {
	if ret := r.GetRule(rule.RuleName); ret != nil {
		return ret
	}
	ret := NewRule(rule)
	r.Rules = append(r.Rules, ret)
	return ret
}

func (r *Report) FirstOrCreateFile(editor *memedit.MemEditor) *File {
	if ret := r.GetFile(editor.GetFilename()); ret != nil {
		return ret
	}
	ret := NewFile(r.ReportType, editor)
	r.File = append(r.File, ret)
	return ret
}

func ConvertRisksToJson(risks []*schema.SSARisk) ([]byte, error) {
	reporter := NewReport(IRifyReportType)
	reporter.AddSyntaxFlowRisks(risks)
	var writer bytes.Buffer
	err := reporter.PrettyWrite(&writer)
	if err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}
