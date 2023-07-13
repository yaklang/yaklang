// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser // SuricataRuleParser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by SuricataRuleParser.
type SuricataRuleParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SuricataRuleParser#rules.
	VisitRules(ctx *RulesContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#rule.
	VisitRule(ctx *RuleContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#action.
	VisitAction(ctx *ActionContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#protocol.
	VisitProtocol(ctx *ProtocolContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#src_address.
	VisitSrc_address(ctx *Src_addressContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#dest_address.
	VisitDest_address(ctx *Dest_addressContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#address.
	VisitAddress(ctx *AddressContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#ipv4.
	VisitIpv4(ctx *Ipv4Context) interface{}

	// Visit a parse tree produced by SuricataRuleParser#ipv4block.
	VisitIpv4block(ctx *Ipv4blockContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#ipv4mask.
	VisitIpv4mask(ctx *Ipv4maskContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#environment_var.
	VisitEnvironment_var(ctx *Environment_varContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#ipv6.
	VisitIpv6(ctx *Ipv6Context) interface{}

	// Visit a parse tree produced by SuricataRuleParser#hex_part.
	VisitHex_part(ctx *Hex_partContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#h16.
	VisitH16(ctx *H16Context) interface{}

	// Visit a parse tree produced by SuricataRuleParser#src_port.
	VisitSrc_port(ctx *Src_portContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#dest_port.
	VisitDest_port(ctx *Dest_portContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#port.
	VisitPort(ctx *PortContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#params.
	VisitParams(ctx *ParamsContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#param.
	VisitParam(ctx *ParamContext) interface{}

	// Visit a parse tree produced by SuricataRuleParser#string.
	VisitString(ctx *StringContext) interface{}
}
