package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type SSADiffResultKind string

const (
	Unknown   SSADiffResultKind = "unknown"
	RuntimeId SSADiffResultKind = "runtimeId" //task ID
	Program   SSADiffResultKind = "program"   //利用Program来进行对比
)

type SSADiffCompareType string

const (
	CustomDiff SSADiffCompareType = "custom"
	RiskDiff                      = "risk"
)

type SSADiffResult struct {
	*gorm.Model
	BaseLine string
	Compare  string
	RuleName string // rule name

	BaseLineRiskHash string
	CompareRiskHash  string

	Status string
	//CompareType 比较类型，是custom还是risk
	CompareType    string
	DiffResultKind string //结果类型，是taskID还是program

	Hash string `gorm:"uniqueIndex"`
}

func ValidSSADiffResultCompareType(typ string) SSADiffCompareType {
	switch typ {
	case "custom", "customType":
		return CustomDiff
	case "risk", "riskType":
		return RiskDiff
	default:
		return RiskDiff
	}
}

func ValidSSADiffResultKind(typ string) SSADiffResultKind {
	switch typ {
	case "runtimeId", "taskId", "taskID", "runtime":
		return RuntimeId
	case "prog", "program", "ssaProgram":
		return Program
	default:
		return Unknown
	}
}

func (d *SSADiffResult) CalcHash() string {
	return utils.CalcSha1(d.BaseLine, d.Compare)
}

func (d *SSADiffResult) BeforeCreate() {
	d.Hash = d.CalcHash()
	d.CompareType = string(ValidSSADiffResultCompareType(d.CompareType))
	d.DiffResultKind = string(ValidSSADiffResultKind(d.DiffResultKind))
}

func (d *SSADiffResult) BeforeUpdate() {
	d.Hash = d.CalcHash()
	d.CompareType = string(ValidSSADiffResultCompareType(d.CompareType))
	d.DiffResultKind = string(ValidSSADiffResultKind(d.DiffResultKind))
}

func (d *SSADiffResult) BeforeSave() {
	d.Hash = d.CalcHash()
	d.CompareType = string(ValidSSADiffResultCompareType(d.CompareType))
	d.DiffResultKind = string(ValidSSADiffResultKind(d.DiffResultKind))
}

// TableName ensures GORM uses the correct table name
func (SSADiffResult) TableName() string {
	return "ssa_diff_results"
}
