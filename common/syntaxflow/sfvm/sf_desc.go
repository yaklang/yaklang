package sfvm

import (
	"strings"
)

type SFDescKeyType string

const (
	SFDescKeyType_Unknown   SFDescKeyType = "unknown"
	SFDescKeyType_Title     SFDescKeyType = "title"
	SFDescKeyType_Title_ZH  SFDescKeyType = "title_zh"
	SFDescKeyType_Desc      SFDescKeyType = "desc"
	SFDescKeyType_Type      SFDescKeyType = "type"
	SFDescKeyType_Lib       SFDescKeyType = "lib"
	SFDescKeyType_Level     SFDescKeyType = "level"
	SFDescKeyType_Lang      SFDescKeyType = "language"
	SFDescKeyType_CVE       SFDescKeyType = "cve"
	SFDescKeyType_CWE       SFDescKeyType = "cwe"
	SFDescKeyType_Risk      SFDescKeyType = "risk"
	SFDescKeyType_Solution  SFDescKeyType = "solution"
	SFDescKeyType_Rule_Id   SFDescKeyType = "rule_id"
	SFDescKeyType_Reference SFDescKeyType = "reference"
	SFDescKeyType_Message   SFDescKeyType = "message"
	SFDescKeyType_Name      SFDescKeyType = "name"
	// SFDescKeyType_Mode selects execution backend: "source" → sfpattern (no SSA);
	// empty / other → default sfvm on SSA.
	SFDescKeyType_Mode SFDescKeyType = "mode"
)

const (
	// RuleModeSource marks rules that scan source files without SSA IR (sfpattern).
	RuleModeSource = "source"
	// RuleModeSSA marks rules that run on SSA IR (default).
	RuleModeSSA = "ssa"
	// RuleModeStruct marks rules that scan the current compile unit's resident SSA.
	RuleModeStruct = "struct"
)

func ValidDescItemKeyType(key string) SFDescKeyType {
	switch strings.ToLower(key) {
	case "title":
		return SFDescKeyType_Title
	case "title_zh":
		return SFDescKeyType_Title_ZH
	case "description", "desc", "note":
		return SFDescKeyType_Desc
	case "type", "purpose":
		return SFDescKeyType_Type
	case "lib", "allow_include", "as_library", "as_lib", "library_name":
		return SFDescKeyType_Lib
	case "level", "severity", "sev":
		return SFDescKeyType_Level
	case "language", "lang":
		return SFDescKeyType_Lang
	case "cve":
		return SFDescKeyType_CVE
	case "cwe":
		return SFDescKeyType_CWE
	case "risk_type", "risk":
		return SFDescKeyType_Risk
	case "solution", "fix":
		return SFDescKeyType_Solution
	case "rule_id", "id":
		return SFDescKeyType_Rule_Id
	case "reference", "ref":
		return SFDescKeyType_Reference
	case "message", "msg":
		return SFDescKeyType_Message
	case "mode", "engine", "exec_mode":
		return SFDescKeyType_Mode
	default:
		return SFDescKeyType_Unknown
	}
}

// AppendRuleTag appends tag if not already present (pipe-separated).
func AppendRuleTag(existing, tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return existing
	}
	if existing == "" {
		return tag
	}
	for _, part := range strings.Split(existing, "|") {
		if strings.EqualFold(strings.TrimSpace(part), tag) {
			return existing
		}
	}
	return existing + "|" + tag
}

// FrameIsStructMode reports whether a compiled frame should run via StructQueryTarget.
// Mode is written onto the rule during SyntaxFlow compile (desc(mode: ...)).
func FrameIsStructMode(frame *SFFrame) bool {
	if frame == nil {
		return false
	}
	return frame.GetRule().IsStructMode()
}

// FrameIsSourceMode reports whether a compiled frame should run via sfpattern.
func FrameIsSourceMode(frame *SFFrame) bool {
	if frame == nil {
		return false
	}
	return frame.GetRule().IsSourceMode()
}

// GetSupplyInfoDescKeyType 拿到所有desc item中，
// 用于给规则扩充提示信息的key
func GetSupplyInfoDescKeyType() []SFDescKeyType {
	return []SFDescKeyType{
		SFDescKeyType_Title,
		SFDescKeyType_Title_ZH,
		SFDescKeyType_Desc,
		SFDescKeyType_Solution,
		SFDescKeyType_Reference,
	}
}

func GetAlertDescKeyType() []SFDescKeyType {
	return []SFDescKeyType{
		SFDescKeyType_Name,
		SFDescKeyType_Title,
		SFDescKeyType_Title_ZH,
		SFDescKeyType_Message,
		SFDescKeyType_Solution,
		SFDescKeyType_Risk,
		SFDescKeyType_Desc,
	}
}

func IsComplexDescType(typ SFDescKeyType) bool {
	switch typ {
	case SFDescKeyType_Desc, SFDescKeyType_Solution, SFDescKeyType_Reference:
		return true
	default:
		return false
	}
}
