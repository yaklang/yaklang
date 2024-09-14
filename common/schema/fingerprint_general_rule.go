package schema

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

type CPE struct {
	Part     string `yaml:"part,omitempty" json:"part"`
	Vendor   string `yaml:"vendor,omitempty" json:"vendor"`
	Product  string `yaml:"product,omitempty" json:"product"`
	Version  string `yaml:"version,omitempty" json:"version"`
	Update   string `yaml:"update,omitempty" json:"update"`
	Edition  string `yaml:"edition,omitempty" json:"edition"`
	Language string `yaml:"language,omitempty" json:"language"`
}

func (c *CPE) Init() {
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
	c.Init()
	raw := fmt.Sprintf("cpe:/%s:%s:%s:%s:%s:%s:%s", c.Part, c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language)
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ToLower(raw)
	return raw
}

type GeneralRule struct {
	gorm.Model
	*CPE
	RuleName        string `json:"指纹名称" gorm:"unique_index"`
	WebPath         string `json:"web路径"`
	ExtInfo         string
	MatchExpression string `json:"指纹规则"`
}

func (g *GeneralRule) String() string {
	items := []string{fmt.Sprintf("name:%s", g.RuleName)}
	cpeStr := g.CPE.String()
	items = append(items, fmt.Sprintf("cpe:%s", cpeStr))

	if g.WebPath != "" {
		items = append(items, fmt.Sprintf("webpath:%s", g.WebPath))
	}
	if g.ExtInfo != "" {
		items = append(items, fmt.Sprintf("info:%s", g.ExtInfo))
	}
	items = append(items, fmt.Sprintf("rule:%s", g.MatchExpression))
	return strings.Join(items, " ")
}

func FromFingerprintGRPCModel(gr *ypb.FingerprintRule) *GeneralRule {
	if gr == nil {
		return nil
	}
	cpe := &CPE{}
	if gr.Cpe != nil {
		cpe.Part = gr.Cpe.Part
		cpe.Vendor = gr.Cpe.Vendor
		cpe.Product = gr.Cpe.Product
		cpe.Version = gr.Cpe.Version
		cpe.Update = gr.Cpe.Update
		cpe.Edition = gr.Cpe.Edition
		cpe.Language = gr.Cpe.Language
	}
	rule := &GeneralRule{
		CPE:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
	}
	rule.ID = uint(gr.Id)
	return rule
}

func (gr *GeneralRule) ToGRPCModel() *ypb.FingerprintRule {
	if gr == nil {
		return nil
	}
	cpe := &ypb.CPE{}
	if gr.CPE != nil {
		cpe = &ypb.CPE{
			Part:    gr.Part,
			Vendor:  gr.Vendor,
			Product: gr.Product,
			Version: gr.Version,
			Update:  gr.Update,
			Edition: gr.Edition,
		}
	}

	return &ypb.FingerprintRule{
		Cpe:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
	}
}
