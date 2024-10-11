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

	if len(filter.GetTaskIDs()) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "task_id", filter.GetTaskIDs())
	}

	if len(filter.GetResultIDs()) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "id", filter.GetResultIDs())
	}

	if len(filter.GetRuleNames()) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_name", filter.GetRuleNames())
	}

	if len(filter.GetProgramNames()) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramNames())
	}

	if filter.GetAfterID() > 0 {
		db = db.Where("id > ?", filter.GetAfterID())
	}
	if filter.GetBeforeID() > 0 {
		db = db.Where("id < ?", filter.GetBeforeID())
	}

	if filter.GetOnlyRisk() {
		db = db.Where("risk_count > 0")
	}

	if len(filter.GetSeverity()) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_severity", filter.GetSeverity())
	}

	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchEx(db, []string{
			"rule_name", "program_name", "rule_title",
		}, filter.GetKeyword(), false)
	}

	return db
}
