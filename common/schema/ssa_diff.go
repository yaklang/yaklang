package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type SSADiffResultKind int64

const (
	Unknown   SSADiffResultKind = iota
	RuntimeId                   //task ID
	Prog                        //利用Program来进行对比
)

type compareType int64

const (
	CustomDiff compareType = iota + 1
	RiskDiff
)

type SSADiffResult struct {
	*gorm.Model
	BaseItem        string
	CompareItem     string
	ResultHash      string `gorm:"index"` // hash(BaseItem + CompareItem)
	RuleName        string // rule name
	BaseRiskHash    string
	CompareRiskHash string
	Status          int
	//CompareType 比较类型，是custom还是risk
	CompareType    int
	DiffResultKind SSADiffResultKind //结果类型，是taskID还是program
}

func (d *SSADiffResult) CalcHash() string {
	return utils.CalcMd5(d.BaseItem, d.CompareItem)
}
func (d *SSADiffResult) BeforeCreate(tx *gorm.DB) (err error) {
	d.ResultHash = d.CalcHash()
	return nil
}
