package rule

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func (v *RuleSyntaxVisitor) Errorf(msg string, items ...interface{}) {
	if len(items) == 0 {
		v.Errors = append(v.Errors, utils.Error(msg))
		return
	}
	v.Errors = append(v.Errors, utils.Errorf(msg, items...))
}

func (v *RuleSyntaxVisitor) ShowErrors() {
	if len(v.Errors) > 0 {
		for _, e := range v.Errors {
			log.Error(e.Error())
		}
	}
}

func (v *RuleSyntaxVisitor) VisitRules(ctx *parser.RulesContext) interface{} {
	if ctx == nil {
		v.Errorf("visit rule met emtpy rules...")
		return nil
	}

	for _, rule := range ctx.AllRule_() {
		if rule == nil {
			continue
		}
		v.VisitRule(rule.(*parser.RuleContext))
	}
	return nil
}

func (v *RuleSyntaxVisitor) MergeErrors() error {
	if v.Errors == nil {
		return nil
	}
	errors := funk.Map(v.Errors, func(er error) string {
		return er.Error()
	}).([]string)
	return utils.Error(strings.Join(errors, "\n"))
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func (v *RuleSyntaxVisitor) VisitRule(rule *parser.RuleContext) interface{} {
	if rule == nil {
		return nil
	}
	defer func() {
		if err := recover(); err != nil {
			start := rule.GetStart()
			end := rule.GetStop()
			log.Errorf("visit rule %v (%v:%v-%v:%v) failed: %s",
				rule.GetText(),
				start.GetLine(), start.GetColumn(),
				end.GetLine(), end.GetColumn(),
				err,
			)
			//panic(err)
		}
	}()
	ruleIns := &Rule{
		Raw: strings.TrimSpace(rule.GetText()),
	}
	end := rule.GetStop().GetStop()
	if len(v.Raw) > end {
		end += 1
	}
	data := v.CompileRaw[rule.GetStart().GetStart():end]
	ruleIns.Raw = utils.EscapeInvalidUTF8Byte([]byte(data))
	ruleIns.ContentRuleConfig = &ContentRuleConfig{}

	/*
		fill rules protocol src dst info
	*/
	action := rule.Action_()
	ruleIns.Action = trim(action.GetText())
	ruleIns.Protocol = trim(rule.Protocol().GetText())
	ruleIns.SourceAddress = v.VisitSrcAddress(rule.Src_address().(*parser.Src_addressContext))
	ruleIns.DestinationAddress = v.VisitDstAddress(rule.Dest_address().(*parser.Dest_addressContext))
	ruleIns.SourcePort = v.VisitSrcPort(rule.Src_port().(*parser.Src_portContext))
	ruleIns.DestinationPort = v.VisitDstPort(rule.Dest_port().(*parser.Dest_portContext))

	valid := ruleIns.Action != "" && ruleIns.Protocol != "" &&
		ruleIns.SourcePort != nil && ruleIns.DestinationPort != nil &&
		ruleIns.SourceAddress != nil && ruleIns.DestinationAddress != nil

	if !valid {
		spew.Dump(ruleIns)
		v.Errorf("met error, parse failed")
		return nil
	}

	/*
		parse contents
	*/
	params := rule.Params()
	if params == nil {
		v.Errorf("params cannot be empty")
		return nil
	}

	err := v.VisitParams(params.(*parser.ParamsContext), ruleIns)
	if err != nil {
		v.Errorf("visit params failed: %v", err)
		return nil
	}
	v.Rules = append(v.Rules, ruleIns)
	return nil
}
