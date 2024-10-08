package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

type AuditResult struct {
	gorm.Model

	TaskID string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultID string `json:"result_id" gorm:"index"`
	// rule
	RuleName     string                                      `json:"rule_name"`
	RuleTitle    string                                      `json:"rule_title"`
	RuleSeverity string                                      `json:"rule_severity"`
	RuleType     string                                      `json:"rule_type"`
	RuleDesc     string                                      `json:"rule_desc"`
	AlertDesc    schema.MapEx[string, *schema.ExtraDescInfo] `gorm:"type:text"`

	CheckMsg StringSlice `json:"check_msg" gorm:"type:text"`
	Errors   StringSlice `json:"errors" gorm:"type:text"`

	UnValueVariable StringSlice `json:"un_value_variable" gorm:"type:text"`
}

func GetResultByID(resultID string) (*AuditResult, error) {
	var result AuditResult
	if err := GetDB().Where("result_id = ?", resultID).First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

func DeleteResultByID(resultID string) error {
	return GetDB().Where("result_id = ?", resultID).Unscoped().Delete(&AuditResult{}).Error
}
