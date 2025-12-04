package sfvm

import (
	"fmt"
	"strings"
)

type EvidenceNodeType string

const (
	EvidenceTypeFilterCondition EvidenceNodeType = "FilterCondition"
	EvidenceTypeOpcodeCondition EvidenceNodeType = "OpcodeCondition"
	EvidenceTypeStringCondition EvidenceNodeType = "StringCondition"
	EvidenceTypeLogicGate       EvidenceNodeType = "LogicGate"
)

type EvidenceNodeCondition string

const (
	ConditionTypeAnd  EvidenceNodeCondition = "AND"
	ConditionTypeOr   EvidenceNodeCondition = "OR"
	ConditionTypeNot  EvidenceNodeCondition = "NOT"
	ConditionTypeNone EvidenceNodeCondition = "" // 叶子节点无逻辑操作符
)

type EvidenceAttach struct {
	Description   string
	DescriptionZh string
	// Values 是变量（如$result)对应的证据
	Values ValueOperator
	// EvidenceTree是?{...}条件过滤对应的证据
	EvidenceTree *EvidenceNode
	// SearchMode 搜索模式信息（仅搜索步骤使用）
	SearchMode *SearchMode
}

func (e *EvidenceAttach) GetDescriptionZh() string {
	if e == nil {
		return ""
	}
	if e.DescriptionZh != "" {
		return e.Description
	}
	if e.SearchMode != nil {
		return e.SearchMode.GenerateDescZh()
	}
	if e.EvidenceTree != nil {
		return e.EvidenceTree.GenerateDescZh()
	}
	return ""
}

func (e *EvidenceAttach) GetDescription() string {
	if e == nil {
		return ""
	}
	if e.Description != "" {
		return e.Description
	}
	if e.SearchMode != nil {
		return e.SearchMode.GenerateDesc()
	}
	if e.EvidenceTree != nil {
		return e.EvidenceTree.GenerateDesc()
	}
	return ""
}

func (e *EvidenceAttach) GetConditionFilterSummary() string {
	if e == nil {
		return ""
	}
	if e.EvidenceTree != nil {
		return e.EvidenceTree.GetFilterSummary()
	}
	return ""
}

type EvidenceAttachOption func(*EvidenceAttach)

func NewEvidenceAttach(opts ...EvidenceAttachOption) *EvidenceAttach {
	e := &EvidenceAttach{}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func WithDescription(desc string) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.Description = desc }
}

func WithDescriptionZh(descZh string) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.DescriptionZh = descZh }
}

func WithValues(vs ValueOperator) EvidenceAttachOption {
	return func(e *EvidenceAttach) {
		e.Values = vs
	}
}

func WithEvidenceTree(tree *EvidenceNode) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.EvidenceTree = tree }
}

func WithSearchMode(searchType SearchType, pattern string, matchMode int, isRecursive bool) EvidenceAttachOption {
	return func(e *EvidenceAttach) {
		e.SearchMode = &SearchMode{
			SearchType:   searchType,
			Pattern:      pattern,
			MatchMode:    matchMode,
			MatchModeStr: MatchModeString(matchMode),
			IsRecursive:  isRecursive,
		}
	}
}

func (e *EvidenceAttach) String() string {
	if e == nil {
		return "<nil evidence>"
	}
	var sb strings.Builder

	desc := e.GetDescription()
	if desc == "" {
		desc = "Evidence"
	}
	sb.WriteString(fmt.Sprintf("== %s ==\n", desc))
	descriptionZh := e.GetDescriptionZh()
	if descriptionZh != "" {
		sb.WriteString(fmt.Sprintf("Desc(中文): %s\n", descriptionZh))
	}
	if e.SearchMode != nil {
		sb.WriteString(fmt.Sprintf("SearchMode: %s [%s] (match: %s)\n",
			e.SearchMode.SearchType, e.SearchMode.Pattern, e.SearchMode.MatchModeStr))
	}
	if summary := e.GetConditionFilterSummary(); summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", summary))
	}

	if e.Values != nil {
		sb.WriteString(fmt.Sprintf("Values: %v\n", e.Values))
	}
	if e.EvidenceTree != nil {
		sb.WriteString("EvidenceTree:\n")
		sb.WriteString(e.formatEvidenceNode(e.EvidenceTree, 1))
	}
	return sb.String()
}

func (e *EvidenceAttach) formatEvidenceNode(node *EvidenceNode, indent int) string {
	if node == nil {
		return ""
	}

	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	// 输出节点类型和逻辑操作符
	if node.LogicOp != ConditionTypeNone {
		sb.WriteString(fmt.Sprintf("%s[%s] %s\n", prefix, node.Type, node.LogicOp))
	} else {
		sb.WriteString(fmt.Sprintf("%s[%s]\n", prefix, node.Type))
	}

	// 输出描述
	if node.Description != "" {
		sb.WriteString(fmt.Sprintf("%s  Description: %s\n", prefix, node.Description))
	}

	// 输出过滤信息
	if node.Filter != nil {
		sb.WriteString(fmt.Sprintf("%s  Filter: %s", prefix, node.Filter.FilterType))
		if node.Filter.MatchMode != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", node.Filter.MatchMode))
		}
		if node.Filter.Operator != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", node.Filter.Operator))
		}
		sb.WriteString("\n")
		for _, cond := range node.Filter.Conditions {
			sb.WriteString(fmt.Sprintf("%s    - %s: %s\n", prefix, cond.Type, cond.Value))
		}
	}

	// 输出通过和失败的证据
	if node.Passed != nil {
		sb.WriteString(fmt.Sprintf("%s  Passed: %v\n", prefix, node.Passed))
	}
	if node.Failed != nil {
		sb.WriteString(fmt.Sprintf("%s  Failed: %v\n", prefix, node.Failed))
	}

	// 递归输出子节点
	for _, child := range node.Children {
		sb.WriteString(e.formatEvidenceNode(child, indent+1))
	}

	return sb.String()
}
