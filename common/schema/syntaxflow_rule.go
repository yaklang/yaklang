package schema

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SyntaxFlowRulePurposeType string
type SyntaxFlowRuleType string
type SyntaxFlowSeverity string

const (
	SFR_PURPOSE_AUDIT    SyntaxFlowRulePurposeType = "audit"
	SFR_PURPOSE_VULN     SyntaxFlowRulePurposeType = "vuln"
	SFR_PURPOSE_CONFIG   SyntaxFlowRulePurposeType = "config"
	SFR_PURPOSE_SECURITY SyntaxFlowRulePurposeType = "security"
)

func GetAllSFPurposeTypes() []string {
	return []string{
		string(SFR_PURPOSE_AUDIT),
		string(SFR_PURPOSE_VULN),
		string(SFR_PURPOSE_CONFIG),
		string(SFR_PURPOSE_SECURITY),
	}
}

func GetAllSFSupportLanguage() []string {
	return []string{
		"yak",
		"java",
		"javaScript",
		"php",
		"golang",
		"general", // 通用规则
	}
}

const (
	SFR_SEVERITY_INFO     = "info"
	SFR_SEVERITY_LOW      = "low"
	SFR_SEVERITY_WARNING  = "middle"
	SFR_SEVERITY_CRITICAL = "critical"
	SFR_SEVERITY_HIGH     = "high"
)

func GetAllSFSeverityTypes() []string {
	return []string{
		SFR_SEVERITY_INFO,
		SFR_SEVERITY_LOW,
		SFR_SEVERITY_WARNING,
		SFR_SEVERITY_CRITICAL,
		SFR_SEVERITY_HIGH,
	}
}

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
	case "info", "i", "verbose", "debug", "prompt":
		return SFR_SEVERITY_INFO
	case "warning", "w", "middle", "mid", "warn", "medium":
		return SFR_SEVERITY_WARNING
	case "critical", "c", "fatal", "e", "essential":
		return SFR_SEVERITY_CRITICAL
	case "high", "h", "error":
		return SFR_SEVERITY_HIGH
	case "low":
		return SFR_SEVERITY_LOW
	default:
		return SFR_SEVERITY_INFO
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

type MapEx[K comparable, V any] map[K]V

func (m *MapEx[K, V]) Scan(value interface{}) error {
	return json.Unmarshal(codec.AnyToBytes(value), m)
}
func (m MapEx[K, V]) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type SlicesEx[K comparable] []K

func (s *SlicesEx[K]) Scan(value interface{}) error {
	return json.Unmarshal(codec.AnyToBytes(value), s)
}

func (s *SlicesEx[K]) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// for describe the rule and create risk
type SyntaxFlowDescInfo struct {
	RuleId      string `json:"rule_id"`
	Title       string `json:"title"`
	TitleZh     string `json:"title_zh"`
	Description string `json:"description"`
	Solution    string `json:"solution"`
	Tag         string `json:"tag"`
	// info / warning / critical
	Severity SyntaxFlowSeverity `json:"severity"`
	// Purpose is the purpose of the rule.
	// audit / vuln / config / security / information
	Purpose SyntaxFlowRulePurposeType `json:"purpose"`

	OnlyMsg   bool   `json:"only_msg"`
	Msg       string `json:"msg"`
	CVE       string `json:"cve"`
	RiskType  string
	ExtraInfo map[string]string `json:"extra_info"`
}

func (info *SyntaxFlowDescInfo) String() string {
	if info.OnlyMsg {
		return info.Msg
	}
	return fmt.Sprintf("%s: %s", info.Title, info.Description)
}

type SyntaxFlowRule struct {
	gorm.Model
	RuleId        string `gorm:"unique_index"`
	IsBuildInRule bool

	// Language is the language of the rule.
	// if the rule is not set, all languages will be used.
	Language string

	RuleName    string `gorm:"unique_index"`
	Title       string
	TitleZh     string
	Description string
	Tag         string
	AlertDesc   MapEx[string, *SyntaxFlowDescInfo] `gorm:"type:text"`
	CVE         string
	// yak or sf
	RiskType string
	Type     SyntaxFlowRuleType
	Severity SyntaxFlowSeverity
	Content  string

	// Purpose is the purpose of the rule.
	// audit / vuln / config / security / information
	Purpose  SyntaxFlowRulePurposeType
	Solution string
	// DemoFileSystem will description the file system of the rule.
	// This is a json string.
	//    save map[string]quotedString
	TypicalHitFileSystem []byte
	Verified             bool

	// AllowIncluded is the rule can be included by other rules.
	// If the rule is included by other rules, the rule will not be shown in the result.
	AllowIncluded bool
	IncludedName  string
	OpCodes       string

	Groups []*SyntaxFlowGroup `gorm:"many2many:syntax_flow_rule_and_group;"`

	Hash string `json:"hash" gorm:"unique_index"`
}

func (s *SyntaxFlowRule) CalcHash() string {
	s.Hash = utils.CalcSha256(s.RuleName, s.Content, s.Tag, s.OpCodes)
	return s.Hash
}

func (s *SyntaxFlowRule) BeforeSave() error {
	s.CalcHash()
	s.Purpose = ValidPurpose(s.Purpose)
	s.Type = ValidRuleType(s.Type)
	s.Severity = ValidSeverityType(s.Severity)
	return nil
}
func (s *SyntaxFlowRule) GetAlertInfo(name string) (*SyntaxFlowDescInfo, bool) {
	if info, ok := s.AlertDesc[name]; ok {
		return info, true
	}
	return nil, false
}

func (s *SyntaxFlowRule) GetInfo() *SyntaxFlowDescInfo {
	// load info from rule self, and create new syntaxflowdescinfo return
	info := &SyntaxFlowDescInfo{
		RuleId:      s.RuleId,
		Title:       s.Title,
		TitleZh:     s.TitleZh,
		Description: s.Description,
		Solution:    s.Solution,
		Tag:         s.Tag,
		Severity:    s.Severity,
		Purpose:     s.Purpose,
		OnlyMsg:     false,
		Msg:         "",
		CVE:         s.CVE,
		RiskType:    s.RiskType,
	}
	return info
}

func (s *SyntaxFlowRule) ToGRPCModel() *ypb.SyntaxFlowRule {
	groupNames := make([]string, 0, len(s.Groups))
	for _, group := range s.Groups {
		groupNames = append(groupNames, group.GroupName)
	}
	sfRule := &ypb.SyntaxFlowRule{
		Id:            int64(s.ID),
		IsBuildInRule: s.IsBuildInRule,
		Language:      s.Language,
		RuleName:      s.RuleName,
		Title:         s.Title,
		TitleZh:       s.TitleZh,
		Description:   s.Description,
		Type:          string(s.Type),
		Severity:      string(s.Severity),
		Content:       s.Content,
		Purpose:       string(s.Purpose),
		Verified:      s.Verified,
		AllowIncluded: s.AllowIncluded,
		IncludedName:  s.IncludedName,
		Hash:          s.Hash,
		Tag:           s.Tag,
		GroupName:     groupNames,
	}
	return sfRule
}
