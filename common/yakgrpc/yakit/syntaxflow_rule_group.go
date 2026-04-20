package yakit

import (
	"math"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func resolveSyntaxFlowRuleGroupFilter(filter *ypb.SyntaxFlowRuleGroupFilter) (includeTags []string, excludeTags []string, purposes []string) {
	if filter == nil {
		return nil, nil, nil
	}
	includeTags = normalizeSyntaxFlowTags(filter.GetTag())
	excludeTags = normalizeSyntaxFlowTags(filter.GetExcludeTags())
	if filter.GetComplianceMode() == ComplianceModeExclude {
		purposes = []string{string(schema.SFR_PURPOSE_VULN)}
	}
	return includeTags, excludeTags, purposes
}

type GroupAndRuleCount struct {
	GroupName string
	Count     int64
}

// QuerySyntaxFlowRuleGroup 查询规则组中相关规则的个数
func QuerySyntaxFlowRuleGroup(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleGroupRequest) (*bizhelper.Paginator, []*schema.SyntaxFlowGroup, error) {
	if params == nil {
		return nil, nil, utils.Error("query syntax flow rule group failed: request is nil")
	}
	filter := params.GetFilter()
	p := params.Pagination
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	includeTags, excludeTags, purposes := resolveSyntaxFlowRuleGroupFilter(filter)
	if filter != nil && (len(includeTags) > 0 || len(excludeTags) > 0 || len(purposes) > 0) {
		baseDB := db.Model(&schema.SyntaxFlowGroup{})
		baseDB = bizhelper.OrderByPaging(baseDB, p)
		baseDB = FilterSyntaxFlowGroups(baseDB, filter)

		var allGroups []*schema.SyntaxFlowGroup
		if err := baseDB.Find(&allGroups).Error; err != nil {
			return nil, nil, err
		}

		filteredGroups := make([]*schema.SyntaxFlowGroup, 0, len(allGroups))
		for _, group := range allGroups {
			if group == nil {
				continue
			}
				count, err := QuerySyntaxFlowRuleCount(db, &ypb.SyntaxFlowRuleFilter{
					GroupNames:  []string{group.GroupName},
					Purpose:     purposes,
					Tag:         includeTags,
					ExcludeTags: excludeTags,
				})
				if err != nil {
					return nil, nil, err
				}
			if count <= 0 {
				continue
			}
			group.Count = count
			filteredGroups = append(filteredGroups, group)
		}

		page := int(p.GetPage())
		if page < 1 {
			page = 1
		}
		limit := int(p.GetLimit())
		if limit == 0 {
			limit = 30
		}

		totalRecord := len(filteredGroups)
		totalPage := 1
		if limit > 0 {
			totalPage = int(math.Ceil(float64(totalRecord) / float64(limit)))
			if totalPage == 0 {
				totalPage = 1
			}
		}

		start := 0
		end := totalRecord
		if limit > 0 {
			start = (page - 1) * limit
			if start > totalRecord {
				start = totalRecord
			}
			end = start + limit
			if end > totalRecord {
				end = totalRecord
			}
		}

		paging := &bizhelper.Paginator{
			TotalRecord: totalRecord,
			TotalPage:   totalPage,
			Records:     filteredGroups[start:end],
			Offset:      start,
			Limit:       limit,
			Page:        page,
			PrevPage:    maxInt(page-1, 1),
			NextPage:    minInt(page+1, totalPage),
		}
		return paging, filteredGroups[start:end], nil
	}

	db = db.Model(&schema.SyntaxFlowGroup{}).Preload("Rules")
	db = bizhelper.OrderByPaging(db, p)
	db = FilterSyntaxFlowGroups(db, filter)
	var ret []*schema.SyntaxFlowGroup
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	return paging, ret, db.Error
}

func FilterSyntaxFlowGroups(db *gorm.DB, filter *ypb.SyntaxFlowRuleGroupFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	db = bizhelper.ExactOrQueryStringArrayOr(db, "group_name", filter.GetGroupNames())
	if filter.GetKeyWord() != "" {
		db = bizhelper.FuzzQueryStringArrayOrLike(db,
			"group_name", []string{filter.GetKeyWord()})
	}
	switch filter.GetFilterGroupKind() {
	case FilterBuiltinRuleTrue:
		db = db.Where("is_build_in = ?", true)
	case FilterBuiltinRuleFalse:
		db = db.Where("is_build_in = ?", false)
	}
	return db
}

func DeleteSyntaxFlowRuleGroup(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleGroupRequest) (int64, error) {
	if params == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete syntaxflow rule request is nil")
	}
	if params.GetFilter() == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete filter is nil")
	}

	db = FilterSyntaxFlowGroups(db, params.GetFilter())
	db = db.Model(&schema.SyntaxFlowGroup{}).
		Unscoped().Delete(&schema.SyntaxFlowGroup{})
	return db.RowsAffected, db.Error
}

func QuerySyntaxFlowGroupCount(db *gorm.DB, groupNames []string) int64 {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var count int64
	db.Where("group_name IN (?)", groupNames).Count(&count)
	return count
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
