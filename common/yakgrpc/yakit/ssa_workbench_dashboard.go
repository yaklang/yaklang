package yakit

import (
	"math"
	"sort"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	defaultRecentProjectLimit = 5
	defaultTopRuleHitLimit    = 5
)

var ssaSeverityRank = map[string]int{
	"critical": 5,
	"high":     4,
	"middle":   3,
	"low":      2,
	"info":     1,
}

var ssaSeverityOrder = []string{"critical", "high", "middle", "low", "info"}

type ssaWorkbenchProjectRiskStat struct {
	SSAProjectID uint64
	Severity     string
	RiskCount    int64
}

func normalizeWorkbenchLimit(limit int64, fallback int64) int64 {
	if limit <= 0 {
		return fallback
	}
	return limit
}

func calcPercent(count int64, total int64) float64 {
	if total <= 0 || count <= 0 {
		return 0
	}
	return math.Round(float64(count)/float64(total)*1000) / 10
}

func CountSSAProject(db *gorm.DB, filter *ypb.SSAProjectFilter) (int64, error) {
	db = db.Model(&schema.SSAProject{})
	db = FilterSSAProject(db, filter)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CountAISessionMeta(db *gorm.DB, filter *ypb.AISessionFilter) (int64, error) {
	db = FilterAISessionMeta(db, filter)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func BuildSSAWorkbenchRiskOverview(levelCounts []*SSARiskLevelCount, severityVerbose func(string) string) ([]*ypb.SSAWorkbenchRiskLevelItem, int64) {
	countBySeverity := make(map[string]int64, len(levelCounts))
	var total int64
	for _, item := range levelCounts {
		if item == nil {
			continue
		}
		severity := string(schema.ValidSeverityType(item.Severity))
		countBySeverity[severity] += item.Count
		total += item.Count
	}

	result := make([]*ypb.SSAWorkbenchRiskLevelItem, 0, len(ssaSeverityOrder))
	for _, severity := range ssaSeverityOrder {
		count := countBySeverity[severity]
		if count == 0 {
			continue
		}
		result = append(result, &ypb.SSAWorkbenchRiskLevelItem{
			Severity: severity,
			Verbose:  severityVerbose(severity),
			Count:    count,
			Percent:  calcPercent(count, total),
		})
	}
	return result, total
}

func BuildSSAWorkbenchRiskDistribution(riskTypeGroups []*ypb.FieldGroup, riskTypeVerbose func(string) string) []*ypb.SSAWorkbenchRiskTypeItem {
	var total int64
	for _, group := range riskTypeGroups {
		if group == nil {
			continue
		}
		total += int64(group.Total)
	}

	items := make([]*ypb.SSAWorkbenchRiskTypeItem, 0, len(riskTypeGroups))
	for _, group := range riskTypeGroups {
		if group == nil || group.GetName() == "" {
			continue
		}
		count := int64(group.Total)
		items = append(items, &ypb.SSAWorkbenchRiskTypeItem{
			RiskType: group.GetName(),
			Verbose:  riskTypeVerbose(group.GetName()),
			Count:    count,
			Percent:  calcPercent(count, total),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].RiskType < items[j].RiskType
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func QuerySSAWorkbenchTopRuleHits(ssaDB, profileDB *gorm.DB, filter *ypb.SSARisksFilter, limit int64) ([]*ypb.SSAWorkbenchRuleHitItem, error) {
	limit = normalizeWorkbenchLimit(limit, defaultTopRuleHitLimit)
	db := ssaDB.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	db = db.Where("from_rule <> ''").
		Select("from_rule as name, COUNT(*) as total").
		Group("from_rule").
		Order("total DESC, from_rule ASC").
		Limit(int(limit))

	var groups []*ypb.FieldGroup
	if err := db.Scan(&groups).Error; err != nil {
		return nil, utils.Errorf("query top rule hits failed: %v", err)
	}

	ruleNames := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.GetName() == "" {
			continue
		}
		ruleNames = append(ruleNames, group.GetName())
	}

	titleByRule := make(map[string]string, len(ruleNames))
	if len(ruleNames) > 0 && profileDB != nil {
		var rules []*schema.SyntaxFlowRule
		if err := profileDB.Model(&schema.SyntaxFlowRule{}).
			Where("rule_name IN (?)", ruleNames).
			Find(&rules).Error; err == nil {
			for _, rule := range rules {
				if rule == nil {
					continue
				}
				title := rule.TitleZh
				if title == "" {
					title = rule.Title
				}
				if title == "" {
					title = rule.RuleName
				}
				titleByRule[rule.RuleName] = title
			}
		}
	}

	result := make([]*ypb.SSAWorkbenchRuleHitItem, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.GetName() == "" {
			continue
		}
		ruleName := group.GetName()
		title := titleByRule[ruleName]
		if title == "" {
			title = ruleName
		}
		result = append(result, &ypb.SSAWorkbenchRuleHitItem{
			RuleName:     ruleName,
			TitleVerbose: title,
			HitCount:     int64(group.Total),
		})
	}
	return result, nil
}

func querySSAWorkbenchProjectRiskStats(db *gorm.DB, projectIDs []uint64) (map[uint64]*ssaWorkbenchProjectRiskStat, error) {
	result := make(map[uint64]*ssaWorkbenchProjectRiskStat, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}

	var rows []struct {
		SSAProjectID uint64
		Severity     string
		RiskCount    int64
	}
	if err := db.Model(&schema.SSARisk{}).
		Where("ssa_project_id IN (?)", projectIDs).
		Select("ssa_project_id, severity, COUNT(*) as risk_count").
		Group("ssa_project_id, severity").
		Scan(&rows).Error; err != nil {
		return nil, utils.Errorf("query project risk stats failed: %v", err)
	}

	for _, row := range rows {
		severity := string(schema.ValidSeverityType(row.Severity))
		current := result[row.SSAProjectID]
		if current == nil {
			current = &ssaWorkbenchProjectRiskStat{
				SSAProjectID: row.SSAProjectID,
				Severity:     severity,
				RiskCount:    row.RiskCount,
			}
			result[row.SSAProjectID] = current
			continue
		}
		current.RiskCount += row.RiskCount
		if ssaSeverityRank[severity] > ssaSeverityRank[current.Severity] {
			current.Severity = severity
		}
	}
	return result, nil
}

func BuildSSAWorkbenchRecentProjects(
	projects []*schema.SSAProject,
	riskStats map[uint64]*ssaWorkbenchProjectRiskStat,
	severityVerbose func(string) string,
) []*ypb.SSAWorkbenchRecentProject {
	result := make([]*ypb.SSAWorkbenchRecentProject, 0, len(projects))
	for _, project := range projects {
		if project == nil {
			continue
		}
		item := &ypb.SSAWorkbenchRecentProject{
			ID:          int64(project.ID),
			ProjectName: project.ProjectName,
			Language:    string(project.Language),
			UpdatedAt:   project.UpdatedAt.Unix(),
		}
		// 填充 JSONStringConfig，供前端编译按钮使用
		if config, err := project.GetConfig(); err == nil {
			if jsonStr, err := config.ToJSONString(); err == nil {
				item.JSONStringConfig = jsonStr
			}
		}
		if stat := riskStats[uint64(project.ID)]; stat != nil {
			item.RiskCount = stat.RiskCount
			item.HighestRiskSeverity = stat.Severity
			item.HighestRiskVerbose = severityVerbose(stat.Severity)
		}
		result = append(result, item)
	}
	return result
}

func QuerySSAWorkbenchRecentProjects(
	profileDB *gorm.DB,
	ssaDB *gorm.DB,
	filter *ypb.SSAProjectFilter,
	limit int64,
	severityVerbose func(string) string,
) ([]*ypb.SSAWorkbenchRecentProject, error) {
	limit = normalizeWorkbenchLimit(limit, defaultRecentProjectLimit)
	_, projects, err := QuerySSAProject(profileDB, &ypb.QuerySSAProjectRequest{
		Filter: filter,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   limit,
			OrderBy: "updated_at",
			Order:   "desc",
		},
	})
	if err != nil {
		return nil, err
	}

	projectIDs := make([]uint64, 0, len(projects))
	for _, project := range projects {
		if project == nil || project.ID == 0 {
			continue
		}
		projectIDs = append(projectIDs, uint64(project.ID))
	}
	riskStats, err := querySSAWorkbenchProjectRiskStats(ssaDB, projectIDs)
	if err != nil {
		return nil, err
	}
	return BuildSSAWorkbenchRecentProjects(projects, riskStats, severityVerbose), nil
}
