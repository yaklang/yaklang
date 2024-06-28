package rule

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/go-funk"
	"strings"
)

type MatchResource struct {
	Data     []byte
	Port     int
	Protocol string
}

func NewHttpResource(data []byte) *MatchResource {
	return &MatchResource{
		Data:     data,
		Protocol: "http",
	}
}

type GeneralRule struct {
	gorm.Model
	*CPE
	MatchExpression string `gorm:"uniqueIndex"`
}

func init() {
	db := consts.GetGormProjectDatabase()
	db.AutoMigrate(&GeneralRule{})
}

type MatchMethodParam struct {
	ExtParams map[string]any
	Info      *CPE

	// regexp
	RegexpPattern string
	Keyword       *webfingerprint.KeywordMatcher

	// complex
	Condition string
	SubRules  []*FingerPrintRule

	// http header
	HeaderKey       string
	HeaderMatchRule *FingerPrintRule

	//md5
	Md5 string

	// exp
	Params []any
	Op     string
}

type FingerPrintRule struct {
	Method     string
	WebPath    string
	MatchParam *MatchMethodParam
}

func NewEmptyFingerPrintRule() *FingerPrintRule {
	return &FingerPrintRule{
		MatchParam: &MatchMethodParam{},
	}
}

type CPE struct {
	Part     string `yaml:"part,omitempty" json:"part"`
	Vendor   string `yaml:"vendor,omitempty" json:"vendor"`
	Product  string `yaml:"product,omitempty" json:"product"`
	Version  string `yaml:"version,omitempty" json:"version"`
	Update   string `yaml:"update,omitempty" json:"update"`
	Edition  string `yaml:"edition,omitempty" json:"edition"`
	Language string `yaml:"language,omitempty" json:"language"`
}

func (f *FingerPrintRule) preToOpCodes() []*OpCode {
	res := []*OpCode{}
	pushCode := func(code *OpCode) {
		res = append(res, code)
	}
	switch f.Method {
	case "exp":
		if len(f.MatchParam.Params) != 2 {
			return nil
		}
		strParams := []string{}
		for _, param := range f.MatchParam.Params {
			p, ok := param.(string)
			if !ok {
				return nil
			}
			strParams = append(strParams, p)
		}
		ref := strParams[0]
		value := strParams[1]
		pushCode(&OpCode{Op: OpExtractData, data: []any{f.WebPath, ref}})
		pushCode(&OpCode{Op: OpPush, data: []any{value}})
		if f.MatchParam.Op == "=" {
			pushCode(&OpCode{Op: OpContains})
		} else if f.MatchParam.Op == "!=" {
			pushCode(&OpCode{Op: OpContains})
			pushCode(&OpCode{Op: OpNot})
		}
	case "regexp":
		pushCode(&OpCode{Op: OpData, data: []any{f.WebPath}})
		pushCode(&OpCode{Op: OpPush, data: []any{f.MatchParam.RegexpPattern}})
		extGroup := []any{f.MatchParam.Keyword.VersionIndex, f.MatchParam.Keyword.ProductIndex, f.MatchParam.Keyword.VersionIndex, f.MatchParam.Keyword.UpdateIndex, f.MatchParam.Keyword.EditionIndex, f.MatchParam.Keyword.LanguageIndex}
		if !funk.Any(extGroup...) {
			extGroup = nil
		}
		pushCode(&OpCode{Op: OpRegexpMatch, data: extGroup})
	case "md5":
		pushCode(&OpCode{Op: OpExtractData, data: []any{f.WebPath, "md5"}})
		pushCode(&OpCode{Op: OpPush, data: []any{f.MatchParam.Md5}})
		pushCode(&OpCode{Op: OpEqual})
	case "http_header":
		pushCode(&OpCode{Op: OpExtractData, data: []any{f.WebPath, "header_item", f.MatchParam.HeaderKey}})
		subParam := f.MatchParam.HeaderMatchRule.MatchParam
		pushCode(&OpCode{Op: OpPush, data: []any{subParam.RegexpPattern}})
		extGroup := []any{subParam.Keyword.VersionIndex, subParam.Keyword.ProductIndex, subParam.Keyword.VersionIndex, subParam.Keyword.UpdateIndex, subParam.Keyword.EditionIndex, subParam.Keyword.LanguageIndex}
		if !funk.Any(extGroup...) {
			extGroup = nil
		}
		pushCode(&OpCode{Op: OpRegexpMatch, data: extGroup})
	case "complex":
		jmpPoint := map[*OpCode]int{}
		codes := []*OpCode{}
		switch f.MatchParam.Condition {
		case "or":
			for i, rule := range f.MatchParam.SubRules {
				codes = append(codes, rule.preToOpCodes()...)
				if i == len(f.MatchParam.SubRules)-1 {
					continue
				}
				code := &OpCode{Op: OpOr}
				jmpPoint[code] = len(codes)
				codes = append(codes, code)
			}
			res = append(res, codes...)
			for opCode, i := range jmpPoint {
				opCode.data = []any{len(codes) - i}
			}
		case "and":
			for i, rule := range f.MatchParam.SubRules {
				codes = append(codes, rule.preToOpCodes()...)
				if i == len(f.MatchParam.SubRules)-1 {
					continue
				}
				code := &OpCode{Op: OpAnd}
				jmpPoint[code] = len(codes)
				codes = append(codes, code)
			}
			res = append(res, codes...)
			for opCode, i := range jmpPoint {
				opCode.data = []any{len(codes) - i}
			}
		}
	default:
		return nil
	}
	return res
}
func (f *FingerPrintRule) ToOpCodes() []*OpCode {
	codes := f.preToOpCodes()
	codes = append(codes, &OpCode{Op: OpInfo, data: []any{f.MatchParam.Info}})
	return codes
}

func (c *CPE) init() {
	if c.Part == "" {
		c.Part = "a"
	}

	setWildstart := func(raw *string) {
		if *raw == "" {
			*raw = "*"
		}
	}

	setWildstart(&c.Vendor)
	setWildstart(&c.Product)
	setWildstart(&c.Version)
	setWildstart(&c.Update)
	setWildstart(&c.Edition)
	setWildstart(&c.Language)
}

func (c *CPE) String() string {
	c.init()
	raw := fmt.Sprintf("cpe:/%s:%s:%s:%s:%s:%s:%s", c.Part, c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language)
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ToLower(raw)
	return raw
}
