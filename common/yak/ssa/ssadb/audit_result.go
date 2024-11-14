package ssadb

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	RuleContent  string `json:"rule_content" gorm:"type:text"`

	AlertDesc schema.MapEx[string, *schema.SyntaxFlowDescInfo] `gorm:"type:text"`

	// Program
	ProgramName string `json:"program_name"`
	Language    string `json:"language"`

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

func DeleteResultByTaskID(taskId string) error {
	return GetDB().Where("task_id = ?", taskId).Unscoped().Delete(&AuditResult{}).Error
}

func DeleteResultByID(resultID uint) error {
	return GetDB().Where("id = ?", resultID).Unscoped().Delete(&AuditResult{}).Error
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

func YieldAuditResults(DB *gorm.DB, ctx context.Context) chan *AuditResult {
	db := DB.Model(&AuditResult{})

	outC := make(chan *AuditResult)

	go func() {
		paginator := bizhelper.NewFastPaginator(db, 100)
		for {
			var items []*AuditResult
			if err, ok := paginator.Next(&items); !ok {
				break
			} else if err != nil {
				log.Errorf("paging failed: %s", err)
				continue
			}

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}
		}
	}()
	return outC
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
	}
	return res
}

func (r *AuditResult) AfterUpdate(tx *gorm.DB) (err error) {
	schema.GetBroadCast_Data().Call("syntaxflow_result", map[string]string{
		"task_id": r.TaskID,
		"action":  "update",
	})
	return nil
}

func (r *AuditResult) AfterDelete(tx *gorm.DB) (err error) {
	schema.GetBroadCast_Data().Call("syntaxflow_result", map[string]string{
		"task_id": r.TaskID,
		"action":  "delete",
	})
	return nil
}
