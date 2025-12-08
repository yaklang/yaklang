package sfvm

import (
	"fmt"
	"strings"
)

func (n *EvidenceNode) GenerateDesc() string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case EvidenceTypeStringCondition:
		if n.CompareEvidence != nil && len(n.CompareEvidence.Conditions) > 0 {
			// 详细格式: String Filter (have all): "hello", "world" 或 String Filter (match any): "a", "b"
			mode := n.CompareEvidence.MatchMode
			modeDesc := "have all"
			if mode == "any" {
				modeDesc = "match any"
			}
			var condDescs []string
			for _, cond := range n.CompareEvidence.Conditions {
				switch cond.Type {
				case "regexp":
					condDescs = append(condDescs, fmt.Sprintf("regexp /%s/", cond.Value))
				case "glob":
					condDescs = append(condDescs, fmt.Sprintf("glob [%s]", cond.Value))
				default:
					condDescs = append(condDescs, fmt.Sprintf("%q", cond.Value))
				}
			}
			return fmt.Sprintf("String Filter (%s): %s", modeDesc, strings.Join(condDescs, ", "))
		}
		return "String Filter"
	case EvidenceTypeOpcodeCondition:
		if n.CompareEvidence != nil && len(n.CompareEvidence.Conditions) > 0 {
			// 详细格式: Opcode Filter: const, call
			values := make([]string, 0)
			for _, cond := range n.CompareEvidence.Conditions {
				values = append(values, cond.Value)
			}
			return fmt.Sprintf("Opcode Filter: %s", strings.Join(values, ", "))
		}
		return "Opcode Filter"
	case EvidenceTypeLogicGate:
		switch n.LogicOp {
		case ConditionTypeAnd:
			return "Logic AND"
		case ConditionTypeOr:
			return "Logic OR"
		case ConditionTypeNot:
			return "Logic NOT"
		}
		return "Logic Gate"
	case EvidenceTypeFilterCondition:
		return "Condition Filter"
	default:
		return "Filter"
	}
}

func (n *EvidenceNode) GenerateDescZh() string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case EvidenceTypeStringCondition:
		if n.CompareEvidence != nil && len(n.CompareEvidence.Conditions) > 0 {
			// 详细格式: 字符串过滤（全部包含）: "hello", "world" 或 字符串过滤（任一匹配）: "a", "b"
			modeDesc := "全部包含"
			if n.CompareEvidence.MatchMode == "any" {
				modeDesc = "任一匹配"
			}
			var condDescs []string
			for _, cond := range n.CompareEvidence.Conditions {
				switch cond.Type {
				case "regexp":
					condDescs = append(condDescs, fmt.Sprintf("正则 /%s/", cond.Value))
				case "glob":
					condDescs = append(condDescs, fmt.Sprintf("通配符 [%s]", cond.Value))
				default:
					condDescs = append(condDescs, fmt.Sprintf("%q", cond.Value))
				}
			}
			return fmt.Sprintf("字符串过滤（%s）: %s", modeDesc, strings.Join(condDescs, ", "))
		}
		return "字符串过滤"
	case EvidenceTypeOpcodeCondition:
		if n.CompareEvidence != nil && len(n.CompareEvidence.Conditions) > 0 {
			// 详细格式: 指令类型过滤: const, call
			values := make([]string, 0)
			for _, cond := range n.CompareEvidence.Conditions {
				values = append(values, cond.Value)
			}
			return fmt.Sprintf("指令类型过滤: %s", strings.Join(values, ", "))
		}
		return "指令类型过滤"
	case EvidenceTypeLogicGate:
		switch n.LogicOp {
		case ConditionTypeAnd:
			return "逻辑与（AND）"
		case ConditionTypeOr:
			return "逻辑或（OR）"
		case ConditionTypeNot:
			return "逻辑非（NOT）"
		}
		return "逻辑运算"
	case EvidenceTypeFilterCondition:
		return "条件过滤"
	default:
		return "过滤"
	}
}

// GetFilterSummary 获取过滤摘要（通过/失败数量）
func (n *EvidenceNode) GetFilterSummary() string {
	if n == nil {
		return ""
	}
	passedCount := 0
	failedCount := 0
	for _, r := range n.Results {
		if r.Passed {
			passedCount++
		} else {
			failedCount++
		}
	}
	total := passedCount + failedCount
	if total == 0 {
		return ""
	}
	percent := float64(passedCount) / float64(total) * 100
	return fmt.Sprintf("过滤结果: %d/%d 通过 (%.0f%%)", passedCount, total, percent)
}

type SearchType string

const (
	SearchTypeExact  SearchType = "exact"  // 精确搜索
	SearchTypeFuzzy  SearchType = "fuzzy"  // 模糊搜索（glob）
	SearchTypeRegexp SearchType = "regexp" // 正则搜索
)

func (s *SearchMode) GenerateDesc() string {
	if s == nil {
		return ""
	}
	prefix := ""
	if s.IsRecursive {
		prefix = "Recursive "
	}
	matchModeDesc := ""
	if s.MatchMode != BothMatch && s.MatchModeStr != "" {
		matchModeDesc = fmt.Sprintf(" by %s", s.MatchModeStr)
	}
	switch s.SearchType {
	case SearchTypeExact:
		return fmt.Sprintf("%sExact Search【%s】%s", prefix, s.Pattern, matchModeDesc)
	case SearchTypeFuzzy:
		return fmt.Sprintf("%sFuzzy Search【%s】%s", prefix, s.Pattern, matchModeDesc)
	case SearchTypeRegexp:
		return fmt.Sprintf("%sRegexp Search【%s】%s", prefix, s.Pattern, matchModeDesc)
	default:
		return fmt.Sprintf("%sSearch【%s】%s", prefix, s.Pattern, matchModeDesc)
	}
}

func (s *SearchMode) GenerateDescZh() string {
	if s == nil {
		return ""
	}
	prefix := ""
	if s.IsRecursive {
		prefix = "递归"
	}
	matchModeDesc := ""
	if s.MatchMode != BothMatch && s.MatchModeStr != "" {
		matchModeDesc = fmt.Sprintf("通过%s", s.MatchModeStr)
	}
	switch s.SearchType {
	case SearchTypeExact:
		return fmt.Sprintf("%s精确搜索【%s】%s", prefix, s.Pattern, matchModeDesc)
	case SearchTypeFuzzy:
		return fmt.Sprintf("%s模糊搜索【%s】%s", prefix, s.Pattern, matchModeDesc)
	case SearchTypeRegexp:
		return fmt.Sprintf("%s正则搜索【%s】%s", prefix, s.Pattern, matchModeDesc)
	default:
		return fmt.Sprintf("%s搜索【%s】%s", prefix, s.Pattern, matchModeDesc)
	}
}

func GenerateNativeCallDesc(name string, configs []*RecursiveConfigItem) string {
	if len(configs) == 0 {
		return fmt.Sprintf("Native Call [%s]", name)
	}
	var params []string
	for _, cfg := range configs {
		if cfg != nil {
			params = append(params, fmt.Sprintf("%s=%s", cfg.Key, cfg.Value))
		}
	}
	if len(params) == 0 {
		return fmt.Sprintf("Native Call [%s]", name)
	}
	return fmt.Sprintf("Native Call [%s] with params: %s", name, strings.Join(params, ", "))
}

func GenerateNativeCallDescZh(name string, configs []*RecursiveConfigItem, nativeCallDesc string) string {
	if len(configs) == 0 {
		return fmt.Sprintf("调用 NativeCall【%s】；描述：%s", name, nativeCallDesc)
	}
	var params []string
	for _, cfg := range configs {
		if cfg != nil {
			params = append(params, fmt.Sprintf("%s=%s", cfg.Key, cfg.Value))
		}
	}
	if len(params) == 0 {
		return fmt.Sprintf("调用 NativeCall【%s】；描述：%s", name, nativeCallDesc)
	}
	return fmt.Sprintf("调用 NativeCall【%s】，参数：%s；描述：%s", name, strings.Join(params, ", "), nativeCallDesc)
}

// GenerateDesc 生成数据流分析的英文描述
func (m *DataFlowMode) GenerateDesc() string {
	if m == nil {
		return ""
	}

	var parts []string

	// 方向
	switch m.Direction {
	case DataFlowDirectionTopDef:
		parts = append(parts, "Get TopDefs")
	case DataFlowDirectionBottomUse:
		parts = append(parts, "Get BottomUse")
	default:
		parts = append(parts, "DataFlow Analysis")
	}

	// 深度配置
	var depthParts []string
	if m.Depth > 0 {
		depthParts = append(depthParts, fmt.Sprintf("depth=%d", m.Depth))
	}
	if m.DepthMin > 0 {
		depthParts = append(depthParts, fmt.Sprintf("min=%d", m.DepthMin))
	}
	if m.DepthMax > 0 {
		depthParts = append(depthParts, fmt.Sprintf("max=%d", m.DepthMax))
	}
	if len(depthParts) > 0 {
		parts = append(parts, fmt.Sprintf("[%s]", strings.Join(depthParts, ", ")))
	}

	// 配置规则
	for key, values := range m.Config {
		if len(values) > 0 {
			parts = append(parts, fmt.Sprintf("%s: %s", key, strings.Join(values, ", ")))
		}
	}

	return strings.Join(parts, " ")
}

// GenerateDescZh 生成数据流分析的中文描述
func (m *DataFlowMode) GenerateDescZh() string {
	if m == nil {
		return ""
	}

	var parts []string

	// 方向
	switch m.Direction {
	case DataFlowDirectionTopDef:
		parts = append(parts, "自顶向下数据流分析")
	case DataFlowDirectionBottomUse:
		parts = append(parts, "自底向上数据流分析")
	default:
		parts = append(parts, "数据流分析")
	}

	// 深度配置
	var depthParts []string
	if m.Depth > 0 {
		depthParts = append(depthParts, fmt.Sprintf("深度=%d", m.Depth))
	}
	if m.DepthMin > 0 {
		depthParts = append(depthParts, fmt.Sprintf("最小深度=%d", m.DepthMin))
	}
	if m.DepthMax > 0 {
		depthParts = append(depthParts, fmt.Sprintf("最大深度=%d", m.DepthMax))
	}
	if len(depthParts) > 0 {
		parts = append(parts, fmt.Sprintf("【%s】", strings.Join(depthParts, "，")))
	}

	// 配置规则（中文映射）
	configNameZh := map[string]string{
		"include": "包含规则",
		"exclude": "排除规则",
		"until":   "终止条件",
		"hook":    "hook条件",
	}
	for key, values := range m.Config {
		if len(values) > 0 {
			name := configNameZh[key]
			if name == "" {
				name = key
			}
			parts = append(parts, fmt.Sprintf("%s: %s", name, strings.Join(values, ", ")))
		}
	}

	return strings.Join(parts, " ")
}
