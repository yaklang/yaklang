package ssadb

import (
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/reportstore"
)

var SSAProjectTables = []any{
	// instruction
	&IrCode{},
	&IrIndex{},
	&IrNamePool{},
	// source code
	&IrSource{},
	// type
	&IrType{},
	// program
	&IrProgram{},
	&IrOffset{},

	// audit
	&AuditResult{},
	&AuditNode{},
	&AuditEdge{},
	&schema.SSARisk{},
	&schema.SSARiskDisposals{},

	&schema.SyntaxFlowScanTask{},

	// report
	&schema.ReportRecord{},
	&reportstore.SSAReportRecord{},
	&reportstore.SSAReportRecordFile{},

	//diff result
	&schema.SSADiffResult{},

	&schema.ProjectGeneralStorage{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrSourceQuotedCode)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrCodeIndex)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchSSAReportStoreTextColumns)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchMigrateLegacySSAReportStore)
}

// patchIrSourceQuotedCode patches the QuotedCode column type based on database dialect
// MySQL: use LONGTEXT for large text storage (up to 4GB)
// PostgreSQL: use TEXT (unlimited length)
// SQLite: use TEXT (supports up to 2GB, no modification needed)
func patchIrSourceQuotedCode(db *gorm.DB) {
	if !db.HasTable(TableIrSources) {
		return
	}

	dialect := db.Dialect().GetName()
	switch dialect {
	case "mysql":
		// For MySQL, change TEXT to LONGTEXT to support larger source files
		// TEXT in MySQL is limited to ~64KB, but LONGTEXT can store up to 4GB
		err := db.Exec("ALTER TABLE " + TableIrSources + " MODIFY COLUMN quoted_code LONGTEXT").Error
		if err != nil {
			log.Warnf("failed to modify %s.quoted_code to LONGTEXT for MySQL: %v", TableIrSources, err)
		} else {
			log.Infof("MySQL: %s.quoted_code column type changed to LONGTEXT", TableIrSources)
		}
	case "postgres", "postgresql":
		// PostgreSQL TEXT type already supports unlimited length, no modification needed
		log.Debugf("PostgreSQL: %s.quoted_code uses TEXT type (unlimited length)", TableIrSources)
	case "sqlite3", "sqlite":
		// SQLite TEXT type supports up to 2GB (SQLITE_MAX_LENGTH), no modification needed
		log.Debugf("SQLite: %s.quoted_code uses TEXT type (up to 2GB)", TableIrSources)
	default:
		// For other databases, use default TEXT type
		log.Debugf("Database dialect %s: using default TEXT type for %s.quoted_code", dialect, TableIrSources)
	}
}

// doSSAPatch 添加数据库索引以优化查询性能
func patchIrCodeIndex(db *gorm.DB) {
	if !db.HasTable(TableIrCodes) {
		return
	}

	// 为 ir_codes 表添加复合索引 (program_name, code_id)
	// 这是最常见的查询模式: WHERE program_name = ? AND code_id IN (...)
	indexQueries := []struct {
		name  string
		query string
	}{
		{
			"idx_ir_codes_program_code",
			`CREATE INDEX IF NOT EXISTS "idx_ir_codes_program_code" ON "` + TableIrCodes + `" ("program_name", "code_id");`,
		},
		{
			"idx_ir_codes_program_opcode",
			// composite index for program+opcode lookups
			`CREATE INDEX IF NOT EXISTS "idx_ir_codes_program_opcode" ON "` + TableIrCodes + `" ("program_name", "opcode");`,
		},
		// 为 ir_types 表添加复合索引
		{
			"idx_ir_types_program_type",
			`CREATE INDEX IF NOT EXISTS "idx_ir_types_program_type" ON "` + TableIrTypes + `" ("program_name", "type_id");`,
		},
		// 为 ir_indices 表添加复合索引以优化常见查询
		{
			"idx_ir_indices_program_value",
			`CREATE INDEX IF NOT EXISTS "idx_ir_indices_program_value" ON "` + TableIrIndices + `" ("program_name", "value_id");`,
		},
		{
			"idx_ir_name_pool_program_name_name",
			`CREATE INDEX IF NOT EXISTS "idx_ir_name_pool_program_name_name" ON "` + TableIrNamePool + `" ("program_name", "name");`,
		},
	}

	for _, idx := range indexQueries {
		if err := db.Exec(idx.query).Error; err != nil {
			log.Warnf("failed to add index %s: %v", idx.name, err)
		}
	}
}

func patchSSAReportStoreTextColumns(db *gorm.DB) {
	if !db.HasTable(&reportstore.SSAReportRecord{}) {
		return
	}
	dialect := db.Dialect().GetName()
	if dialect != "mysql" {
		return
	}

	queries := []struct {
		name  string
		query string
	}{
		{
			"ssa_report_records.source_request_json",
			"ALTER TABLE `" + reportstore.SSAReportRecordTableName + "` MODIFY COLUMN source_request_json LONGTEXT",
		},
		{
			"ssa_report_records.snapshot_json",
			"ALTER TABLE `" + reportstore.SSAReportRecordTableName + "` MODIFY COLUMN snapshot_json LONGTEXT",
		},
		{
			"ssa_report_records.preview_json",
			"ALTER TABLE `" + reportstore.SSAReportRecordTableName + "` MODIFY COLUMN preview_json LONGTEXT",
		},
	}

	for _, item := range queries {
		if err := db.Exec(item.query).Error; err != nil {
			log.Warnf("failed to widen %s to LONGTEXT: %v", item.name, err)
		}
	}
}

func patchMigrateLegacySSAReportStore(db *gorm.DB) {
	if !db.HasTable(&schema.ReportRecord{}) || !db.HasTable(&reportstore.SSAReportRecord{}) {
		return
	}

	var legacyRecords []*schema.ReportRecord
	if err := db.Model(&schema.ReportRecord{}).Find(&legacyRecords).Error; err != nil {
		log.Warnf("failed to query legacy ssa report records for migration: %v", err)
		return
	}

	for _, legacy := range legacyRecords {
		if legacy == nil {
			continue
		}
		reportType := strings.TrimSpace(legacy.ReportType)
		if reportType == "" {
			reportType = strings.TrimSpace(legacy.From)
		}
		if reportType != "ssa-scan" {
			continue
		}
		var count int64
		if err := db.Model(&reportstore.SSAReportRecord{}).Where("id = ?", legacy.ID).Count(&count).Error; err != nil {
			log.Warnf("failed to count migrated ssa report record id=%d: %v", legacy.ID, err)
			continue
		}
		if count > 0 {
			continue
		}

		previewJSON := strings.TrimSpace(legacy.QuotedJson)
		if unquoted, err := strconv.Unquote(legacy.QuotedJson); err == nil {
			previewJSON = unquoted
		}

		record := &reportstore.SSAReportRecord{
			Model:             legacy.Model,
			Title:             legacy.Title,
			PublishedAt:       legacy.PublishedAt,
			Hash:              legacy.Hash,
			Owner:             legacy.Owner,
			From:              legacy.From,
			ReportType:        reportType,
			ScopeType:         legacy.ScopeType,
			ScopeName:         legacy.ScopeName,
			ProjectName:       legacy.ProjectName,
			ProgramName:       legacy.ProgramName,
			TaskID:            legacy.TaskID,
			TaskCount:         legacy.TaskCount,
			ScanBatch:         legacy.ScanBatch,
			RiskTotal:         legacy.RiskTotal,
			RiskCritical:      legacy.RiskCritical,
			RiskHigh:          legacy.RiskHigh,
			RiskMedium:        legacy.RiskMedium,
			RiskLow:           legacy.RiskLow,
			SourceFinishedAt:  legacy.SourceFinishedAt,
			SourceRequestJSON: legacy.QueryJSON,
			PreviewJSON:       previewJSON,
		}
		if strings.TrimSpace(record.ReportType) == "" {
			record.ReportType = strings.TrimSpace(legacy.From)
		}

		if err := db.Create(record).Error; err != nil {
			log.Warnf("failed to migrate legacy ssa report record id=%d: %v", legacy.ID, err)
		}
	}

	if !db.HasTable(&schema.ReportRecordFile{}) || !db.HasTable(&reportstore.SSAReportRecordFile{}) {
		return
	}

	var legacyFiles []*schema.ReportRecordFile
	if err := db.Model(&schema.ReportRecordFile{}).Find(&legacyFiles).Error; err != nil {
		log.Warnf("failed to query legacy ssa report files for migration: %v", err)
		return
	}
	for _, legacy := range legacyFiles {
		if legacy == nil {
			continue
		}
		var count int64
		if err := db.Model(&reportstore.SSAReportRecordFile{}).Where("id = ?", legacy.ID).Count(&count).Error; err != nil {
			log.Warnf("failed to count migrated ssa report file id=%d: %v", legacy.ID, err)
			continue
		}
		if count > 0 {
			continue
		}
		file := &reportstore.SSAReportRecordFile{
			Model:           legacy.Model,
			ReportRecordID:  legacy.ReportRecordID,
			Format:          legacy.Format,
			FileName:        legacy.FileName,
			ObjectKey:       legacy.ObjectKey,
			Bucket:          legacy.Bucket,
			ContentType:     legacy.ContentType,
			SizeBytes:       legacy.SizeBytes,
			SHA256:          legacy.SHA256,
			Status:          legacy.Status,
			CreatedBy:       legacy.CreatedBy,
			GenerationError: legacy.GenerationError,
		}
		if err := db.Create(file).Error; err != nil {
			log.Warnf("failed to migrate legacy ssa report file id=%d: %v", legacy.ID, err)
		}
	}
}

func GetDB() *gorm.DB {
	return consts.GetGormSSAProjectDataBase()
}

func SetDB(db *gorm.DB) {
	consts.SetGormSSAProjectDatabase(db)
}

func DeleteProgram(db *gorm.DB, program string) {
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		tx.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{})
		deleteProgramCodeOnly(tx, program)
		deleteProgramAuditResult(tx, program)
		deleteProgramRiskAndScanTask(tx, program)
		return nil
	})
}

func DeleteProgramIrCode(db *gorm.DB, program string) {
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		deleteProgramCodeOnly(tx, program)
		deleteProgramAuditResult(tx, program) // because audit result depends on ir code
		return nil
	})
}

func deleteProgramCodeOnly(db *gorm.DB, program string) {
	// delete the program
	// code
	deleteCache(program)
	db.Model(&IrCode{}).Where("program_name = ?", program).Unscoped().Delete(&IrCode{})
	db.Model(&IrIndex{}).Where("program_name = ?", program).Unscoped().Delete(&IrIndex{})
	db.Model(&IrNamePool{}).Where("program_name = ?", program).Unscoped().Delete(&IrNamePool{})
	db.Model(&IrSource{}).Where("program_name = ?", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrSource{}).Where("folder_path = ? AND file_name = ?", "/", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrType{}).Where("program_name = ?", program).Unscoped().Delete(&IrType{})
	db.Model(&IrOffset{}).Where("program_name = ?", program).Unscoped().Delete(&IrOffset{})
}

func deleteProgramAuditResult(db *gorm.DB, program string) {
	// analyze result
	db.Model(&AuditResult{}).Where("program_name = ?", program).Unscoped().Delete(&AuditResult{})
	db.Model(&AuditNode{}).Where("program_name = ?", program).Unscoped().Delete(&AuditNode{})
	db.Model(&AuditEdge{}).Where("program_name = ?", program).Unscoped().Delete(&AuditEdge{})
}

func deleteProgramRiskAndScanTask(db *gorm.DB, program string) {
	// risk and scan task
	db.Model(&schema.SSARisk{}).Where("program_name = ?", program).Unscoped().Delete(&schema.SSARisk{})
	db.Model(&schema.SyntaxFlowScanTask{}).Where("programs = ?", program).Unscoped().Delete(&schema.SyntaxFlowScanTask{})
}
