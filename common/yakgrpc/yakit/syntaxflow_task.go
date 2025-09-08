package yakit

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func QuerySyntaxFlowScanTask(db *gorm.DB, params *ypb.QuerySyntaxFlowScanTaskRequest) (*bizhelper.Paginator, []*schema.SyntaxFlowScanTask, error) {
	db = db.Model(&schema.SyntaxFlowScanTask{})
	db = FilterSyntaxFlowScanTask(db, params.GetFilter())
	var data []*schema.SyntaxFlowScanTask
	paging := params.GetPagination()
	db = bizhelper.QueryOrder(db, paging.GetOrderBy(), paging.GetOrder())
	p, db := bizhelper.Paging(db, int(paging.GetPage()), int(paging.GetLimit()), &data)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, data, nil
}

func FilterSyntaxFlowScanTask(DB *gorm.DB, filter *ypb.SyntaxFlowScanTaskFilter) *gorm.DB {
	if filter == nil {
		return DB
	}
	db := DB
	db = bizhelper.FuzzQueryStringArrayOrLike(db, "programs", filter.GetPrograms())
	db = bizhelper.ExactQueryStringArrayOr(db, "task_id", filter.GetTaskIds())
	db = bizhelper.ExactQueryStringArrayOr(db, "status", filter.GetStatus())
	db = bizhelper.ExactQueryStringArrayOr(db, "kind", filter.GetKind())
	if filter.GetFromId() > 0 {
		db = db.Where("id > ?", filter.GetFromId())
	}
	if filter.GetUntilId() > 0 {
		db = db.Where("id <= ?", filter.GetUntilId())
	}
	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"programs",
		}, []string{filter.GetKeyword()}, false)
	}
	if filter.GetHaveRisk() {
		db = db.Where("EXISTS (SELECT 1 FROM ssa_risks WHERE ssa_risks.runtime_id = syntax_flow_scan_tasks.task_id)")
	}
	return db
}

func DeleteAllSyntaxFlowScanTask(db *gorm.DB) (int64, error) {
	db = db.Unscoped().Delete(&schema.SyntaxFlowScanTask{})
	return db.RowsAffected, db.Error
}

func DeleteSyntaxFlowScanTask(db *gorm.DB, params *ypb.DeleteSyntaxFlowScanTaskRequest) (int64, error) {
	db = db.Model(&schema.SyntaxFlowScanTask{})
	if params == nil || params.Filter == nil {
		return 0, utils.Errorf("delete syntaxFlow rule failed: synatx flow filter is nil")
	}
	db = FilterSyntaxFlowScanTask(db, params.Filter)
	db = db.Unscoped().Delete(&schema.SyntaxFlowScanTask{})
	return db.RowsAffected, db.Error
}

// GetMaxScanBatch 获取指定程序的最大扫描批次号
func GetMaxScanBatch(db *gorm.DB, programs []string) (uint64, error) {
	var result struct {
		MaxBatch uint64 `json:"max_batch"`
	}

	programsStr := strings.Join(programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	err := db.Model(&schema.SyntaxFlowScanTask{}).
		Where("programs = ?", programsStr).
		Select("COALESCE(MAX(scan_batch), 0) as max_batch").
		Scan(&result).Error
	if err != nil {
		return 0, err
	}
	return result.MaxBatch, nil
}

// FormatTaskName 格式化任务名称
// 例如: [批次8]JavaSecLab(2025-0905-16:25)<2025-09-05 16:25:00>
func FormatTaskName(scanBatch uint64, programs string, time time.Time) string {
	scanTime := time.Format("2006-01-02 15:04:05")
	return fmt.Sprintf("[批次%d]%s<%s>", scanBatch, programs, scanTime)
}

// GetFormattedTaskName 根据任务ID获取格式化的任务名称
// 如果任务不存在，返回任务ID本身
func GetFormattedTaskName(db *gorm.DB, taskId string) string {
	if taskId == "" {
		return ""
	}
	var task schema.SyntaxFlowScanTask
	if err := db.Where("task_id = ?", taskId).First(&task).Error; err != nil {
		return taskId
	}
	return FormatTaskName(task.ScanBatch, task.Programs, task.CreatedAt)
}
