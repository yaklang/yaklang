package schema

import (
	"fmt"
	"github.com/samber/lo"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	MatchExpression string              `json:"指纹规则"`
	Groups          []*GeneralRuleGroup `gorm:"many2many:general_rule_and_group;"`
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

func GRPCGeneralRuleToSchemaGeneralRule(gr *ypb.FingerprintRule) *GeneralRule {
	if gr == nil {
		return nil
	}
	cpe := &CPE{}
	if gr.CPE != nil {
		cpe.Part = gr.CPE.Part
		cpe.Vendor = gr.CPE.Vendor
		cpe.Product = gr.CPE.Product
		cpe.Version = gr.CPE.Version
		cpe.Update = gr.CPE.Update
		cpe.Edition = gr.CPE.Edition
		cpe.Language = gr.CPE.Language
	}
	rule := &GeneralRule{
		CPE:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
		Groups: lo.Map(gr.GroupName, func(name string, _ int) *GeneralRuleGroup {
			return &GeneralRuleGroup{GroupName: name}
		}),
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
		Id:              int64(gr.ID),
		CPE:             cpe,
		RuleName:        gr.RuleName,
		WebPath:         gr.WebPath,
		ExtInfo:         gr.ExtInfo,
		MatchExpression: gr.MatchExpression,
		GroupName: lo.Map(gr.Groups, func(g *GeneralRuleGroup, _ int) string {
			return g.GroupName
		}),
	}
}
