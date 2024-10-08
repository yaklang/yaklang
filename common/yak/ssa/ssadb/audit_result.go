package ssadb

import "github.com/jinzhu/gorm"

type AuditResult struct {
	gorm.Model

	TaskID string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultID string `json:"result_id" gorm:"index"`
	// program info
	ProgramName string `json:"program_name"`
	// rule
	RuleName     string `json:"rule_name"`
	RuleTitle    string `json:"rule_title"`
	RuleSeverity string `json:"rule_severity"`
	RuleType     string `json:"rule_type"`
	RuleDesc     string `json:"rule_desc"`
	RuntimeInfo  string `json:"runtime_info"`

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
