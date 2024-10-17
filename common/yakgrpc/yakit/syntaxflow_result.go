package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func FilterSyntaxFlowResult(rawDB *gorm.DB, filter *ypb.SyntaxFlowResultFilter) *gorm.DB {
	db := rawDB
	if filter == nil {
		return db
	}

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
