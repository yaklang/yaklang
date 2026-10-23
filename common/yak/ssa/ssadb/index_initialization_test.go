package ssadb

import (
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

// A disposable schema keeps this regression independent of the server's data.
func postgresIndexFixture(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	raw := os.Getenv("YAK_SSA_POSTGRES_TEST_DSN")
	if raw == "" {
		t.Skip("YAK_SSA_POSTGRES_TEST_DSN is not configured")
	}
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, u.Scheme)
	admin, err := gorm.Open("postgres", raw)
	require.NoError(t, err)
	schema := "ssa_index_" + uuid.New().String()[:8]
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; _ = admin.Close() })
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open("postgres", u.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, u.String()
}

func TestPostgresRequiredIRIndexesRejectDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name, table, columns, values string
		patch                        func(*gorm.DB)
	}{
		{"codes", "ir_codes", "id BIGSERIAL PRIMARY KEY, program_name TEXT, code_id BIGINT", "'p', 1", ensureUniqueIrCodesProgramCodeIndex},
		{"types", "ir_types", "id BIGSERIAL PRIMARY KEY, program_name TEXT, type_id BIGINT", "'p', 1", ensureUniqueIrTypesProgramTypeIndex},
		{"offsets", "ir_offsets", "id BIGSERIAL PRIMARY KEY, program_name TEXT, value_id BIGINT, file_hash TEXT, start_offset BIGINT, end_offset BIGINT, variable_name TEXT", "'p', 1, 'hash', 0, 1, NULL", ensureUniqueIrOffsetsIndex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := postgresIndexFixture(t)
			require.NoError(t, db.Exec("CREATE TABLE "+tc.table+" ("+tc.columns+")").Error)
			insert := "INSERT INTO " + tc.table + " VALUES (DEFAULT, " + tc.values + ")"
			require.NoError(t, db.Exec(insert).Error)
			require.NoError(t, db.Exec(insert).Error)
			tc.patch(db)
			require.ErrorContains(t, db.Error, "install required SSA index")
			var count int
			require.NoError(t, db.DB().QueryRow("SELECT COUNT(*) FROM "+tc.table).Scan(&count))
			require.Equal(t, 2, count, "failed initialization must retain historical rows")
			// An operator's explicit repair permits the next initialization.
			db.Error = nil
			require.NoError(t, db.Exec("DELETE FROM "+tc.table+" WHERE id > (SELECT MIN(id) FROM "+tc.table+")").Error)
			tc.patch(db)
			require.NoError(t, db.Error)
			require.Error(t, db.Exec(insert).Error, "the required uniqueness invariant must be enforced")
		})
	}
}

func TestPostgresSSAInitializationReturnsPatchFailure(t *testing.T) {
	db, dsn := postgresIndexFixture(t)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&IrCode{ProgramName: "duplicate", CodeID: 1}).Error)
	}
	t.Setenv(consts.ENV_SSA_DB_SKIP_MIGRATE, "")
	previous := consts.SSADatabaseSkipMigrate()
	consts.SetSSADatabaseSkipMigrate(false)
	t.Cleanup(func() { consts.SetSSADatabaseSkipMigrate(previous) })
	initialized, err := consts.CreateSSAProjectDatabaseRaw(dsn)
	if initialized != nil {
		t.Cleanup(func() { _ = initialized.Close() })
	}
	require.ErrorContains(t, err, "install required SSA index")
	require.Nil(t, initialized, "must not publish an unusable SSA handle")
	var count int
	require.NoError(t, db.DB().QueryRow("SELECT COUNT(*) FROM ir_codes WHERE program_name = 'duplicate'").Scan(&count))
	require.Equal(t, 2, count)
}

func TestMySQLExistingIRCodeIndexDoesNotPoisonHandle(t *testing.T) {
	raw := os.Getenv("YAK_SSA_MYSQL_TEST_DSN")
	if raw == "" {
		t.Skip("YAK_SSA_MYSQL_TEST_DSN is not configured")
	}
	// The supplied database must be a dedicated test database, not a live schema.
	db, err := gorm.Open("mysql", raw)
	require.NoError(t, err)
	defer db.Close()
	require.False(t, db.HasTable(TableIrCodes), "use an empty dedicated MySQL test database")
	require.NoError(t, db.Exec("CREATE TABLE ir_codes (id BIGINT PRIMARY KEY, program_name VARCHAR(64), code_id BIGINT, UNIQUE INDEX ux_ir_codes_program_code(program_name, code_id))").Error)
	defer db.Exec("DROP TABLE ir_codes")
	require.NoError(t, db.Exec("INSERT INTO ir_codes VALUES (1, 'existing', 42)").Error)
	ensureUniqueIrCodesProgramCodeIndex(db)
	require.NoError(t, db.Error)
	var code int64
	require.NoError(t, db.Raw("SELECT code_id FROM ir_codes WHERE id = ?", 1).Row().Scan(&code))
	require.Equal(t, int64(42), code, "existing MySQL IR must remain readable")
}
