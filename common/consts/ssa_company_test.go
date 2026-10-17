package consts

import (
	"github.com/yaklang/gorm"
	"testing"
)

func TestCompanySSAIRBindingFailsClosed(t *testing.T) {
	db, err := gorm.Open(SQLite, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := CheckCompanySSAIRBinding(db, "company-a"); err == nil {
		t.Fatal("accepted missing marker")
	}
	if err := db.Exec("CREATE TABLE legion_company_ir_binding (singleton boolean PRIMARY KEY CHECK(singleton), company_id text NOT NULL, schema_version integer NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := CheckCompanySSAIRBinding(db, "company-a"); err == nil {
		t.Fatal("accepted empty marker")
	}
	if err := db.Exec("INSERT INTO legion_company_ir_binding VALUES (true, 'company-a', 1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := CheckCompanySSAIRBinding(db, "company-a"); err != nil {
		t.Fatal(err)
	}
	if err := CheckCompanySSAIRBinding(db, "company-b"); err == nil {
		t.Fatal("accepted another company")
	}
	db.Exec("UPDATE legion_company_ir_binding SET schema_version = 2")
	if err := CheckCompanySSAIRBinding(db, "company-a"); err == nil {
		t.Fatal("accepted future schema")
	}
}

func TestCompanySSAIRRuntimeRejectsSQLiteAndMigration(t *testing.T) {
	SetSSADatabaseCompanyID("company-a")
	defer SetSSADatabaseCompanyID("")
	SetSSADatabaseSkipMigrate(true)
	defer SetSSADatabaseSkipMigrate(false)
	if _, err := CreateSSAProjectDatabase(SQLiteExtend, ":memory:"); err == nil {
		t.Fatal("company runtime accepted SQLite")
	}
	SetSSADatabaseSkipMigrate(false)
	if _, err := CreateSSAProjectDatabase(Postgres, "postgres://invalid.invalid/unused"); err == nil {
		t.Fatal("company runtime accepted migration")
	}
	if err := ManageCompanySSAIRSchema("sqlite://unused", "company-a", "init"); err == nil {
		t.Fatal("operations accepted SQLite")
	}
	if err := ManageCompanySSAIRSchema("postgres://unused/db", "", "init"); err == nil {
		t.Fatal("operations accepted missing company")
	}
}
