package sfvm

import (
	"fmt"
	"strings"
)

type EvidenceNode struct {
	Type        EvidenceNodeType
	LogicOp     EvidenceNodeCondition
	Description string
	Children    []*EvidenceNode
	// string compare, opcode compare的证据
	CompareEvidence *CompareEvidence
	// check empty 的证据
	Results []*FilterResult
}

type CompareEvidence struct {
	FilterType string             `json:"filter_type"` // "string", "opcode", "compare", "version", "regex", "glob", "empty_check"
	MatchMode  string             `json:"match_mode"`  // "have"(全部匹配), "any"(任意匹配) - 仅用于字符串过滤
	Operator   string             `json:"operator"`    // "==", "!=", ">", ">=", "<", "<=" - 仅用于比较操作
	Conditions []*FilterCondition `json:"conditions"`  // 过滤条件列表
	Results    []*FilterResult
}

type CheckEmptyEvidence struct {
	Results []*FilterResult
}

// FilterCondition 单个过滤条件
type FilterCondition struct {
	Type  string `json:"type"`  // "exact", "regexp", "glob"
	Value string `json:"value"` // 匹配的值或模式
}

type FilterResult struct {
	Value       ValueOperator // 原始值 (如 sink1)
	IntermValue ValueOperator // 中间值：过滤表达式产生的值
	Passed      bool          // 是否通过过滤
}

type FilterEvidenceItem = FilterResult

func generateEvidenceFromSFI(sfi *SFI) (string, *CompareEvidence) {
	if sfi == nil {
		return "", nil
	}

	filter := &CompareEvidence{}
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
		Type:            typ,
		LogicOp:         ConditionTypeNone,
		Description:     description,
		CompareEvidence: filter,
		Children:        nil,
	}
}

type SearchMode struct {
	SearchType   SearchType `json:"search_type"`    // 搜索类型：exact, fuzzy, regexp
	Pattern      string     `json:"pattern"`        // 搜索模式
	MatchMode    int        `json:"match_mode"`     // NameMatch=1, KeyMatch=2, BothMatch=3
	MatchModeStr string     `json:"match_mode_str"` // "name", "key", "name+key"
	IsRecursive  bool       `json:"is_recursive"`   // 是否递归搜索
}

type DataFlowDirection string

const (
	DataFlowDirectionTopDef    DataFlowDirection = "top_def"
	DataFlowDirectionBottomUse DataFlowDirection = "bottom_use"
)

// DataFlowMode 数据流分析语句的证据
type DataFlowMode struct {
	Direction DataFlowDirection   `json:"direction"`        // 数据流方向
	Depth     int                 `json:"depth"`            // 深度限制
	DepthMin  int                 `json:"depth_min"`        // 最小深度
	DepthMax  int                 `json:"depth_max"`        // 最大深度
	Config    map[string][]string `json:"config,omitempty"` // 配置规则 (include/exclude/until/hook)
}

func NewDataFlowMode(direction DataFlowDirection, configs []*RecursiveConfigItem) *DataFlowMode {
	mode := &DataFlowMode{
		Direction: direction,
		Config:    make(map[string][]string),
	}
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		switch RecursiveConfigKey(cfg.Key) {
		case RecursiveConfig_Depth:
			mode.Depth = parseIntOrZero(cfg.Value)
		case RecursiveConfig_DepthMin:
			mode.DepthMin = parseIntOrZero(cfg.Value)
		case RecursiveConfig_DepthMax:
			mode.DepthMax = parseIntOrZero(cfg.Value)
		case RecursiveConfig_Include, RecursiveConfig_Exclude, RecursiveConfig_Until, RecursiveConfig_Hook:
			mode.Config[cfg.Key] = append(mode.Config[cfg.Key], cfg.Value)
		}
	}
	return mode
}

func (m *DataFlowMode) String() string {
	if m == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(string(m.Direction))
	if m.Depth > 0 || m.DepthMin > 0 || m.DepthMax > 0 {
		sb.WriteString(fmt.Sprintf("\n  Depth: %d, Min: %d, Max: %d", m.Depth, m.DepthMin, m.DepthMax))
	}
	for key, values := range m.Config {
		if len(values) > 0 {
			sb.WriteString(fmt.Sprintf("\n  %s: %v", key, values))
		}
	}
	return sb.String()
}

func parseIntOrZero(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}
