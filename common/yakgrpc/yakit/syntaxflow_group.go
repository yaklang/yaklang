package yakit

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func QuerySyntaxFlowRuleGroupByField(db *gorm.DB, field string) (results []*ypb.SyntaxFlowRuleGroupNormalized, err error) {
	db = db.Model(&schema.SyntaxFlowRule{}).Select(fmt.Sprintf("%s as group_name, is_build_in_rule, COUNT(*) as count", field))
	db = db.Group(fmt.Sprintf("%s,is_build_in_rule", field)).Order(`count desc`).Scan(&results)
	if db.Error != nil {
		return nil, utils.Wrap(db.Error, "QuerySyntaxFlowRuleGroupByField failed")
	}
	return results, nil
}
