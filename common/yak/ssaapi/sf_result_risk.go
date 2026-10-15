package ssaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
		ScanMode:    string(schema.ValidRuleMode(rule.Mode)),
		// result
		Variable: variable,
		Index:    int64(index),

		Line:     riskCodeRange.StartLine,
		Language: string(result.program.GetLanguage()),
	}
	if functionName := stableRiskFunctionName(value); functionName != "" {
		newSSARisk.FunctionName = functionName
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

	return newSSARisk
}

func stableRiskFunctionName(value *Value) string {
	if value == nil {
		return ""
	}
	fun := value.GetFunction()
	if fun == nil {
		return ""
	}
	rawName := strings.TrimSpace(fun.GetName())
	if funcInst, ok := ssa.ToFunction(fun.GetSSAInst()); ok {
		// Prefer semantic method names when present. They are more stable than
		// compiler-generated internal function names and avoid build-specific suffix drift.
		methodName := strings.TrimSpace(funcInst.GetMethodName())
		if methodName != "" {
			return utils.EscapeInvalidUTF8Byte([]byte(methodName))
		}
	}
	return utils.EscapeInvalidUTF8Byte([]byte(rawName))
}

// ssaRiskName is the slot of one alert inside a single result: the alert
// variable plus its index. GetRiskByValue and the stored RiskHashs map use it
// to find that slot again. It is not the cover key. Covering the same finding
// across scan modes uses RiskFeatureHash.
func ssaRiskName(variable string, index int) string {
	return fmt.Sprintf("%s-%d", variable, index)
}

func (r *SyntaxFlowResult) GetRisks() []*schema.SSARisk {
	if r == nil || r.riskMap == nil {
		return nil
	}
	out := make([]*schema.SSARisk, 0, len(r.riskMap))
	for _, risk := range r.riskMap {
		if risk != nil {
			out = append(out, risk)
		}
	}
	return out
}

// BuildRisk materializes one alert and publishes it once. Persistence belongs
// to the registered handler, or to the direct Save caller's saver.
func (r *SyntaxFlowResult) BuildRisk(variable string, index int, value *Value) string {
	hash, _ := r.stageRisk(variable, index, value)
	return hash
}

// stageRisk is BuildRisk plus whether the handler now owns the audit graph.
// The second result is true only when this call attached that graph to the
// published risk, so the caller must not write it again.
func (r *SyntaxFlowResult) stageRisk(variable string, index int, value *Value) (string, bool) {
	_, ok := r.GetAlertInfo(variable)
	if !ok {
		return "", false
	}
	name := ssaRiskName(variable, index)
	// Building a risk is idempotent per alert slot: the same result may persist
	// its values after the streaming path already materialized them.
	if prev, exist := r.riskMap[name]; exist && prev != nil {
		if r.auditHanded != nil {
			if _, owned := r.auditHanded[name]; owned {
				return prev.Hash, true
			}
		}
		if prev.ResultID != uint64(r.GetResultID()) || prev.RuntimeId != r.TaskID {
			// Saving a prebuilt memory result binds the numeric audit row id.
			// Copy it: a scan handler may still be reading the published risk.
			// The copy keeps the row id, so Save updates that row. Clearing it
			// would insert a second row under the same hash.
			row := *prev
			row.ResultID, row.RuntimeId = uint64(r.GetResultID()), r.TaskID
			row.PrepareForSave(riskFeatureText(value))
			r.riskMap[name] = &row
			return row.Hash, false
		}
		return prev.Hash, false
	}
	ssaRisk := buildSSARisk(r, variable, index, value)
	if ssaRisk == nil {
		return "", false
	}

	ssaRisk.SSAProjectID = r.GetProjectID()
	ssaRisk.RuntimeId = r.TaskID
	ssaRisk.ResultID = uint64(r.GetResultID())
	ssaRisk.ResultUUID = r.GetResultUUID()
	// The row hash covers the result identity, so it must be computed after
	// those fields are set: the same finding reported by two results is two
	// rows, and the unique index on hash must not collapse them. The cover key
	// uses the value text and is computed in the same pass.
	ssaRisk.PrepareForSave(riskFeatureText(value))
	r.riskMap[name] = ssaRisk
	if len(r.onRisk) == 0 {
		return ssaRisk.Hash, false
	}
	var audits []func(*gorm.DB) error
	if audit := r.auditSaver(value, variable, index, ssaRisk); audit != nil {
		audits = []func(*gorm.DB) error{audit}
		if r.auditHanded == nil {
			r.auditHanded = map[string]struct{}{}
		}
		r.auditHanded[name] = struct{}{}
	}
	for _, fn := range r.onRisk {
		fn(ssaRisk, audits...)
	}
	_, owned := r.auditHanded[name]
	return ssaRisk.Hash, owned
}

func riskFeatureText(value *Value) string {
	if value == nil {
		return ""
	}
	return value.String()
}

// auditSaver walks the graph immediately. The database saver later inserts
// the rows; it must not walk again, because the rule may already have cleared
// the value's analysis caches by then.
func (r *SyntaxFlowResult) auditSaver(value *Value, variable string, index int, risk *schema.SSARisk) func(*gorm.DB) error {
	if r == nil || value == nil || risk == nil {
		return nil
	}
	nodes, edges, sources, err := captureAuditGraph(context.Background(), value,
		OptionSaveValue_TaskID(risk.RuntimeId),
		OptionSaveValue_ResultID(uint(risk.ResultID)),
		OptionSaveValue_RuleName(risk.FromRule),
		OptionSaveValue_RuleTitle(risk.Title),
		OptionSaveValue_ProgramName(risk.ProgramName),
		OptionSaveValue_ResultVariable(variable),
		OptionSaveValue_ResultIndex(uint(index)),
		OptionSaveValue_ResultRiskHash(risk.Hash),
		OptionSaveValue_ResultRiskFeatureHash(risk.RiskFeatureHash),
		OptionSaveValue_IsMemoryCompile(r.IsProgMemoryCompile()),
	)
	if err != nil {
		return func(*gorm.DB) error { return err }
	}
	if len(nodes) == 0 && len(edges) == 0 && len(sources) == 0 {
		return nil
	}
	return func(db *gorm.DB) error {
		if db == nil {
			return utils.Error("save audit: database is nil")
		}
		if err := batchSaveAuditNodes(db, nodes); err != nil {
			return err
		}
		if err := batchSaveAuditEdges(db, edges); err != nil {
			return err
		}
		return saveAuditSources(db, sources)
	}
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
	// Binding an audit row changes metadata but not the number of risks.
	// Do not cache by map length: it would return stale ids and timestamps.
	return lo.MapToSlice(r.riskMap, func(name string, risk *schema.SSARisk) *ypb.SSARisk {
		return risk.ToGRPCModel()
	})
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
		yielded := make(map[string]struct{})
		emit := func(risk *schema.SSARisk) {
			if risk == nil || risk.Hash == "" {
				return
			}
			if _, ok := yielded[risk.Hash]; ok {
				return
			}
			yielded[risk.Hash] = struct{}{}
			ch <- risk
		}

		r.GetAlertVariables()
		r.GetAlertValues().ForEach(func(variable string, v Values) bool {
			for index := range v {
				emit(r.GetRiskByValue(variable, index))
			}
			return true
		})
		// Include merged base-only risks (not present in current rule alert values).
		for _, risk := range r.riskMap {
			emit(risk)
		}
	}()
	return ch
}

func (r *SyntaxFlowResult) RiskCount() int {
	if r == nil {
		return 0
	}
	return len(r.riskMap)
}
