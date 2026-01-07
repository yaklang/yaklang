package cveresources

import (
	"context"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CWE struct {
	Id     int    `json:"id" gorm:"primary_key"`
	IdStr  string `json:"id_str" gorm:"uniqueIndex"`
	Name   string
	NameZh string

	// 描述 CWE 之间的关系
	Parent   string `json:"parent"`   // 父子关系
	Siblings string `json:"siblings"` // 兄弟关系
	InferTo  string `json:"infer_to"` // 推导关系(有上一个问题，多半也会有这个问题)
	Requires string `json:"requires"` // 依赖关系

	Status                string // CWE 发布状态 draft / incomplete / stable
	Stable                bool
	Incomplete            bool
	Description           string
	DescriptionZh         string
	ExtendedDescription   string
	ExtendedDescriptionZh string
	Abstraction           string // base / varint
	RelativeLanguage      string // 可能出现的语言
	CWESolution           string // 修复方案
	CVEExamples           string // 典型 CVE 案例
	CAPECVectors          string
}

func (c *CWE) ToGRPCModel() *ypb.CWEDetail {
	return &ypb.CWEDetail{
		CWE: c.CWEString(), Name: c.Name, NameZh: c.NameZh,
		Stable: c.Stable, Incomplete: c.Incomplete,
		Status: StatusVerbose(c.Status), Description: c.Description, DescriptionZh: c.DescriptionZh,
		LongDescription: c.ExtendedDescription, LongDescriptionZh: c.ExtendedDescriptionZh,
		RelativeLanguage: utils.PrettifyListFromStringSplitEx(c.RelativeLanguage, ",", "|"),
		Solution:         c.CWESolution,
		RelativeCVE:      utils.PrettifyListFromStringSplitEx(c.CVEExamples, ","),
	}
}

func StatusVerbose(i string) string {
	i = strings.ToLower(i)
	switch i {
	case "draft":
		return "草案"
	case "incomplete":
		return "不完整"
	case "stable":
		return "稳定"
	default:
		return "-"
	}
}

func CreateOrUpdateCWE(db *gorm.DB, id string, i interface{}) error {
	if db := db.Where("id_str = ?", id).Assign(i).FirstOrCreate(&CWE{}); db.Error != nil {
		log.Errorf("save cwe failed: 5s")
		return db.Error
	}
	return nil
}

func GetCWE(db *gorm.DB, id string) (*CWE, error) {
	var cwe CWE
	if db := db.Where("id_str = ?", id).First(&cwe); db.Error != nil {
		return nil, db.Error
	}
	return &cwe, nil
}

func (c *CWE) CWEString() string {
	return "CWE-" + c.IdStr
}

// String returns a brief string representation of the CWE
func (c *CWE) String() string {
	return c.CWEString() + ": " + c.Name
}

// HumanReading returns a human-readable description of the CWE for AI learning materials
func (c *CWE) HumanReading() string {
	var builder strings.Builder

	// Title section
	builder.WriteString("# ")
	builder.WriteString(c.CWEString())
	builder.WriteString(": ")
	builder.WriteString(c.Name)
	if c.NameZh != "" {
		builder.WriteString(" (")
		builder.WriteString(c.NameZh)
		builder.WriteString(")")
	}
	builder.WriteString("\n\n")

	// Status section
	builder.WriteString("## Status\n")
	builder.WriteString("- Status: ")
	builder.WriteString(StatusVerbose(c.Status))
	builder.WriteString("\n")
	builder.WriteString("- Abstraction: ")
	builder.WriteString(c.Abstraction)
	builder.WriteString("\n\n")

	// Description section
	builder.WriteString("## Description\n")
	if c.Description != "" {
		builder.WriteString(c.Description)
		builder.WriteString("\n")
	}
	if c.DescriptionZh != "" {
		builder.WriteString("\n**Chinese:** ")
		builder.WriteString(c.DescriptionZh)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	// Extended Description section
	if c.ExtendedDescription != "" {
		builder.WriteString("## Extended Description\n")
		builder.WriteString(c.ExtendedDescription)
		builder.WriteString("\n")
		if c.ExtendedDescriptionZh != "" {
			builder.WriteString("\n**Chinese:** ")
			builder.WriteString(c.ExtendedDescriptionZh)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// Relationships section
	hasRelationships := c.Parent != "" || c.Siblings != "" || c.InferTo != "" || c.Requires != ""
	if hasRelationships {
		builder.WriteString("## Relationships\n")
		if c.Parent != "" {
			builder.WriteString("- Parent CWEs: ")
			builder.WriteString(formatCWEList(c.Parent))
			builder.WriteString("\n")
		}
		if c.Siblings != "" {
			builder.WriteString("- Sibling CWEs: ")
			builder.WriteString(formatCWEList(c.Siblings))
			builder.WriteString("\n")
		}
		if c.InferTo != "" {
			builder.WriteString("- Can Lead To: ")
			builder.WriteString(formatCWEList(c.InferTo))
			builder.WriteString("\n")
		}
		if c.Requires != "" {
			builder.WriteString("- Requires: ")
			builder.WriteString(formatCWEList(c.Requires))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// Applicable Platforms section
	if c.RelativeLanguage != "" {
		builder.WriteString("## Applicable Languages\n")
		builder.WriteString(c.RelativeLanguage)
		builder.WriteString("\n\n")
	}

	// Solution section
	if c.CWESolution != "" {
		builder.WriteString("## Solution\n")
		builder.WriteString(c.CWESolution)
		builder.WriteString("\n\n")
	}

	// Related CVEs section
	if c.CVEExamples != "" {
		builder.WriteString("## Related CVE Examples\n")
		cves := strings.Split(c.CVEExamples, ",")
		for i, cve := range cves {
			if i > 9 { // Show at most 10 CVEs
				builder.WriteString("- ... and more\n")
				break
			}
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(cve))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// CAPEC Vectors section
	if c.CAPECVectors != "" {
		builder.WriteString("## Related CAPEC Attack Patterns\n")
		capecs := strings.Split(c.CAPECVectors, ",")
		for _, capec := range capecs {
			builder.WriteString("- CAPEC-")
			builder.WriteString(strings.TrimSpace(capec))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// formatCWEList formats a comma-separated list of CWE IDs into a readable format
func formatCWEList(cweIds string) string {
	ids := strings.Split(cweIds, ",")
	var formatted []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			formatted = append(formatted, "CWE-"+id)
		}
	}
	return strings.Join(formatted, ", ")
}

func (c *CWE) BeforeSave() error {
	if c.Id <= 0 {
		c.Id, _ = strconv.Atoi(c.IdStr)
	}
	if c.Id <= 0 {
		return utils.Error("save error for emtpy id")
	}
	c.Stable = strings.ToLower(c.Status) == "stable"
	c.Incomplete = strings.ToLower(c.Status) == "incomplete"
	return nil
}
func YieldCWEs(db *gorm.DB, ctx context.Context) chan *CWE {
	return bizhelper.YieldModel[*CWE](ctx, db, bizhelper.WithYieldModel_PageSize(1000))
}

func GetCWEById(db *gorm.DB, id int) (*CWE, error) {
	var cwe CWE
	if db = db.Where("id = ?", id).First(&cwe); db.Error != nil {
		return nil, db.Error
	}
	return &cwe, nil
}
