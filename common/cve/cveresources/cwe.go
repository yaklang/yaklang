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
