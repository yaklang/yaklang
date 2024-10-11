package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

type AuditResult struct {
	gorm.Model

	TaskID string `json:"task_id" gorm:"index"`
	// rule
	RuleName     string `json:"rule_name"`
	RuleTitle    string `json:"rule_title"`
	RuleTitleZh  string `json:"rule_title_zh"`
	RulePurpose  string `json:"purpose"`
	RuleSeverity string `json:"rule_severity"`
	RuleDesc     string `json:"rule_desc"`

	AlertDesc schema.MapEx[string, *schema.ExtraDescInfo] `gorm:"type:text"`

	// Program
	ProgramName string `json:"program_name"`
	Language    string `json:"language"`

	RiskCount       uint64      `json:"risk_count"`
	CheckMsg        StringSlice `json:"check_msg" gorm:"type:text"`
	Errors          StringSlice `json:"errors" gorm:"type:text"`
	UnValueVariable StringSlice `json:"un_value_variable" gorm:"type:text"`
}

func GetResultByID(resultID uint) (*AuditResult, error) {
	var result AuditResult
	if err := GetDB().Where("id = ?", resultID).First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

func DeleteResultByID(resultID uint) error {
	return GetDB().Where("id = ?", resultID).Unscoped().Delete(&AuditResult{}).Error
}

func CreateResult(TaskIDs ...string) *AuditResult {
	var taskID string
	if len(TaskIDs) > 0 {
		taskID = TaskIDs[0]
	}
	ret := &AuditResult{
		TaskID: taskID,
	}
	GetDB().Create(ret)
	return ret
}

func SaveResult(result *AuditResult) error {
	return GetDB().Save(result).Error
}
