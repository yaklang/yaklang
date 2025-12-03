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

// FilterCondition 表示单个过滤条件
type FilterCondition struct {
	Type  string `json:"type"`  // "exact", "regexp", "glob"
	Value string `json:"value"` // 匹配的值或模式
}

// EvidenceFilter 描述过滤器的详细信息
type EvidenceFilter struct {
	FilterType string             `json:"filter_type"` // "string", "opcode", "compare", "version", "regex", "glob", "empty_check"
	MatchMode  string             `json:"match_mode"`  // "have"(全部匹配), "any"(任意匹配) - 仅用于字符串过滤
	Operator   string             `json:"operator"`    // "==", "!=", ">", ">=", "<", "<=" - 仅用于比较操作
	Conditions []*FilterCondition `json:"conditions"`  // 过滤条件列表
}

type EvidenceNode struct {
	Type        EvidenceNodeType
	LogicOp     EvidenceNodeCondition
	Description string          // 人类可读的描述
	Filter      *EvidenceFilter // 结构化的过滤信息
	Children    []*EvidenceNode

	// 运行时添加的证据
	Passed ValueOperator
	Failed ValueOperator
}

type EvidenceAttach struct {
	Label   string
	LabelZh string
	// Values 是变量（如$result)对应的证据
	Values ValueOperator
	// EvidenceTree是?{...}条件过滤对应的证据
	EvidenceTree *EvidenceNode
}

func NewEvidenceLeafNode(typ EvidenceNodeType, sfi *SFI) *EvidenceNode {
	description, filter := generateEvidenceFromSFI(sfi)
	return &EvidenceNode{
		Type:        typ,
		LogicOp:     ConditionTypeNone,
		Description: description,
		Filter:      filter,
		Children:    nil,
	}
}

// generateEvidenceFromSFI 根据 SFI 生成描述和过滤信息
func generateEvidenceFromSFI(sfi *SFI) (string, *EvidenceFilter) {
	if sfi == nil {
		return "", nil
	}

	filter := &EvidenceFilter{}
	var description string

	switch sfi.OpCode {
	case OpCompareString:
		// 字符串条件过滤: ?{have:"a","b"} 或 ?{any:"a","b"}
		mode := ValidStringMatchMode(sfi.UnaryInt)
		filter.FilterType = "string"
		filter.MatchMode = mode.String() // "have" 或 "any"

		// 构建条件列表
		var descParts []string
		for idx, val := range sfi.Values {
			filterMode := ExactConditionFilter
			if idx < len(sfi.MultiOperator) {
				filterMode = ValidConditionFilter(sfi.MultiOperator[idx])
			}

			cond := &FilterCondition{Value: val}
			switch filterMode {
			case RegexpConditionFilter:
				cond.Type = "regexp"
				descParts = append(descParts, fmt.Sprintf("正则匹配 /%s/", val))
			case GlobalConditionFilter:
				cond.Type = "glob"
				descParts = append(descParts, fmt.Sprintf("通配符匹配 %s", val))
			default: // ExactConditionFilter
				cond.Type = "exact"
				descParts = append(descParts, fmt.Sprintf("包含 \"%s\"", val))
			}
			filter.Conditions = append(filter.Conditions, cond)
		}

		// 生成描述
		connector := " 且 "
		if mode != MatchHave {
			connector = " 或 "
		}
		description = fmt.Sprintf("检查值是否满足: %s", strings.Join(descParts, connector))

	case OpCompareOpcode:
		// Opcode 条件过滤: ?{opcode: call}
		filter.FilterType = "opcode"
		for _, val := range sfi.Values {
			filter.Conditions = append(filter.Conditions, &FilterCondition{
				Type:  "opcode",
				Value: val,
			})
		}
		description = fmt.Sprintf("检查指令类型是否为: %s", strings.Join(sfi.Values, " 或 "))

	case OpVersionIn:
		// 版本范围过滤
		filter.FilterType = "version"
		var versionParts []string
		for _, config := range sfi.SyntaxFlowConfig {
			if config != nil {
				filter.Conditions = append(filter.Conditions, &FilterCondition{
					Type:  config.Key,
					Value: config.Value,
				})
				switch config.Key {
				case "greaterThan":
					versionParts = append(versionParts, fmt.Sprintf("> %s", config.Value))
				case "greaterEqual":
					versionParts = append(versionParts, fmt.Sprintf(">= %s", config.Value))
				case "lessThan":
					versionParts = append(versionParts, fmt.Sprintf("< %s", config.Value))
				case "lessEqual":
					versionParts = append(versionParts, fmt.Sprintf("<= %s", config.Value))
				}
			}
		}
		if len(versionParts) > 0 {
			description = fmt.Sprintf("检查版本是否满足: %s", strings.Join(versionParts, " 且 "))
		} else {
			description = "检查版本范围"
		}

	case OpEq:
		filter.FilterType = "compare"
		filter.Operator = "=="
		description = "相等比较 (==)"

	case OpNotEq:
		filter.FilterType = "compare"
		filter.Operator = "!="
		description = "不等比较 (!=)"

	case OpGt:
		filter.FilterType = "compare"
		filter.Operator = ">"
		description = "大于比较 (>)"

	case OpGtEq:
		filter.FilterType = "compare"
		filter.Operator = ">="
		description = "大于等于比较 (>=)"

	case OpLt:
		filter.FilterType = "compare"
		filter.Operator = "<"
		description = "小于比较 (<)"

	case OpLtEq:
		filter.FilterType = "compare"
		filter.Operator = "<="
		description = "小于等于比较 (<=)"

	case OpEmptyCompare:
		filter.FilterType = "empty_check"
		description = "非空检查"

	case OpReMatch:
		filter.FilterType = "regex"
		filter.Conditions = append(filter.Conditions, &FilterCondition{
			Type:  "regexp",
			Value: sfi.UnaryStr,
		})
		description = fmt.Sprintf("检查值是否匹配正则: /%s/", sfi.UnaryStr)

	case OpGlobMatch:
		filter.FilterType = "glob"
		filter.Conditions = append(filter.Conditions, &FilterCondition{
			Type:  "glob",
			Value: sfi.UnaryStr,
		})
		description = fmt.Sprintf("检查值是否匹配通配符: %s", sfi.UnaryStr)
	default:
		return "", nil
	}
	return description, filter
}

func NewEvidenceLogicNode(op EvidenceNodeCondition, children ...*EvidenceNode) *EvidenceNode {
	return &EvidenceNode{
		Type:     EvidenceTypeLogicGate,
		LogicOp:  op,
		Children: children,
	}
}

type EvidenceAttachOption func(*EvidenceAttach)

func NewEvidenceAttach(opts ...EvidenceAttachOption) *EvidenceAttach {
	e := &EvidenceAttach{}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func WithLabel(label string) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.Label = label }
}

func WithLabelZh(label string) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.LabelZh = label }
}

func WithValues(vs ValueOperator) EvidenceAttachOption {
	return func(e *EvidenceAttach) {
		e.Values = vs
	}
}

func WithEvidenceTree(tree *EvidenceNode) EvidenceAttachOption {
	return func(e *EvidenceAttach) { e.EvidenceTree = tree }
}

func (e *EvidenceAttach) String() string {
	if e == nil {
		return "<nil evidence>"
	}
	var sb strings.Builder
	label := e.Label
	if label == "" {
		label = "Evidence"
	}
	sb.WriteString(fmt.Sprintf("== %s ==\n", label))

	// 添加中文标签
	if e.LabelZh != "" {
		sb.WriteString(fmt.Sprintf("Label(中文): %s\n", e.LabelZh))
	}

	// 添加 Values 信息
	if e.Values != nil {
		sb.WriteString(fmt.Sprintf("Values: %v\n", e.Values))
	}

	// 添加 EvidenceTree 信息
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
