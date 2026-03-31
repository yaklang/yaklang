package consts

import "testing"

func TestGetSSADataBaseConnString(t *testing.T) {
	oldDialect := SSA_PROJECT_DB_DIALECT
	oldRaw := SSA_PROJECT_DB_RAW
	t.Cleanup(func() {
		SSA_PROJECT_DB_DIALECT = oldDialect
		SSA_PROJECT_DB_RAW = oldRaw
	})

	SetSSADatabaseInfo("sqlite:///tmp/test-ssa.db")
	if got := GetSSADataBaseConnString(); got != "/tmp/test-ssa.db" {
		t.Fatalf("sqlite conn string = %q, want %q", got, "/tmp/test-ssa.db")
	}

	SetSSADatabaseInfo("postgres://user:pass@db.example.com:5432/yak?sslmode=disable")
	if got := GetSSADataBaseConnString(); got != "postgres://user:pass@db.example.com:5432/yak?sslmode=disable" {
		t.Fatalf("postgres conn string = %q", got)
	}
}
