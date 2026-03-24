package reportstore

import (
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

const SSAReportRecordTableName = "ssa_report_records"

type SSAReportRecord struct {
	gorm.Model

	Title             string
	PublishedAt       time.Time  `json:"published_at" gorm:"index"`
	Hash              string     `json:"hash" gorm:"unique_index"`
	Owner             string     `json:"owner" gorm:"index"`
	From              string     `json:"from" gorm:"index"`
	ReportType        string     `json:"report_type" gorm:"index"`
	ScopeType         string     `json:"scope_type" gorm:"index"`
	ScopeName         string     `json:"scope_name"`
	ProjectName       string     `json:"project_name" gorm:"index"`
	ProgramName       string     `json:"program_name" gorm:"index"`
	TaskID            string     `json:"task_id" gorm:"index"`
	TaskCount         int64      `json:"task_count"`
	ScanBatch         int64      `json:"scan_batch" gorm:"index"`
	RiskTotal         int64      `json:"risk_total"`
	RiskCritical      int64      `json:"risk_critical"`
	RiskHigh          int64      `json:"risk_high"`
	RiskMedium        int64      `json:"risk_medium"`
	RiskLow           int64      `json:"risk_low"`
	SourceFinishedAt  *time.Time `json:"source_finished_at" gorm:"index"`
	SourceRequestJSON string     `json:"source_request_json" gorm:"type:text"`
	SnapshotJSON      string     `json:"snapshot_json" gorm:"type:text"`
	PreviewJSON       string     `json:"preview_json" gorm:"type:text"`
}

func (*SSAReportRecord) TableName() string {
	return SSAReportRecordTableName
}

func (r *SSAReportRecord) CalcHash() string {
	return utils.CalcSha1(r.Title, r.PublishedAt.Format(utils.DefaultTimeFormat))
}

func (r *SSAReportRecord) BeforeSave() {
	if r == nil {
		return
	}
	if r.PublishedAt.IsZero() {
		r.PublishedAt = time.Now()
	}
	if strings.TrimSpace(r.Hash) == "" {
		r.Hash = r.CalcHash()
	}
}
