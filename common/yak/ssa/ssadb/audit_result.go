package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ServerPushType_SyntaxflowResult = "syntaxflow_result"
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
	RuleContent  string `json:"rule_content" gorm:"type:text;index"`

	AlertDesc schema.MapEx[string, *schema.SyntaxFlowDescInfo] `gorm:"type:text"`

	// Program
	ProgramName string `json:"program_name" gorm:"index"`
	Language    string `json:"language"`

	Kind schema.SyntaxflowResultKind `json:"kind" gorm:"index"` // debug / scan / query / search

	RiskCount uint64                       `json:"risk_count"`
	RiskHashs schema.MapEx[string, string] `json:"risk_hashs" gorm:"type:text"`

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

func GetResultByRuleContent(programName, rule string, kind schema.SyntaxflowResultKind) *AuditResult {
	var result AuditResult
	if err := GetDB().Where("program_name = ? AND rule_content = ? AND kind = ?", programName, rule, kind).First(&result).Error; err != nil {
		return nil
	}
	return &result
}

func DeleteResultByTaskID(taskId string) (int64, error) {
	db := GetDB()
	db = bizhelper.ExactQueryString(db, "task_id", taskId)
	return DetleteResultByDB(db)
}

func DetleteResultByDB(db *gorm.DB) (int64, error) {
	var ids []uint
	if err := db.Model(&AuditResult{}).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return DeleteResultByID(ids...)
}

func DeleteResultByID(resultID ...uint) (int64, error) {
	if len(resultID) == 0 {
		return 0, nil
	}

	db := GetDB()
	// Delete edges using result_id directly
	{
		db := bizhelper.ExactQueryUIntArrayOr(db, "result_id", resultID)
		if err := db.Unscoped().Delete(&AuditEdge{}).Error; err != nil {
			return 0, err
		}
		// Delete nodes
		if err := db.Unscoped().Delete(&AuditNode{}).Error; err != nil {
			return 0, err
		}
	}

	// Delete results
	db = db.Unscoped().Where("id IN (?)", resultID).Delete(&AuditResult{})
	return db.RowsAffected, db.Error
}

func CreateResult(TaskIDs ...string) *AuditResult {
	taskID := ""
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

func CountAuditResults(DB *gorm.DB) (int, error) {
	var count int64
	db := DB
	if err := db.Model(&AuditResult{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func YieldAuditResults(db *gorm.DB, ctx context.Context) chan *AuditResult {
	return bizhelper.YieldModel[*AuditResult](ctx, db)
}

func (r *AuditResult) ToGRPCModel() *ypb.SyntaxFlowResult {
	res := &ypb.SyntaxFlowResult{
		ResultID:    uint64(r.ID),
		TaskID:      r.TaskID,
		RuleName:    r.RuleName,
		Title:       r.RuleTitle,
		TitleZh:     r.RuleTitleZh,
		Description: r.RuleDesc,
		Severity:    r.RuleSeverity,
		Purpose:     r.RulePurpose,
		ProgramName: r.ProgramName,
		Language:    r.Language,
		RiskCount:   r.RiskCount,
		RuleContent: r.RuleContent,
		Kind:        string(r.Kind),
	}
	return res
}

func (r *AuditResult) AfterUpdate(tx *gorm.DB) (err error) {
	schema.GetBroadCast_Data().Call(ServerPushType_SyntaxflowResult, map[string]string{
		"task_id": r.TaskID,
		"action":  "update",
	})
	return nil
}

func (r *AuditResult) AfterDelete(tx *gorm.DB) (err error) {
	schema.GetBroadCast_Data().Call(ServerPushType_SyntaxflowResult, map[string]string{
		"task_id": r.TaskID,
		"action":  "delete",
	})
	return nil
}
