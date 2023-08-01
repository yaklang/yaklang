package suricata

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func (r *RuleSyntaxVisitor) Errorf(msg string, items ...interface{}) {
	if len(items) == 0 {
		r.Errors = append(r.Errors, utils.Error(msg))
		return
	}
	r.Errors = append(r.Errors, utils.Errorf(msg, items...))
}

func (r *RuleSyntaxVisitor) ShowErrors() {
	if len(r.Errors) > 0 {
		for _, e := range r.Errors {
			log.Error(e.Error())
		}
	}
}

func (r *RuleSyntaxVisitor) VisitRules(ctx *parser.RulesContext) interface{} {
	if ctx == nil {
		r.Errorf("visit rule met emtpy rules...")
		return nil
	}

	for _, rule := range ctx.AllRule_() {
		if rule == nil {
			continue
		}
		r.VisitRule(rule.(*parser.RuleContext))
	}
	return nil
}

func (r *RuleSyntaxVisitor) MergeErrors() error {
	if r.Errors == nil {
		return nil
	}
	errors := funk.Map(r.Errors, func(er error) string {
		return er.Error()
	}).([]string)
	return utils.Error(strings.Join(errors, "\n"))
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func (r *RuleSyntaxVisitor) VisitRule(rule *parser.RuleContext) interface{} {
	if rule == nil {
		return nil
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("visit rule failed: %s", err)
		}
	}()

	ruleIns := &Rule{
		Raw: strings.TrimSpace(rule.GetText()),
	}
	end := rule.GetStop().GetStop()
	if len(r.Raw) > end {
		end += 1
	}
	data := r.Raw[rule.GetStart().GetStart():end]
	ruleIns.Raw = utils.EscapeInvalidUTF8Byte(data)
	ruleIns.ContentRuleConfig = &ContentRuleConfig{}

	/*
		fill rules protocol src dst info
	*/
	action := rule.Action_()
	ruleIns.Action = trim(action.GetText())
	ruleIns.Protocol = trim(rule.Protocol().GetText())
	ruleIns.SourceAddress = r.VisitSrcAddress(rule.Src_address().(*parser.Src_addressContext))
	ruleIns.DestinationAddress = r.VisitDstAddress(rule.Dest_address().(*parser.Dest_addressContext))
	ruleIns.SourcePort = r.VisitSrcPort(rule.Src_port().(*parser.Src_portContext))
	ruleIns.DestinationPort = r.VisitDstPort(rule.Dest_port().(*parser.Dest_portContext))

	valid := ruleIns.Action != "" && ruleIns.Protocol != "" &&
		ruleIns.SourcePort != nil && ruleIns.DestinationPort != nil &&
		ruleIns.SourceAddress != nil && ruleIns.DestinationAddress != nil

	if !valid {
		spew.Dump(ruleIns)
		r.Errorf("met error, parse failed")
		return nil
	}

	/*
		parse contents
	*/
	params := rule.Params()
	if params == nil {
		r.Errorf("params cannot be empty")
		return nil
	}

	r.VisitParams(params.(*parser.ParamsContext), ruleIns)
	r.Rules = append(r.Rules, ruleIns)
	return nil
}
