package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

type SyntaxFlowRulePurposeType string
type SyntaxFlowRuleType string
type SyntaxFlowSeverity string

const (
	SFR_PURPOSE_AUDIT    SyntaxFlowRulePurposeType = "audit"
	SFR_PURPOSE_VULN     SyntaxFlowRulePurposeType = "vuln"
	SFR_PURPOSE_CONFIG   SyntaxFlowRulePurposeType = "config"
	SFR_PURPOSE_SECURITY SyntaxFlowRulePurposeType = "securiy"
)

const (
	SFR_SEVERITY_LOW      = "info"
	SFR_SEVERITY_WARNING  = "middle"
	SFR_SEVERITY_CRITICAL = "critical"
	SFR_SEVERITY_HIGH     = "high"
)

const (
	SFR_RULE_TYPE_YAK SyntaxFlowRuleType = "yak"
	SFR_RULE_TYPE_SF  SyntaxFlowRuleType = "sf"
)

func ValidRuleType(i any) SyntaxFlowRuleType {
	switch strings.ToLower(codec.AnyToString(i)) {
	case "yak", "y", "yaklang":
		return SFR_RULE_TYPE_YAK
	case "sf", "syntaxflow":
		return SFR_RULE_TYPE_SF
	default:
		return SFR_RULE_TYPE_SF
	}
}

func ValidSeverityType(i any) SyntaxFlowSeverity {
	switch strings.ToLower(yakunquote.TryUnquote(codec.AnyToString(i))) {
	case "info", "i", "low", "verbose", "debug", "prompt":
		return SFR_SEVERITY_LOW
	case "warning", "w", "middle", "mid", "warn":
		return SFR_SEVERITY_WARNING
	case "critical", "c", "fatal", "e", "essential":
		return SFR_SEVERITY_CRITICAL
	case "high", "h", "error":
		return SFR_SEVERITY_HIGH
	default:
		return SFR_SEVERITY_LOW
	}
}

func ValidPurpose(i any) SyntaxFlowRulePurposeType {
	result := yakunquote.TryUnquote(codec.AnyToString(i))
	switch strings.ToLower(result) {
	case "audit", "a", "audition":
		return SFR_PURPOSE_AUDIT
	case "vuln", "v", "vulnerability", "vul", "vulnerabilities", "weak", "weakness":
		return SFR_PURPOSE_VULN
	case "config", "c", "configuration", "conf", "configure":
		return SFR_PURPOSE_CONFIG
	case "security", "s", "secure", "securely", "secureity":
		return SFR_PURPOSE_SECURITY
	default:
		return SFR_PURPOSE_AUDIT
	}
}

type SyntaxFlowRule struct {
	gorm.Model

	IsBuildInRule bool

	// Language is the language of the rule.
	// if the rule is not set, all languages will be used.
	Language string

	RuleName    string
	Title       string
	TitleZh     string
	Description string

	// yak or sf
	Type     SyntaxFlowRuleType
	Severity SyntaxFlowSeverity
	Content  string

	// Purpose is the purpose of the rule.
	// audit / vuln / config / security / information
	Purpose SyntaxFlowRulePurposeType

	// DemoFileSystem will description the file system of the rule.
	// This is a json string.
	//    save map[string]quotedString
	TypicalHitFileSystem []byte
	Verified             bool

	// AllowIncluded is the rule can be included by other rules.
	// If the rule is included by other rules, the rule will not be shown in the result.
	AllowIncluded bool
	IncludedName  string

	Hash string `json:"hash" gorm:"unique_index"`
}

func (s *SyntaxFlowRule) CalcHash() string {
	s.Hash = utils.CalcSha256(s.Content)
	return s.Hash
}

func (s *SyntaxFlowRule) BeforeSave() error {
	s.CalcHash()
	s.Purpose = ValidPurpose(s.Purpose)
	s.Type = ValidRuleType(s.Type)
	s.Severity = ValidSeverityType(s.Severity)
	return nil
}
