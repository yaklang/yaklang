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

func FilterSSARisk(db *gorm.DB, filter *ypb.SSARisksFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	// 对比查询
	if dr := filter.GetSSARiskDiffRequest(); dr != nil {
		baselineTaskID := dr.GetBaseLine().GetRiskRuntimeId()
		compareTaskID := dr.GetCompare().GetRiskRuntimeId()

		if baselineTaskID != "" && compareTaskID != "" {
			var compareHash []string
			compareQuery := db.Model(&schema.SSARisk{}).
				Where("runtime_id = ?", compareTaskID).
				Where("risk_feature_hash != ''").
				Pluck("risk_feature_hash", &compareHash)

			if compareQuery.Error == nil {
				if len(compareHash) > 0 {
					db = db.Where("runtime_id = ?", baselineTaskID).
						Where("risk_feature_hash NOT IN (?)", compareHash)
				} else {
					db = db.Where("runtime_id = ?", baselineTaskID)
				}
			} else {
				log.Errorf("Failed to query baseline task hashes: %v", compareQuery.Error)
				db = db.Where("1 = 0")
				return db
			}
		} else {
			// 如果任务ID不完整，返回空结果
			db = db.Where("1 = 0")
			return db
		}
	}

	if filter.GetIncremental() {
		// 增量模式：基于当前扫描产生的RiskFeatureHash，查找历史上相同特征但未处置的风险
		// 逻辑：
		// 1. 找到当前批次产生的所有RiskFeatureHash
		// 2. 查找所有批次≤当前批次中，具有相同RiskFeatureHash但未处置的风险

		runtimeIDs := filter.GetRuntimeID()
		if len(runtimeIDs) > 0 {
			baseTaskID := runtimeIDs[0]

			// 获取基础任务的扫描批次
			var baseTask schema.SyntaxFlowScanTask
			if err := db.New().Where("task_id = ?", baseTaskID).First(&baseTask).Error; err == nil {
				// 第一步：获取当前批次产生的所有RiskFeatureHash
				var currentBatchFeatureHashes []string
				err := db.New().Model(&schema.SSARisk{}).
					Where("runtime_id = ? AND risk_feature_hash != ''", baseTaskID).
					Pluck("DISTINCT risk_feature_hash", &currentBatchFeatureHashes).Error

				if err != nil || len(currentBatchFeatureHashes) == 0 {
					// 如果当前批次没有风险或查询失败，返回空结果
					db = db.Where("1 = 0")
				} else {
					// 第二步：获取早于或等于基础批次的已被处置过的 risk_feature_hash
					var disposedFeatureHashes []string
					subQuery := db.New().Model(&schema.SSARiskDisposals{}).
						Joins("JOIN syntax_flow_scan_tasks ON ssa_risk_disposals.task_id = syntax_flow_scan_tasks.task_id").
						Where("ssa_risk_disposals.risk_feature_hash != '' AND ssa_risk_disposals.risk_feature_hash IN (?) AND syntax_flow_scan_tasks.scan_batch <= ?",
							currentBatchFeatureHashes, baseTask.ScanBatch)

					if err := subQuery.Pluck("DISTINCT ssa_risk_disposals.risk_feature_hash", &disposedFeatureHashes).Error; err == nil && len(disposedFeatureHashes) > 0 {
						// 第三步：查找具有当前批次RiskFeatureHash但未处置的历史风险
						// 条件：批次≤当前批次 && RiskFeatureHash在当前批次中 && 该特征未被处置
						validFeatureHashes := make([]string, 0)
						for _, hash := range currentBatchFeatureHashes {
							found := false
							for _, disposedHash := range disposedFeatureHashes {
								if hash == disposedHash {
									found = true
									break
								}
							}
							if !found {
								validFeatureHashes = append(validFeatureHashes, hash)
							}
						}

						if len(validFeatureHashes) > 0 {
							// 查找具有有效特征的历史风险
							db = db.Select("ssa_risks.*").
								Joins("JOIN syntax_flow_scan_tasks ON ssa_risks.runtime_id = syntax_flow_scan_tasks.task_id").
								Where("ssa_risks.risk_feature_hash IN (?) AND syntax_flow_scan_tasks.scan_batch <= ?",
									validFeatureHashes, baseTask.ScanBatch)
						} else {
							// 所有特征都已处置，返回空结果
							db = db.Where("1 = 0")
						}
					} else {
						// 没有已处置的特征，查找所有具有当前批次RiskFeatureHash的历史风险
						db = db.Select("ssa_risks.*").
							Joins("JOIN syntax_flow_scan_tasks ON ssa_risks.runtime_id = syntax_flow_scan_tasks.task_id").
							Where("ssa_risks.risk_feature_hash IN (?) AND syntax_flow_scan_tasks.scan_batch <= ?",
								currentBatchFeatureHashes, baseTask.ScanBatch)
					}
				}
			} else {
				// 如果无法获取基础任务信息，回退到简单的未处置过滤
				log.Errorf("Failed to get base task scan batch for incremental query: %v", err)
				var disposedFeatureHashes []string
				if err := db.New().Model(&schema.SSARiskDisposals{}).
					Where("risk_feature_hash != ''").
					Pluck("DISTINCT risk_feature_hash", &disposedFeatureHashes).Error; err == nil && len(disposedFeatureHashes) > 0 {
					db = db.Where("risk_feature_hash != ''").
						Where("risk_feature_hash NOT IN (?)", disposedFeatureHashes)
				} else {
					db = db.Where("risk_feature_hash != ''")
				}
			}
		} else {
			// 如果没有 RuntimeID，回退到简单的未处置过滤
			var disposedFeatureHashes []string
			if err := db.New().Model(&schema.SSARiskDisposals{}).
				Where("risk_feature_hash != ''").
				Pluck("DISTINCT risk_feature_hash", &disposedFeatureHashes).Error; err == nil && len(disposedFeatureHashes) > 0 {
				db = db.Where("risk_feature_hash != ''").
					Where("risk_feature_hash NOT IN (?)", disposedFeatureHashes)
			} else {
				db = db.Where("risk_feature_hash != ''")
			}
		}
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
