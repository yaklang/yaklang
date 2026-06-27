package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestMigrateSQLiteVulnVerificationTextColumns_rewritesLegacyVarchar(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate_test.sqlite3")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?cache=shared"
	db, err := gorm.Open("sqlite3", dsn)
	require.NoError(t, err)
	defer func() {
		if s := db.DB(); s != nil {
			_ = s.Close()
		}
	}()

	require.NoError(t, db.Exec(`
CREATE TABLE vuln_verifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at datetime,
	updated_at datetime,
	session_id INTEGER NOT NULL,
	syntax_flow_finding_id INTEGER NOT NULL,
	status varchar(255),
	confidence INTEGER,
	exploit_payload varchar(255),
	exploit_response varchar(255),
	ai_analysis varchar(255),
	fix varchar(255));
`).Error)
	require.NoError(t, db.Exec(`
INSERT INTO vuln_verifications (created_at, updated_at, session_id, syntax_flow_finding_id, status, confidence, exploit_payload, exploit_response, ai_analysis, fix)
VALUES (datetime('now'), datetime('now'), 1, 9, 'confirmed', 8, 'p', ?, 'analysis', '')
`, longBlobForTest()).Error)

	require.NoError(t, migrateSQLiteVulnVerificationTextColumns(db))

	gotTypes := vulnVerificationColumnTypes(t, db, "exploit_payload", "exploit_response")
	for _, ct := range gotTypes {
		require.NotContains(t, strings.ToLower(ct), "varchar", "varchar columns must be rewritten: got %q", ct)
	}

	var got string
	require.NoError(t, db.Raw("SELECT exploit_response FROM vuln_verifications WHERE id = 1").Row().Scan(&got))
	require.Equal(t, longBlobForTest(), got, "migration must preserve exploit_response content")
}

func longBlobForTest() string {
	// deliberately longer than legacy varchar(255) DDL tooling expected
	buf := make([]byte, 400)
	for i := range buf {
		buf[i] = 'z'
	}
	return string(buf)
}

func vulnVerificationColumnTypes(t *testing.T, db *gorm.DB, names ...string) map[string]string {
	t.Helper()
	want := map[string]string{}
	for _, n := range names {
		want[n] = ""
	}
	rows, err := db.Raw("PRAGMA table_info(`vuln_verifications`)").Rows()
	require.NoError(t, err)
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		require.NoError(t, rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
		if _, ok := want[name]; ok {
			want[name] = ctype
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	return want
}
