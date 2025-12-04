package sfvm

import (
	"fmt"
	"strings"
)

// EvidenceNode 条件过滤语句的证据
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

// SearchMode 搜索语句的证据
type SearchMode struct {
	SearchType   SearchType `json:"search_type"`    // 搜索类型：exact, fuzzy, regexp
	Pattern      string     `json:"pattern"`        // 搜索模式
	MatchMode    int        `json:"match_mode"`     // NameMatch=1, KeyMatch=2, BothMatch=3
	MatchModeStr string     `json:"match_mode_str"` // "name", "key", "name+key"
	IsRecursive  bool       `json:"is_recursive"`   // 是否递归搜索
}
