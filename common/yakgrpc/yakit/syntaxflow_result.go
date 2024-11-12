package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func FilterSyntaxFlowResult(rawDB *gorm.DB, filter *ypb.SyntaxFlowResultFilter) *gorm.DB {
	db := rawDB
	if filter == nil {
		return db
	}

	/*
		syntaxflow-result create and update,
		when program_name is empty, it means the result just create , not update.
	*/
	db = db.Where("program_name != ?", "")
	db = bizhelper.ExactOrQueryStringArrayOr(db, "task_id", filter.GetTaskIDs())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "id", filter.GetResultIDs())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_name", filter.GetRuleNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_severity", filter.GetSeverity())

	if filter.GetAfterID() > 0 {
		db = db.Where("id > ?", filter.GetAfterID())
	}
	if filter.GetBeforeID() > 0 {
		db = db.Where("id < ?", filter.GetBeforeID())
	}

	if filter.GetOnlyRisk() {
		db = db.Where("risk_count > 0")
	}

	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchEx(db, []string{
			"rule_name", "program_name", "rule_title",
		}, filter.GetKeyword(), false)
	}

	return db
}
func GetSyntaxFlowResultRiskInfo(db *gorm.DB, programs []string, number int, severity []string) []*ssadb.AuditResult {
	var result []*ssadb.AuditResult
	db = db.Where("program_name != ?", "")
	if number != 0 {
		db.Where("risk_count>?", number)
	}
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", programs)
	db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_severity", severity)
	resultx := db.Find(&result)
	if resultx.Error != nil {
		log.Errorf("get syntax flow result risk info fail: %s", resultx.Error)
	}
	return result
}

func GetSyntaxFlowResultByTaskId(db *gorm.DB, taskId string) *gorm.DB {
	filter := &ypb.SyntaxFlowResultFilter{
		TaskIDs: []string{taskId},
	}
	return FilterSyntaxFlowResult(db, filter)
}
