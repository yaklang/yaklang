package ssaapi

import (
	"sort"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowResult struct {
	// result
	memResult *sfvm.SFFrameResult
	dbResult  *ssadb.AuditResult
	// for create value
	program *Program
	//from db
	rule *schema.SyntaxFlowRule
	// cache
	symbol map[string]Values

	// variable
	alertVariable []string
	variable      *orderedmap.OrderedMap // string - int

	// message info
	checkMsg []string

	unName Values

	riskMap map[string]*schema.SSARisk
	// cache
	riskGRPCCache []*ypb.SSARisk
}

func createEmptyResult() *SyntaxFlowResult {
	return &SyntaxFlowResult{
		symbol:  make(map[string]Values),
		riskMap: make(map[string]*schema.SSARisk),
	}
}

func CreateResultFromQuery(res *sfvm.SFFrameResult) *SyntaxFlowResult {
	ret := createEmptyResult()
	ret.setMemoryResult(res)
	ret.rule = res.GetRule()
	return ret
}
func CreateResultWithProg(prog *Program, res *sfvm.SFFrameResult) *SyntaxFlowResult {
	ret := createEmptyResult()
	ret.program = prog
	ret.setMemoryResult(res)
	ret.rule = res.GetRule()
	return ret
}

func (r *SyntaxFlowResult) setMemoryResult(res *sfvm.SFFrameResult) {
	r.memResult = res
	res.SymbolTable = res.SymbolTable.Map(func(s string, vo sfvm.ValueOperator) (string, sfvm.ValueOperator, error) {
		values := SyntaxFlowVariableToValues(vo)
		sort.Slice(values, func(i, j int) bool {
			// sort by file
			valueI := values[i]
			valueJ := values[j]
			rangeI := valueI.GetRange()
			rangeJ := valueJ.GetRange()
			if rangeI == nil || rangeI.GetEditor() == nil {
				return false // i < j
			}
			if rangeJ == nil || rangeJ.GetEditor() == nil {
				return true // i > j
			}
			fileI := rangeI.GetEditor().GetFilename()
			fileJ := rangeJ.GetEditor().GetFilename()
			if fileI != fileJ {
				return fileI > fileJ // i > j
			}
			offsetI := rangeI.GetStartOffset()
			offsetJ := rangeJ.GetStartOffset()
			if offsetI != offsetJ {
				return offsetI > offsetJ // i > j
			}
			return i > j // all same just by index
		})
		return s, values, nil
	})
}

func (r *SyntaxFlowResult) GetSFResult() *sfvm.SFFrameResult {
	if r == nil {
		return nil
	}
	return r.memResult
}

func (r *SyntaxFlowResult) String(opts ...sfvm.ShowOption) string {
	if r == nil || r.memResult == nil {
		return ""
	}
	return r.memResult.String(opts...)
}

func (r *SyntaxFlowResult) Show(opts ...sfvm.ShowOption) {
	if r == nil || r.memResult == nil {
		return
	}
	r.memResult.Show(opts...)
}

func (r *SyntaxFlowResult) Name() string {
	if r == nil {
		return ""
	}

	checkAndHandler := func(str ...string) string {
		for _, s2 := range str {
			if s2 != "" {
				return s2
			}
		}
		return ""
	}
	return checkAndHandler(r.rule.Title, r.rule.TitleZh, r.rule.Description, utils.ShrinkString(r.String(), 40))
}

func (r *SyntaxFlowResult) GetAlertMsg(name string) (string, bool) {
	if info, ok := r.GetAlertInfo(name); ok {
		return info.Msg, true
	}
	return "", false
}
func (r *SyntaxFlowResult) GetAlertInfo(name string) (*schema.SyntaxFlowDescInfo, bool) {
	return r.rule.GetAlertInfo(name)
}

func (r *SyntaxFlowResult) GetErrors() []string {
	if r == nil {
		return nil
	}
	if r.memResult != nil {
		return r.memResult.Errors
	} else if r.dbResult != nil {
		return r.dbResult.Errors
	}
	return nil
}

func (r *SyntaxFlowResult) GetCheckMsg() []string {
	if r == nil {
		return nil
	}

	if r.memResult != nil {
		msgs := make([]string, 0)
		for _, name := range r.memResult.CheckParams {
			if msg, ok := r.memResult.Description.Get("$" + name); ok {
				msgs = append(msgs, msg)
			}
		}
		return msgs
	} else if r.dbResult != nil {
		return r.dbResult.CheckMsg
	}

	return nil
}

func (r *SyntaxFlowResult) GetProgramName() string {
	if r == nil {
		return ""
	}
	if r.program != nil {
		return r.program.GetProgramName()
	}
	return ""
}

func (r *SyntaxFlowResult) GetRule() *schema.SyntaxFlowRule {
	return r.rule
}
