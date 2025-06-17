package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type SSADiffResult struct {
	gorm.Model
	BaseProgram     string // base/last program name
	CompareProgram  string // compare program name
	ResultHash      string `gorm:"index"` // hash(baseProg+CompareProg)
	RuleName        string // rule name
	BaseRiskHash    string
	CompareRiskHash string
	Status          int
	CompareType     int
}

func (d *SSADiffResult) CalcHash() string {
	return utils.CalcMd5(d.BaseProgram, d.CompareProgram)
}
func (d *SSADiffResult) BeforeCreate(tx *gorm.DB) (err error) {
	d.ResultHash = d.CalcHash()
	return nil
}
