package yakit

import (
	"context"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func DeleteSSARiskBySFResult(DB *gorm.DB, resultIDs []int64) error {
	db := DB.Model(&schema.SSARisk{})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "result_id", resultIDs)
	if db := db.Unscoped().Delete(&schema.SSARisk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func CreateSSARisk(DB *gorm.DB, r *schema.SSARisk) error {
	if r == nil {
		return utils.Errorf("save error: ssa-risk is nil")
	}
	if r.TitleVerbose == "" {
		r.TitleVerbose = r.Title
	}
	if db := DB.Create(r); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetSSARiskByID(db *gorm.DB, id int64) (*schema.SSARisk, error) {
	var r schema.SSARisk
	if db := db.Model(&schema.SSARisk{}).Where("id = ?", id).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func GetSSARiskByHash(db *gorm.DB, hash string) (*schema.SSARisk, error) {
	var r schema.SSARisk
	db = FilterSSARisk(db, &ypb.SSARisksFilter{
		Hash: []string{hash},
	})
	if db := db.First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

type SSARiskFilterOption func(*ypb.SSARisksFilter)

func WithSSARiskFilterProgramName(programName string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if programName == "" {
			return
		}
		sf.ProgramName = append(sf.ProgramName, programName)
	}
}

func WithSSARiskFilterRuleName(ruleName string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if ruleName == "" {
			return
		}
		sf.FromRule = append(sf.FromRule, ruleName)
	}
}

func WithSSARiskFilterSourceUrl(sourceUrl string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if sourceUrl == "" {
			return
		}
		sf.CodeSourceUrl = append(sf.CodeSourceUrl, sourceUrl)
	}
}

func WithSSARiskFilterFunction(functionName string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if functionName == "" {
			return
		}
		sf.FunctionName = append(sf.FunctionName, functionName)
	}
}

func WithSSARiskFilterSearch(search string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		sf.Search = search
	}
}

func WithSSARiskFilterCompare(taskID1, taskID2 string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if taskID1 == "" || taskID2 == "" {
			return
		}
		sf.SSARiskDiffRequest = &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{RiskRuntimeId: taskID1},
			Compare:  &ypb.SSARiskDiffItem{RiskRuntimeId: taskID2},
		}
	}
}

func WithSSARiskIncremental() SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		sf.Incremental = true
	}
}

func WithSSARiskFilterTaskID(taskID string) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if taskID == "" {
			return
		}
		sf.RuntimeID = append(sf.RuntimeID, taskID)
	}
}

func WithSSARiskResultID(resultID uint64) SSARiskFilterOption {
	return func(sf *ypb.SSARisksFilter) {
		if resultID == 0 {
			return
		}
		sf.ResultID = append(sf.ResultID, resultID)
	}
}

func NewSSARiskFilter(opts ...SSARiskFilterOption) *ypb.SSARisksFilter {
	filter := &ypb.SSARisksFilter{}
	for _, opt := range opts {
		opt(filter)
	}
	return filter
}

func filterSSARiskByIncremental(db *gorm.DB, runtimeId ...string) *gorm.DB {
	if len(runtimeId) == 0 {
		// 查询所有未处置的漏洞
		var disposedFeatureHashes []string
		err := db.New().Model(&schema.SSARiskDisposals{}).
			Pluck("DISTINCT risk_feature_hash", &disposedFeatureHashes).Error
		if err != nil {
			log.Errorf("Failed to query disposed feature hashes: %v", err)
			return db
		}
		db = db.Where("risk_feature_hash NOT IN (?)", disposedFeatureHashes)
		return db
	} else if len(runtimeId) == 1 {
		var baseTask schema.SyntaxFlowScanTask
		if err := db.New().Where("task_id = ?", runtimeId).First(&baseTask).Error; err != nil {
			return db
		}
		var validFeatureHashes []string
		subWhere := `
		NOT EXISTS (
			SELECT 1
			FROM ssa_risk_disposals
			JOIN syntax_flow_scan_tasks base_task ON ssa_risk_disposals.task_id = base_task.task_id
			WHERE ssa_risk_disposals.risk_feature_hash = ssa_risks.risk_feature_hash
			  AND base_task.scan_batch <= ?
		)
	`
		err := db.New().
			Model(&schema.SSARisk{}).
			Where("ssa_risks.runtime_id = ? AND ssa_risks.risk_feature_hash != ''", runtimeId).
			Where(subWhere, baseTask.ScanBatch).
			Pluck("DISTINCT ssa_risks.risk_feature_hash", &validFeatureHashes).Error
		if err != nil {
			log.Errorf("Failed to query valid feature hashes: %v", err)
			return db
		}
		return db.Where("risk_feature_hash IN (?)", validFeatureHashes)
	}
	return db
}

func filterSSARiskByCompare(db *gorm.DB, baselineTaskID, compareTaskID string) *gorm.DB {
	if baselineTaskID == "" || compareTaskID == "" {
		return db
	}
	var compareHash []string
	db.New().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", compareTaskID).
		Where("risk_feature_hash != ''").
		Pluck("risk_feature_hash", &compareHash)
	if len(compareHash) > 0 {
		db = db.Where("risk_feature_hash NOT IN (?)", compareHash)
	}
	return db
}
func FilterSSARisk(db *gorm.DB, filter *ypb.SSARisksFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetID())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramName())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "function_name", filter.GetFunctionName())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "code_source_url", filter.GetCodeSourceUrl())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "risk_type", filter.GetRiskType())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "severity", filter.GetSeverity())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "from_rule", filter.GetFromRule())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "runtime_id", filter.GetRuntimeID())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "hash", filter.GetHash())
	db = bizhelper.ExactQueryUint64ArrayOr(db, "result_id", filter.GetResultID())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, filter.GetTags(), false)
	db = bizhelper.FuzzSearchEx(db, []string{"title", "title_verbose"}, filter.GetTitle(), false)
	db = bizhelper.FuzzSearchEx(db, []string{
		"id", "hash", // for exact query
		"program_name", "code_source_url", "function_name",
		"risk_type", "severity", "from_rule", "tags",
		"title", "title_verbose",
	}, filter.GetSearch(), false)
	if filter.GetIsRead() != 0 {
		db = bizhelper.QueryByBool(db, "is_read", filter.GetIsRead() > 0)
	}
	if filter.GetAfterCreatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "created_at", filter.GetAfterCreatedAt(), time.Now().Add(10*time.Minute).Unix())
	}

	if filter.GetBeforeCreatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "created_at", 0, filter.GetBeforeCreatedAt())
	}

	if disposalStatuses := filter.GetLatestDisposalStatus(); len(disposalStatuses) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "latest_disposal_status", disposalStatuses)
	}

	if dr := filter.GetSSARiskDiffRequest(); dr != nil {
		baselineTaskID := dr.GetBaseLine().GetRiskRuntimeId()
		compareTaskID := dr.GetCompare().GetRiskRuntimeId()
		db = filterSSARiskByCompare(db, baselineTaskID, compareTaskID)
	}

	if filter.GetIncremental() {
		runtimeIDs := filter.GetRuntimeID()
		db = filterSSARiskByIncremental(db, runtimeIDs...)
	}

	return db
}

func QuerySSARisk(db *gorm.DB, filter *ypb.SSARisksFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	if filter == nil {
		return nil, nil, utils.Errorf("empty filter")
	}
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	var risks []*schema.SSARisk

	db = db.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	queryPaging, queryDb := bizhelper.YakitPagingQuery(db, paging, &risks)
	if queryDb.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return queryPaging, risks, nil
}

func DeleteSSARisks(DB *gorm.DB, filter *ypb.SSARisksFilter) error {
	db := DB.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	if db := db.Unscoped().Delete(&schema.SSARisk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func UpdateSSARiskTags(DB *gorm.DB, id int64, tags []string) error {
	db := DB.Model(&schema.SSARisk{})
	if db := db.Where("id = ?", id).Update("tags", strings.Join(tags, "|")); db.Error != nil {
		return db.Error
	}
	return nil
}

func SSARiskColumnGroupCount(db *gorm.DB, column string) []*ypb.FieldGroup {
	return bizhelper.GroupCount(db, "ssa_risks", column)
}

func NewSSARiskReadRequest(db *gorm.DB, filter *ypb.SSARisksFilter) error {
	db = db.Model(&schema.SSARisk{})
	if filter != nil {
		db = FilterSSARisk(db, filter)
	}
	db = db.UpdateColumn("is_read", true)
	if db.Error != nil {
		return utils.Errorf("NewSSARiskReadRequest failed %s", db.Error)
	}
	return nil
}

func QuerySSARiskCount(DB *gorm.DB, filter *ypb.SSARisksFilter) (int, error) {
	db := DB.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	var count int
	db = db.Count(&count)
	return count, db.Error
}

func QuerySSARiskByRiskHash(db *gorm.DB, riskHash string) (*schema.SSARisk, error) {
	var r schema.SSARisk
	if db := db.Model(&schema.SSARisk{}).Where("hash = ?", riskHash).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func YieldSSARisk(db *gorm.DB, ctx context.Context) chan *schema.SSARisk {
	return bizhelper.YieldModel[*schema.SSARisk](ctx, db, bizhelper.WithYieldModel_PageSize(100))
}

type SSARiskLevelCount struct {
	Count    int64  `json:"count"`
	Severity string `json:"severity"`
}

func GetSSARiskLevelCount(DB *gorm.DB, filter *ypb.SSARisksFilter) ([]*SSARiskLevelCount, error) {
	db := DB.Model(&schema.SSARisk{})
	// db = db.Debug()

	db = FilterSSARisk(db, filter)
	db = db.Select("severity as severity, COUNT(*) as count").Group("severity")

	var v []*SSARiskLevelCount
	if err := db.Scan(&v).Error; err != nil {
		return nil, utils.Errorf("scan failed: %v", err)
	}
	return v, nil
}
