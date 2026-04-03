package consts

import "testing"

func TestGetSSADataBaseInfo(t *testing.T) {
	oldDialect := SSA_PROJECT_DB_DIALECT
	oldRaw := SSA_PROJECT_DB_RAW
	t.Cleanup(func() {
		SSA_PROJECT_DB_DIALECT = oldDialect
		SSA_PROJECT_DB_RAW = oldRaw
	})

	SetSSADatabaseInfo("sqlite:///tmp/test-ssa.db")
	if dialect, raw := GetSSADataBaseInfo(); dialect != SQLiteExtend || raw != "/tmp/test-ssa.db" {
		t.Fatalf("sqlite db info = (%q, %q)", dialect, raw)
	}

	SetSSADatabaseInfo("postgres://user:pass@db.example.com:5432/yak?sslmode=disable")
	if dialect, raw := GetSSADataBaseInfo(); dialect != Postgres || raw != "postgres://user:pass@db.example.com:5432/yak?sslmode=disable" {
		t.Fatalf("postgres db info = (%q, %q)", dialect, raw)
	}
}
