package sfdb

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io/fs"
	"strconv"
	"strings"
)

type PurposeType string
type RuleType string

const (
	PURPOSE_AUDIT    PurposeType = "audit"
	PURPOSE_VULN     PurposeType = "vuln"
	PURPOSE_CONFIG   PurposeType = "config"
	PURPOSE_SECURITY PurposeType = "securiy"
)

const (
	RULE_TYPE_YAK RuleType = "yak"
	RULE_TYPE_SF  RuleType = "sf"
)

func ValidRuleType(i any) RuleType {
	switch strings.ToLower(codec.AnyToString(i)) {
	case "yak", "y", "yaklang":
		return RULE_TYPE_YAK
	case "sf", "syntaxflow":
		return RULE_TYPE_SF
	default:
		return RULE_TYPE_SF
	}
}

func ValidPurpose(i any) PurposeType {
	switch strings.ToLower(codec.AnyToString(i)) {
	case "audit", "a", "audition":
		return PURPOSE_AUDIT
	case "vuln", "v", "vulnerability", "vul", "vulnerabilities", "weak", "weakness":
		return PURPOSE_VULN
	case "config", "c", "configuration", "conf", "configure":
		return PURPOSE_CONFIG
	case "security", "s", "secure", "securely", "secureity":
		return PURPOSE_SECURITY
	default:
		return PURPOSE_AUDIT
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
	Type    RuleType
	Content string

	// Purpose is the purpose of the rule.
	// audit / vuln / config / security / information
	Purpose PurposeType

	// DemoFileSystem will description the file system of the rule.
	// This is a json string.
	//    save map[string]quotedString
	TypicalHitFileSystem string

	Hash string `json:"hash" gorm:"unique_index"`
}

func (s *SyntaxFlowRule) CalcHash() string {
	return utils.CalcSha256(s.Content)
}

func (s *SyntaxFlowRule) BeforeSave() error {
	s.Hash = s.CalcHash()
	s.Purpose = ValidPurpose(s.Purpose)
	s.Type = ValidRuleType(s.Type)
	return nil
}

func (s *SyntaxFlowRule) LoadFileSystem(system filesys.FileSystem) error {
	f := make(map[string]string)
	filesys.Recursive(".", filesys.WithFileSystem(system), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		raw, err := system.ReadFile(s)
		if err != nil {
			return nil
		}
		f[s] = strconv.Quote(string(raw))
		return nil
	}))
	raw, err := json.Marshal(f)
	if err != nil {
		return utils.Wrapf(err, `failed to marshal file system`)
	}
	s.TypicalHitFileSystem = string(raw)
	return nil
}

func (s *SyntaxFlowRule) BuildFileSystem() (filesys.FileSystem, error) {
	f := make(map[string]string)
	err := json.Unmarshal([]byte(s.TypicalHitFileSystem), &f)
	if err != nil {
		return nil, utils.Wrapf(err, `failed to unmarshal file system`)
	}
	fs := filesys.NewVirtualFs()
	for filename, i := range f {
		raw, err := strconv.Unquote(i)
		if err != nil {
			continue
		}
		fs.AddFile(filename, raw)
	}
	return fs, nil
}

func (s *SyntaxFlowRule) Valid() error {
	fs, err := s.BuildFileSystem()
	if err != nil {
		return err
	}
	prog, err := ssaapi.ParseProject(fs)
	if err != nil {
		return err
	}
	result, err := prog.SyntaxFlowWithError(s.Content)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return utils.Errorf(`runtime error: %v`, result.Errors)
	}
	return nil
}
