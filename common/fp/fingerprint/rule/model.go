package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"strings"
)

type MatchMethodParam struct {
	ExtParams map[string]any
	Info      *FingerprintInfo

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

type Pair struct {
	Key  string
	Name string
}
type FingerPrintRule struct {
	ActiveMode bool
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

func (f *FingerPrintRule) ToYaml() string {
	return ""
}

func (f *FingerPrintRule) ToExpression() string {
	return ""
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
		pushCode(&OpCode{Op: OpExtractData, data: []any{ref}})
		pushCode(&OpCode{Op: OpPush, data: []any{value}})
		pushCode(&OpCode{Op: OpContains})
	case "regexp":
		pushCode(&OpCode{Op: OpData})
		pushCode(&OpCode{Op: OpPush, data: []any{f.MatchParam.RegexpPattern}})
		pushCode(&OpCode{Op: OpRegexpMatch, data: []any{1}})
	case "md5":
		pushCode(&OpCode{Op: OpExtractData, data: []any{"md5"}})
		pushCode(&OpCode{Op: OpPush, data: []any{f.MatchParam.Md5}})
		pushCode(&OpCode{Op: OpEqual})
	case "http_header":
		pushCode(&OpCode{Op: OpExtractData, data: []any{"header_item", f.MatchParam.HeaderKey}})
		pushCode(&OpCode{Op: OpPush, data: []any{f.MatchParam.HeaderMatchRule.MatchParam.RegexpPattern}})
		pushCode(&OpCode{Op: OpRegexpMatch, data: []any{1}})
	case "complex":
		code := &OpCode{Op: OpJmpIfTrue}
		codes := []*OpCode{}
		switch f.MatchParam.Condition {
		case "or":
			code = &OpCode{Op: OpJmpIfTrue}
			for _, rule := range f.MatchParam.SubRules {
				codes = append(codes, rule.preToOpCodes()...)
				codes = append(codes, code)
			}
			res = append(res, codes...)
			code2 := &OpCode{Op: OpJmp}
			res = append(res, code2)
			code.data = []any{len(res)}
			res = append(res, &OpCode{Op: OpPush, data: []any{true}})
			code3 := &OpCode{Op: OpJmp}
			res = append(res, code3)
			code2.data = []any{len(res)}
			res = append(res, &OpCode{Op: OpPush, data: []any{false}})
			code3.data = []any{len(res)}
		case "and":
			code = &OpCode{Op: OpJmpIfFalse}
			for _, rule := range f.MatchParam.SubRules {
				codes = append(codes, rule.preToOpCodes()...)
				codes = append(codes, code)
			}
			res = append(res, codes...)
			code2 := &OpCode{Op: OpJmp}
			res = append(res, code2)
			code.data = []any{len(res)}
			res = append(res, &OpCode{Op: OpPush, data: []any{false}})
			code3 := &OpCode{Op: OpJmp}
			res = append(res, code3)
			code2.data = []any{len(res)}
			res = append(res, &OpCode{Op: OpPush, data: []any{true}})
			code3.data = []any{len(res)}
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

type FingerprintInfo struct {
	Proto          string `json:"proto"`
	ServiceName    string `json:"service_name"`
	ProductVerbose string `json:"product_verbose"`
	Info           string `json:"info"`
	Version        string `json:"version"`
	DeviceType     string `json:"device_type"`
	CPE            *CPE   `json:"cpes"`
	Raw            string `json:"raw"`
}
