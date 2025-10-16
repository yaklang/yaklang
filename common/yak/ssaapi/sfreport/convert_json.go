package sfreport

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/google/uuid"
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
		log.Errorf("generate yakit report content failed: %v", err)
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

func (r *Report) AddSyntaxFlowRisks(risks ...*schema.SSARisk) {
	for _, risk := range risks {
		r.ConvertSSARiskToReport(risk)
	}
}

func (r *Report) ConvertSSARiskToReport(ssarisk *schema.SSARisk, results ...*ssaapi.SyntaxFlowResult) {
	if r.GetRisk(ssarisk.Hash) != nil {
		// already exists
		return
	}

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

	// create risk with detailed structure
	risk, toAddIrSourceHashes := NewRisk(ssarisk, r, value)
	r.AddRisks(risk)

	// create report.file
	file, ok := r.FirstOrCreateFile(editor)
	if ok {
		file.AddRisk(risk)
	}
	// }}

	// add ir source from data flow paths
	for _, irSourceHash := range toAddIrSourceHashes {
		file, ok := r.FirstOrCreateFileByHash(irSourceHash)
		if ok {
			file.AddRisk(risk)
		}
	}
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

func (r *Report) FirstOrCreateFile(editor *memedit.MemEditor) (*File, bool) {
	irsourceHash := editor.GetIrSourceHash()
	if _, ok := r.IrSourceHashes[irsourceHash]; ok {
		return nil, false
	}
	r.IrSourceHashes[irsourceHash] = struct{}{}
	ret := NewFile(editor, r)
	r.File = append(r.File, ret)
	return ret, true
}

func (r *Report) FirstOrCreateFileByHash(irSourceHash string) (*File, bool) {
	if _, ok := r.IrSourceHashes[irSourceHash]; ok {
		return nil, false
	}
	r.IrSourceHashes[irSourceHash] = struct{}{}
	editor, err := ssadb.GetEditorByHash(irSourceHash)
	if err != nil {
		log.Errorf("get editor by hash %s error: %v", irSourceHash, err)
		return nil, false
	}
	ret := NewFile(editor, r)
	r.File = append(r.File, ret)
	return ret, true
}

type ImportSSARiskOption func(*ImportSSARiskManager)

type ImportSSARiskManager struct {
	programName string
	db          *gorm.DB
	ctx         context.Context
	callback    func(string, float64)
}

func WithDB(db *gorm.DB) ImportSSARiskOption {
	return func(m *ImportSSARiskManager) {
		m.db = db
	}
}

func WithContext(ctx context.Context) ImportSSARiskOption {
	return func(m *ImportSSARiskManager) {
		m.ctx = ctx
	}
}

func WithProgramName(programName string) ImportSSARiskOption {
	return func(m *ImportSSARiskManager) {
		m.programName = programName
	}
}

func WithCallback(callback func(string, float64)) ImportSSARiskOption {
	return func(m *ImportSSARiskManager) {
		m.callback = callback
	}
}

func (m *ImportSSARiskManager) subProgressCallback(start, end float64) func(string, float64) {
	return func(msg string, subProgress float64) {
		if m.callback == nil {
			return
		}
		if subProgress < 0 {
			subProgress = 0
		} else if subProgress > 1 {
			subProgress = 1
		}
		global := start + (end-start)*subProgress
		m.callback(msg, global)
	}
}

func NewImportSSARiskManager(opts ...ImportSSARiskOption) *ImportSSARiskManager {
	m := &ImportSSARiskManager{}
	for _, opt := range opts {
		opt(m)
	}
	if m.db == nil {
		m.db = ssadb.GetDB()
	}
	return m
}

func ImportSSARiskFromJSON(
	ctx context.Context,
	db *gorm.DB,
	jsonData []byte,
	callBacks ...func(string, float64),
) error {
	var report *Report
	if err := json.Unmarshal(jsonData, &report); err != nil {
		return utils.Wrapf(err, "failed to parse JSON data")
	}

	opts := []ImportSSARiskOption{
		WithDB(db),
		WithContext(ctx),
		WithProgramName(report.ProgramName),
	}
	if len(callBacks) > 0 {
		opts = append(opts, WithCallback(callBacks[0]))
	}

	manager := NewImportSSARiskManager(opts...)

	if err := manager.importRisksFromReport(report, manager.subProgressCallback(0, 0.5)); err != nil {
		return err
	}
	if err := manager.importFilesFromReport(report, manager.subProgressCallback(0.5, 1)); err != nil {
		return err
	}
	return nil
}

func (m *ImportSSARiskManager) importRisksFromReport(report *Report, cb func(string, float64)) error {
	if len(report.Risks) == 0 {
		if cb != nil {
			cb("No risks to import", 1)
		}
		return nil
	}
	total := len(report.Risks)
	count := 0
	for _, risk := range report.Risks {
		select {
		case <-m.ctx.Done():
			return utils.Error("Import SSARisk from JSON failed: context done")
		default:
		}
		count++
		if err := risk.SaveToDB(m.db); err != nil {
			log.Errorf("Import SSARisk from JSON failed: %v", err)
		}
		if cb != nil {
			progress := float64(count) / float64(total)
			cb(fmt.Sprintf("Importing risk %d/%d", count, total), progress)
		}
	}
	return nil
}

func (m *ImportSSARiskManager) importFilesFromReport(report *Report, cb func(string, float64)) error {
	if len(report.File) == 0 {
		if cb != nil {
			cb("No files to import", 1)
		}
		return nil
	}
	total := len(report.File)
	for i, file := range report.File {
		select {
		case <-m.ctx.Done():
			return utils.Error("Import Files from JSON failed: context done")
		default:
		}
		if err := file.SaveToDB(m.db, m.programName); err != nil {
			log.Errorf("Import Files from JSON failed: %v", err)
		}
		if cb != nil {
			progress := float64(i+1) / float64(total)
			cb(fmt.Sprintf("Importing file %d/%d", i+1, total), progress)
		}
	}
	return nil
}
