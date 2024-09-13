package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

type SyntaxFlowResult struct {
	// result
	memResult *sfvm.SFFrameResult

	// cache
	symbol map[string]Values

	// variable
	alertVariable []string
	variable      *orderedmap.OrderedMap // string - int

	// message info
	alertMsg map[string]string
	checkMsg []string

	unName Values
}

func createEmptyResult() *SyntaxFlowResult {
	return &SyntaxFlowResult{
		symbol:   make(map[string]Values),
		alertMsg: make(map[string]string),
	}
}

func CreateResultFromQuery(res *sfvm.SFFrameResult) *SyntaxFlowResult {
	ret := createEmptyResult()
	ret.memResult = res
	return ret
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
	if r.memResult != nil {
		return r.memResult.Name()
	}

	return ""
}

func (r *SyntaxFlowResult) GetAlertMsg(name string) (string, bool) {
	if r == nil {
		return "", false
	}

	if r.memResult != nil {
		res, ok := r.memResult.AlertMsgTable[name]
		return res, ok
	}

	return "", false
}

func (r *SyntaxFlowResult) GetErrors() []string {
	if r == nil {
		return nil
	}
	if r.memResult != nil {
		return r.memResult.Errors
	}
	return nil
}

func (r *SyntaxFlowResult) GetCheckMsg() []string {
	if r == nil {
		return nil
	}

	if r.checkMsg != nil {
		return r.checkMsg
	}

	if r.memResult != nil {
		msgs := make([]string, 0)
		for _, name := range r.memResult.CheckParams {
			if msg, ok := r.memResult.Description.Get("$" + name); ok {
				msgs = append(msgs, msg)
			}
		}
		r.checkMsg = msgs
		return msgs
	}

	return nil
}
