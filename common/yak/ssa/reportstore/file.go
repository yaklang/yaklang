package reportstore

import "github.com/jinzhu/gorm"

const (
	SSAReportRecordFileTableName = "ssa_report_record_files"

	SSAReportRecordFileStatusReady   = "ready"
	SSAReportRecordFileStatusFailed  = "failed"
	SSAReportRecordFileStatusDeleted = "deleted"
)

type SSAReportRecordFile struct {
	gorm.Model

	ReportRecordID  uint   `json:"report_record_id" gorm:"index"`
	Format          string `json:"format" gorm:"index"`
	FileName        string `json:"file_name"`
	ObjectKey       string `json:"object_key" gorm:"unique_index"`
	Bucket          string `json:"bucket"`
	ContentType     string `json:"content_type"`
	SizeBytes       int64  `json:"size_bytes"`
	SHA256          string `json:"sha256" gorm:"index"`
	Status          string `json:"status" gorm:"index"`
	CreatedBy       string `json:"created_by" gorm:"index"`
	GenerationError string `json:"generation_error" gorm:"type:text"`
}

func (*SSAReportRecordFile) TableName() string {
	return SSAReportRecordFileTableName
}
