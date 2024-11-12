package ssaapi

import (
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

	riskMap map[string]*schema.Risk
	// cache
	riskGRPCCache []*ypb.Risk
}

func createEmptyResult() *SyntaxFlowResult {
	return &SyntaxFlowResult{
		symbol:  make(map[string]Values),
		riskMap: make(map[string]*schema.Risk),
	}
}

func CreateResultFromQuery(res *sfvm.SFFrameResult) *SyntaxFlowResult {
	ret := createEmptyResult()
	ret.memResult = res
	ret.rule = res.GetRule()
	return ret
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
	if info, ok := r.rule.GetAlertInfo(name); ok {
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
