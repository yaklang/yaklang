package consts

import (
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

const CompanySSAIRSchemaVersion = 1

func SetSSADatabaseCompanyID(companyID string) {
	ssaDatabaseCompanyID.Store(strings.TrimSpace(companyID))
}

// CheckCompanySSAIRBinding uses only SELECT and never repairs a missing marker.
// The marker is owned by operations; runtime roles have SELECT permission only.
func CheckCompanySSAIRBinding(db *gorm.DB, companyID string) error {
	if strings.TrimSpace(companyID) == "" {
		return fmt.Errorf("company ID is required for SSA IR schema check")
	}
	rows, err := db.Raw("SELECT company_id, schema_version FROM legion_company_ir_binding WHERE singleton = true").Rows()
	if err != nil {
		return fmt.Errorf("company SSA IR binding is unavailable: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("company SSA IR binding is missing")
	}
	var boundCompany string
	var version int
	if err := rows.Scan(&boundCompany, &version); err != nil {
		return err
	}
	if boundCompany != companyID {
		return fmt.Errorf("SSA IR database belongs to a different company")
	}
	if version != CompanySSAIRSchemaVersion {
		return fmt.Errorf("SSA IR schema version %d is incompatible with required version %d", version, CompanySSAIRSchemaVersion)
	}
	if rows.Next() {
		return fmt.Errorf("company SSA IR binding is not unique")
	}
	return rows.Err()
}

// ManageCompanySSAIRSchema is invoked only by the explicit operations CLI.
// PostgreSQL transactional DDL also makes a failing legacy patch abort Commit,
// even where the registered patch historically logged rather than returned it.
func ManageCompanySSAIRSchema(raw, companyID, action string) error {
	companyID = strings.TrimSpace(companyID)
	if companyID == "" {
		return fmt.Errorf("company ID is required")
	}
	if action != "init" && action != "migrate" && action != "check" {
		return fmt.Errorf("action must be init, migrate, or check")
	}
	dialect, dsn := parseDatabaseURL(raw)
	if dialect != Postgres {
		return fmt.Errorf("company SSA IR operations require a PostgreSQL DSN")
	}
	db, err := gorm.Open(dialect, dsn)
	if err != nil {
		return fmt.Errorf("open company SSA IR database: %w", err)
	}
	defer db.Close()
	db.LogMode(false)
	if action == "check" {
		return CheckCompanySSAIRBinding(db, companyID)
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	// Serialize initialization before the singleton exists.
	if err := tx.Exec("SELECT pg_advisory_xact_lock(681904338201)").Error; err != nil {
		return err
	}
	if err := tx.Exec(`CREATE TABLE IF NOT EXISTS legion_company_ir_binding (
 singleton boolean PRIMARY KEY CHECK (singleton),
 company_id text NOT NULL,
 schema_version integer NOT NULL
 )`).Error; err != nil {
		return err
	}
	rows, err := tx.Raw("SELECT company_id, schema_version FROM legion_company_ir_binding WHERE singleton = true FOR UPDATE").Rows()
	if err != nil {
		return err
	}
	exists := rows.Next()
	var boundCompany string
	var version int
	if exists {
		err = rows.Scan(&boundCompany, &version)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if exists && boundCompany != companyID {
		return fmt.Errorf("SSA IR database belongs to a different company")
	}
	if exists && version > CompanySSAIRSchemaVersion {
		return fmt.Errorf("cannot downgrade SSA IR schema version %d", version)
	}
	if action == "migrate" && !exists {
		return fmt.Errorf("SSA IR binding is missing; initialize it explicitly first")
	}
	if err := schema.AutoMigrateWithError(tx, schema.KEY_SCHEMA_SSA_DATABASE); err != nil {
		return fmt.Errorf("migrate SSA IR models: %w", err)
	}
	schema.ApplyPatches(tx, schema.KEY_SCHEMA_SSA_DATABASE)
	if tx.Error != nil {
		return fmt.Errorf("migrate SSA IR patches: %w", tx.Error)
	}
	if exists {
		err = tx.Exec("UPDATE legion_company_ir_binding SET schema_version = ? WHERE singleton = true", CompanySSAIRSchemaVersion).Error
	} else {
		err = tx.Exec("INSERT INTO legion_company_ir_binding (singleton, company_id, schema_version) VALUES (true, ?, ?)", companyID, CompanySSAIRSchemaVersion).Error
	}
	if err != nil {
		return err
	}
	return tx.Commit().Error
}
