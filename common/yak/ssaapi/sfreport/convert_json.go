package sfreport

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	logger "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"io"
)

var _ IReport = (*Report)(nil)

func (r *Report) Save(writer io.Writer) error {
	switch r.ReportType {
	case IRifyReportType, IRifyFullReportType:
		return r.PrettyWrite(writer)
	case IRifyReactReportType:
		return r.SaveForIRify()
	}
	return utils.Errorf("unsupported report format: %s", r.ReportType)
}

func (r *Report) SaveForIRify() error {
	ssaReport := r.ToSSAProjectReport()
	// 创建yakit报告实例
	reportInstance := yakit.NewReport()
	reportInstance.From("ssa-scan")
	reportInstance.Title(fmt.Sprintf("%s-%s", ssaReport.ProgramName, uuid.NewString()))

	// 生成报告内容
	err := GenerateYakitReportContent(reportInstance, ssaReport)
	if err != nil {
		logger.Errorf("generate yakit report content failed: %v", err)
		return utils.Wrapf(err, "generate yakit report content failed")
	}

	// 保存报告
	reportID := reportInstance.SaveForIRify()
	if reportID == 0 {
		return utils.Errorf("save report failed")
	}
	return nil
}

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
	if r.ProgramName == "" {
		r.SetProgramName(result.GetProgramName())
		r.SetProgramLang(result.GetProgramLang())
		r.SetFileCount(result.GetFileCount())
		r.SetCodeLineCount(result.GetLineCount())
	}

	for risk := range result.YieldRisk() {
		r.ConvertSSARiskToReport(risk, result)
	}
	return true
}

func (r *Report) AddSyntaxFlowRisks(risks []*schema.SSARisk) {
	for _, risk := range risks {
		r.ConvertSSARiskToReport(risk)
	}
}

func (r *Report) ConvertSSARiskToReport(ssarisk *schema.SSARisk, results ...*ssaapi.SyntaxFlowResult) {
	if r.GetRisk(ssarisk.Hash) != nil {
		// already exists
		return
	}

	// create risk with detailed structure
	risk := NewRisk(ssarisk, r)
	r.AddRisks(risk)

	r.RiskNums = len(r.Risks)
	// get result
	var result *ssaapi.SyntaxFlowResult = nil
	if len(results) > 0 {
		result = results[0]
	} else {
		var err error
		result, err = ssaapi.LoadResultByID(uint(ssarisk.ResultID))
		if err != nil {
			log.Errorf("load result by id %d error: %v", ssarisk.ResultID, err)
			return
		}
	}

	if result == nil {
		log.Errorf("result is nil for risk %s", ssarisk.Hash)
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
	if ret := r.GetFile(editor.GetUrl()); ret != nil {
		return ret
	}
	ret := NewFile(editor, r)
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
