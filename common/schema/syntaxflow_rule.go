package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

type SyntaxFlowRulePurposeType string
type SyntaxFlowRuleType string

const (
	SFR_PURPOSE_AUDIT    SyntaxFlowRulePurposeType = "audit"
	SFR_PURPOSE_VULN     SyntaxFlowRulePurposeType = "vuln"
	SFR_PURPOSE_CONFIG   SyntaxFlowRulePurposeType = "config"
	SFR_PURPOSE_SECURITY SyntaxFlowRulePurposeType = "securiy"
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

func ValidPurpose(i any) SyntaxFlowRulePurposeType {
	switch strings.ToLower(codec.AnyToString(i)) {
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

	// Language is the language of the rule.
	// if the rule is not set, all languages will be used.
	Language string

	Title       string
	TitleZh     string
	Description string

	// yak or sf
	Type    SyntaxFlowRuleType
	Content string

	// Purpose is the purpose of the rule.
	// audit / vuln / config / security / information
	Purpose SyntaxFlowRulePurposeType

	// DemoFileSystem will description the file system of the rule.
	// This is a json string.
	//    save map[string]quotedString
	TypicalHitFileSystem []byte
	Verified             bool

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
	return nil
}
