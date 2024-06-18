package rule

import (
	"fmt"
	"strings"
)

type MatchMethodParam struct {
	ExtParams map[string]any
	// regexp
	RegexpPattern string

	// complex
	Condition string
	SubRules  []*FingerPrintRule

	// http header
	HeaderKey       string
	HeaderMatchRule *FingerPrintRule

	//md5
	Md5 string
}

type Pair struct {
	Key  string
	Name string
}
type FingerPrintRule struct {
	ActiveMode bool
	Method     string
	Info       *FingerprintInfo
	MatchParam *MatchMethodParam
}

func NewEmptyFingerPrintRule() *FingerPrintRule {
	return &FingerPrintRule{
		MatchParam: &MatchMethodParam{},
	}
}

func (f *FingerPrintRule) Marshal() []byte {
	return nil
}
func ParseFingerPrintRule([]byte) (*FingerPrintRule, error) {
	return nil, nil
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
