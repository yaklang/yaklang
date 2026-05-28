package yakit

import (
	"context"
	"sort"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SSAReadTargetKind labels which SSA IR database slice a read query uses.
type SSAReadTargetKind string

const (
	SSAReadTargetDedicated      SSAReadTargetKind = "dedicated"
	SSAReadTargetDefaultMigrated SSAReadTargetKind = "default_migrated"
	SSAReadTargetDefaultLegacy   SSAReadTargetKind = "default_legacy"
)

// SSAReadTarget is one SSA IR database plus how programs/risks should be scoped on it.
type SSAReadTarget struct {
	DB          *gorm.DB
	Kind        SSAReadTargetKind
	ProjectID   uint64
	Project     *schema.SSAProject
	LegacyMatch bool
}

// ProjectUsesDedicatedSSADB reports whether the project has its own SSA IR sqlite file.
func ProjectUsesDedicatedSSADB(p *schema.SSAProject) bool {
	if p == nil {
		return false
	}
	path := ResolveSSAProjectDatabasePath(p)
	return !IsDefaultSSADatabasePath(path)
}

// ShouldMergeDefaultAndDedicatedForProject is true when reads must union dedicated and default DB data.
func ShouldMergeDefaultAndDedicatedForProject(projectID uint64) (bool, error) {
	if projectID == 0 {
		return false, nil
	}
	project, err := GetSSAProjectById(projectID)
	if err != nil {
		return false, err
	}
	return ProjectUsesDedicatedSSADB(project), nil
}

func defaultMigratedReadTarget(projectID uint64, project *schema.SSAProject) (SSAReadTarget, error) {
	db, err := consts.GetOrOpenSSADB(ResolveDefaultSSADatabasePath())
	if err != nil {
		return SSAReadTarget{}, err
	}
	return SSAReadTarget{
		DB:        db,
		Kind:      SSAReadTargetDefaultMigrated,
		ProjectID: projectID,
		Project:   project,
	}, nil
}

func ensureSSADBForProjectRead(projectID uint64) error {
	if projectID > 0 {
		return EnsureSSAProjectDatabaseOpen(projectID)
	}
	return EnsureSSAProjectDatabaseReady()
}

func ssaRiskIdentityKey(r *schema.SSARisk) string {
	if r == nil {
		return ""
	}
	if r.Hash != "" {
		return r.Hash
	}
	return r.CalcHash()
}

// ResolveSSAReadTargets returns SSA IR databases to query for projectID (read path).
func ResolveSSAReadTargets(projectID uint64) ([]SSAReadTarget, error) {
	if projectID == 0 {
		tg, err := defaultMigratedReadTarget(0, nil)
		if err != nil {
			return nil, err
		}
		return []SSAReadTarget{tg}, nil
	}

	project, err := GetSSAProjectById(projectID)
	if err != nil {
		return nil, err
	}

	if !ProjectUsesDedicatedSSADB(project) {
		tg, err := defaultMigratedReadTarget(projectID, project)
		if err != nil {
			return nil, err
		}
		return []SSAReadTarget{tg}, nil
	}

	dedicatedPath := ResolveSSAProjectDatabasePath(project)
	dedicatedDB, err := consts.GetOrOpenSSADB(dedicatedPath)
	if err != nil {
		return nil, utils.Errorf("open dedicated SSA database failed: %s", err)
	}
	defaultDB, err := consts.GetOrOpenSSADB(ResolveDefaultSSADatabasePath())
	if err != nil {
		return nil, utils.Errorf("open default SSA database failed: %s", err)
	}

	return []SSAReadTarget{
		{
			DB:        dedicatedDB,
			Kind:      SSAReadTargetDedicated,
			ProjectID: projectID,
			Project:   project,
		},
		{
			DB:        defaultDB,
			Kind:      SSAReadTargetDefaultMigrated,
			ProjectID: projectID,
			Project:   project,
		},
		{
			DB:          defaultDB,
			Kind:        SSAReadTargetDefaultLegacy,
			ProjectID:   projectID,
			Project:     project,
			LegacyMatch: true,
		},
	}, nil
}

// ApplyLegacySSAProjectProgramFilter scopes programs in the default DB to legacy rows for project.
func ApplyLegacySSAProjectProgramFilter(db *gorm.DB, project *schema.SSAProject) *gorm.DB {
	if project == nil {
		return db.Model(&ssadb.IrProgram{}).Where("project_id = 0 OR project_id IS NULL")
	}
	db = db.Model(&ssadb.IrProgram{}).Where("project_id = 0 OR project_id IS NULL")

	var parts []string
	var args []interface{}
	if name := strings.TrimSpace(project.ProjectName); name != "" {
		parts = append(parts, "program_name = ?")
		args = append(args, name)
	}
	if url := strings.TrimSpace(project.URL); url != "" {
		parts = append(parts, "config_input LIKE ?")
		args = append(args, "%"+url+"%")
	}
	if config, err := project.GetConfig(); err == nil && config != nil {
		if codeURL := strings.TrimSpace(config.GetCodeSourceLocalFileOrURL()); codeURL != "" && codeURL != project.URL {
			parts = append(parts, "config_input LIKE ?")
			args = append(args, "%"+codeURL+"%")
		}
	}
	if len(parts) == 0 {
		return db.Where("1 = 0")
	}
	return db.Where("("+strings.Join(parts, " OR ")+")", args...)
}

// MergeProgramFilterWithTarget combines a user filter with a read target scope.
func MergeProgramFilterWithTarget(base *ypb.SSAProgramFilter, target SSAReadTarget) *ypb.SSAProgramFilter {
	out := &ypb.SSAProgramFilter{}
	if base != nil {
		*out = *base
	}
	if target.LegacyMatch {
		out.ProjectIds = nil
		return out
	}
	if target.ProjectID > 0 {
		out.ProjectIds = []uint64{target.ProjectID}
	}
	return out
}

// ApplySSAProgramFilterOnDB applies program list filters for a read target.
func ApplySSAProgramFilterOnDB(db *gorm.DB, filter *ypb.SSAProgramFilter, target SSAReadTarget, paging *ypb.Paging) *gorm.DB {
	if target.LegacyMatch && target.Project != nil {
		db = ApplyLegacySSAProjectProgramFilter(db, target.Project)
		if filter != nil {
			if len(filter.GetProgramNames()) > 0 {
				db = db.Where("program_name IN (?)", filter.GetProgramNames())
			}
			if word := filter.GetKeyword(); word != "" {
				db = bizhelper.FuzzSearchEx(db, []string{"program_name", "description"}, word, false)
			}
		}
		if paging == nil {
			paging = defaultSSAProgramPaging()
		}
		return bizhelper.QueryOrder(db, paging.OrderBy, paging.Order)
	}
	merged := MergeProgramFilterWithTarget(filter, target)
	return applySSAProgramListQuery(db, merged, paging)
}

// ResolveSSAReadProjectIDForRiskFilter picks the SSA project context for multi-database risk reads.
func ResolveSSAReadProjectIDForRiskFilter(filter *ypb.SSARisksFilter) uint64 {
	if filter != nil && len(filter.GetProgramName()) > 0 {
		ids := lookupProjectIDsByProgramNames(consts.GetGormProfileDatabase(), filter.GetProgramName())
		if len(ids) == 1 {
			return ids[0]
		}
	}
	return GetCurrentSSAProjectID()
}

// FilterSSARiskForReadTarget scopes risks to programs visible for the read target.
func FilterSSARiskForReadTarget(db *gorm.DB, filter *ypb.SSARisksFilter, target SSAReadTarget) *gorm.DB {
	db = FilterSSARisk(db, filter)
	if target.Project == nil && target.ProjectID == 0 && !target.LegacyMatch {
		return db
	}
	sub := ApplySSAProgramFilterOnDB(db.Model(&ssadb.IrProgram{}), nil, target, defaultSSAProgramPaging()).Select("program_name")
	return db.Where("program_name IN (?)", sub.SubQuery())
}

// QuerySSARiskForProjectRead queries risks across dedicated + default SSA IR databases for a project.
func QuerySSARiskForProjectRead(projectID uint64, filter *ypb.SSARisksFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	return querySSARiskForProjectRead(projectID, filter, paging, 0)
}

// QuerySSARiskForProjectReadAfterID is like QuerySSARiskForProjectRead but only includes rows with id > afterID.
func QuerySSARiskForProjectReadAfterID(projectID uint64, filter *ypb.SSARisksFilter, paging *ypb.Paging, afterID int64) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	return querySSARiskForProjectRead(projectID, filter, paging, afterID)
}

func querySSARiskForProjectRead(projectID uint64, filter *ypb.SSARisksFilter, paging *ypb.Paging, afterID int64) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	if filter == nil {
		return nil, nil, utils.Errorf("empty filter")
	}
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return nil, nil, err
	}
	if !merge {
		if err := ensureSSADBForProjectRead(projectID); err != nil {
			return nil, nil, err
		}
		db := consts.GetGormSSAProjectDataBase()
		if afterID > 0 {
			db = db.Where("id > ?", afterID)
		}
		return QuerySSARisk(db, filter, paging)
	}

	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return nil, nil, err
	}
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 30, OrderBy: "updated_at", Order: "desc"}
	}
	page := int(paging.Page)
	limit := int(paging.Limit)
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 30
	}
	perDBLimit := page * limit

	byHash := make(map[string]*schema.SSARisk)
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
		if afterID > 0 {
			scoped = scoped.Where("id > ?", afterID)
		}
		var batch []*schema.SSARisk
		q := bizhelper.QueryOrder(scoped.Model(&schema.SSARisk{}), paging.OrderBy, paging.Order)
		if perDBLimit > 0 {
			q = q.Limit(perDBLimit)
		}
		if err := q.Find(&batch).Error; err != nil {
			return nil, nil, utils.Errorf("query ssa risk failed: %s", err)
		}
		for _, r := range batch {
			key := ssaRiskIdentityKey(r)
			if key == "" {
				continue
			}
			if prev, ok := byHash[key]; !ok || r.UpdatedAt.After(prev.UpdatedAt) {
				byHash[key] = r
			}
		}
	}

	merged := make([]*schema.SSARisk, 0, len(byHash))
	for _, r := range byHash {
		merged = append(merged, r)
	}
	sortSSARisksByPaging(merged, paging)
	p, data, err := paginateSSARisks(merged, paging)
	if err != nil {
		return nil, nil, err
	}
	if afterID > 0 {
		total, err := querySSARiskCountForProjectRead(projectID, filter, afterID)
		if err != nil {
			return nil, nil, err
		}
		p.TotalRecord = total
	}
	return p, data, nil
}

func sortSSARisksByPaging(risks []*schema.SSARisk, p *ypb.Paging) {
	orderBy := "updated_at"
	order := "desc"
	if p != nil {
		if p.OrderBy != "" {
			orderBy = p.OrderBy
		}
		if p.Order != "" {
			order = p.Order
		}
	}
	sort.Slice(risks, func(i, j int) bool {
		switch orderBy {
		case "created_at":
			if order == "asc" {
				return risks[i].CreatedAt.Before(risks[j].CreatedAt)
			}
			return risks[i].CreatedAt.After(risks[j].CreatedAt)
		case "id":
			if order == "asc" {
				return risks[i].ID < risks[j].ID
			}
			return risks[i].ID > risks[j].ID
		default:
			if order == "asc" {
				return risks[i].UpdatedAt.Before(risks[j].UpdatedAt)
			}
			return risks[i].UpdatedAt.After(risks[j].UpdatedAt)
		}
	})
}

func paginateSSARisks(risks []*schema.SSARisk, p *ypb.Paging) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	if p == nil {
		p = &ypb.Paging{Page: 1, Limit: 30}
	}
	page := int(p.Page)
	limit := int(p.Limit)
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 30
	}
	total := len(risks)
	paginator := &bizhelper.Paginator{TotalRecord: total, Page: page, Limit: limit}
	if total == 0 {
		return paginator, nil, nil
	}
	paginator.TotalPage = (total + limit - 1) / limit
	start := (page - 1) * limit
	if start >= total {
		return paginator, nil, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	paginator.Offset = start
	return paginator, risks[start:end], nil
}

// QuerySSARiskCountForProjectRead counts risks across all read targets for a project.
func QuerySSARiskCountForProjectRead(projectID uint64, filter *ypb.SSARisksFilter) (int, error) {
	return querySSARiskCountForProjectRead(projectID, filter, 0)
}

func querySSARiskCountForProjectRead(projectID uint64, filter *ypb.SSARisksFilter, afterID int64) (int, error) {
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return 0, err
	}
	if !merge {
		if err := ensureSSADBForProjectRead(projectID); err != nil {
			return 0, err
		}
		db := consts.GetGormSSAProjectDataBase()
		if afterID > 0 {
			db = db.Where("id > ?", afterID)
		}
		return QuerySSARiskCount(db, filter)
	}

	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]struct{})
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
		if afterID > 0 {
			scoped = scoped.Where("id > ?", afterID)
		}
		var risks []*schema.SSARisk
		if err := scoped.Model(&schema.SSARisk{}).Select("hash").Find(&risks).Error; err != nil {
			return 0, err
		}
		for _, r := range risks {
			if h := ssaRiskIdentityKey(r); h != "" {
				seen[h] = struct{}{}
			}
		}
	}
	return len(seen), nil
}

// AggregateSSAProjectCompileTimes counts distinct program names across read targets.
func AggregateSSAProjectCompileTimes(projectID uint) int64 {
	targets, err := ResolveSSAReadTargets(uint64(projectID))
	if err != nil {
		return 0
	}
	names := make(map[string]struct{})
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		var list []string
		db := ApplySSAProgramFilterOnDB(target.DB, nil, target, defaultSSAProgramPaging())
		if err := db.Pluck("program_name", &list).Error; err != nil {
			continue
		}
		for _, n := range list {
			if n != "" {
				names[n] = struct{}{}
			}
		}
	}
	return int64(len(names))
}

// AggregateSSAProjectRiskNumber counts distinct risk hashes across read targets.
func AggregateSSAProjectRiskNumber(projectID uint) int64 {
	n, err := QuerySSARiskCountForProjectRead(uint64(projectID), &ypb.SSARisksFilter{})
	if err != nil {
		return 0
	}
	return int64(n)
}

// GetSSARiskDBForRead returns a database handle for read operations (active write DB after ensure).
func GetSSARiskDBForRead(projectID uint64) (*gorm.DB, error) {
	if err := ensureSSADBForProjectRead(projectID); err != nil {
		return nil, err
	}
	return consts.GetGormSSAProjectDataBase(), nil
}

// SSARiskColumnGroupCountAcrossProject merges field group counts from all read targets.
func SSARiskColumnGroupCountAcrossProject(projectID uint64, filter *ypb.SSARisksFilter, column string) []*ypb.FieldGroup {
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil || !merge {
		db, derr := GetSSARiskDBForRead(projectID)
		if derr != nil {
			return nil
		}
		if filter != nil {
			return SSARiskColumnGroupCount(FilterSSARisk(db, filter), column)
		}
		return SSARiskColumnGroupCount(db, column)
	}

	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return nil
	}
	merged := make(map[string]int32)
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
		for _, g := range SSARiskColumnGroupCount(scoped, column) {
			if g == nil {
				continue
			}
			merged[g.Name] += g.Total
		}
	}
	out := make([]*ypb.FieldGroup, 0, len(merged))
	for name, total := range merged {
		out = append(out, &ypb.FieldGroup{Name: name, Total: total})
	}
	return out
}

// YieldSSARiskAcrossProject yields risks from all read databases for a project.
func YieldSSARiskAcrossProject(projectID uint64, filter *ypb.SSARisksFilter, ctx context.Context) chan *schema.SSARisk {
	out := make(chan *schema.SSARisk)
	go func() {
		defer close(out)
		merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
		if err != nil {
			return
		}
		if !merge {
			db, derr := GetSSARiskDBForRead(projectID)
			if derr != nil {
				return
			}
			for r := range YieldSSARisk(FilterSSARisk(db, filter), ctx) {
				out <- r
			}
			return
		}
		targets, err := ResolveSSAReadTargets(projectID)
		if err != nil {
			return
		}
		seen := make(map[string]struct{})
		for _, target := range targets {
			if target.DB == nil {
				continue
			}
			scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
			for r := range YieldSSARisk(scoped, ctx) {
				h := ssaRiskIdentityKey(r)
				if h == "" {
					continue
				}
				if _, ok := seen[h]; ok {
					continue
				}
				seen[h] = struct{}{}
				select {
				case <-ctx.Done():
					return
				case out <- r:
				}
			}
		}
	}()
	return out
}

// QueryAllSSARisksForProjectRead returns all risks matching filter across read targets (no pagination cap in merge path).
func QueryAllSSARisksForProjectRead(projectID uint64, filter *ypb.SSARisksFilter) ([]*schema.SSARisk, error) {
	if filter == nil {
		return nil, utils.Errorf("empty filter")
	}
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return nil, err
	}
	if !merge {
		if err := ensureSSADBForProjectRead(projectID); err != nil {
			return nil, err
		}
		db := FilterSSARisk(consts.GetGormSSAProjectDataBase(), filter)
		var risks []*schema.SSARisk
		if err := db.Model(&schema.SSARisk{}).Order("created_at DESC, id ASC").Find(&risks).Error; err != nil {
			return nil, err
		}
		return risks, nil
	}

	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return nil, err
	}
	byHash := make(map[string]*schema.SSARisk)
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
		var batch []*schema.SSARisk
		if err := scoped.Model(&schema.SSARisk{}).Order("created_at DESC, id ASC").Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, r := range batch {
			key := ssaRiskIdentityKey(r)
			if key == "" {
				continue
			}
			if prev, ok := byHash[key]; !ok || r.UpdatedAt.After(prev.UpdatedAt) {
				byHash[key] = r
			}
		}
	}
	out := make([]*schema.SSARisk, 0, len(byHash))
	for _, r := range byHash {
		out = append(out, r)
	}
	sortSSARisksByPaging(out, &ypb.Paging{OrderBy: "created_at", Order: "desc"})
	return out, nil
}

// GetSSARiskByHashForProjectRead looks up a risk hash across all read targets for a project.
func GetSSARiskByHashForProjectRead(projectID uint64, hash string) (*schema.SSARisk, error) {
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return nil, err
	}
	if !merge {
		db, derr := GetSSARiskDBForRead(projectID)
		if derr != nil {
			return nil, derr
		}
		return GetSSARiskByHash(db, hash)
	}
	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		if r, err := GetSSARiskByHash(target.DB, hash); err == nil {
			return r, nil
		}
	}
	return nil, utils.Errorf("get SSA risk by hash failed: not found")
}

// DeleteSSARisksForProjectWrite deletes risks on the active project database (write path).
func DeleteSSARisksForProjectWrite(projectID uint64, filter *ypb.SSARisksFilter) error {
	if err := ensureSSADBForProjectRead(projectID); err != nil {
		return err
	}
	return DeleteSSARisks(consts.GetGormSSAProjectDataBase(), filter)
}

// NewSSARiskReadRequestForProject marks risks read on all read targets when merging.
func NewSSARiskReadRequestForProject(projectID uint64, filter *ypb.SSARisksFilter) error {
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return err
	}
	if !merge {
		db, derr := GetSSARiskDBForRead(projectID)
		if derr != nil {
			return derr
		}
		return NewSSARiskReadRequest(db, filter)
	}
	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		if err := NewSSARiskReadRequest(FilterSSARiskForReadTarget(target.DB, filter, target), filter); err != nil {
			return err
		}
	}
	return nil
}

// GetSSARiskLevelCountForProjectRead merges severity counts across read targets.
func GetSSARiskLevelCountForProjectRead(projectID uint64, filter *ypb.SSARisksFilter) ([]*SSARiskLevelCount, error) {
	merge, err := ShouldMergeDefaultAndDedicatedForProject(projectID)
	if err != nil {
		return nil, err
	}
	if !merge {
		db, derr := GetSSARiskDBForRead(projectID)
		if derr != nil {
			return nil, derr
		}
		return GetSSARiskLevelCount(db, filter)
	}
	targets, err := ResolveSSAReadTargets(projectID)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]int64)
	for _, target := range targets {
		if target.DB == nil {
			continue
		}
		scoped := FilterSSARiskForReadTarget(target.DB, filter, target)
		rows, err := GetSSARiskLevelCount(scoped, filter)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			merged[row.Severity] += row.Count
		}
	}
	out := make([]*SSARiskLevelCount, 0, len(merged))
	for sev, count := range merged {
		out = append(out, &SSARiskLevelCount{Severity: sev, Count: count})
	}
	return out, nil
}
