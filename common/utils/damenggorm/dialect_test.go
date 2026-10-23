package damenggorm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

type noQueryDB struct{ calls int }

func (d *noQueryDB) Exec(string, ...interface{}) (sql.Result, error) {
	d.calls++
	return nil, errors.New("unexpected SQL")
}
func (d *noQueryDB) Query(string, ...interface{}) (*sql.Rows, error) {
	d.calls++
	return nil, errors.New("unexpected SQL")
}
func (d *noQueryDB) Prepare(string) (*sql.Stmt, error) {
	d.calls++
	return nil, errors.New("unexpected SQL")
}
func (d *noQueryDB) QueryRow(string, ...interface{}) *sql.Row { d.calls++; return nil }

type policyConnector struct{}

func (policyConnector) Connect(context.Context) (driver.Conn, error) { return policyConn{}, nil }
func (policyConnector) Driver() driver.Driver                        { return policyDriver{} }

type policyDriver struct{}

func (policyDriver) Open(string) (driver.Conn, error) { return policyConn{}, nil }

type policyConn struct{}

func (policyConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected SQL") }
func (policyConn) Close() error                        { return nil }
func (policyConn) Begin() (driver.Tx, error)           { return policyTx{}, nil }

type policyTx struct{}

func (policyTx) Commit() error   { return nil }
func (policyTx) Rollback() error { return nil }

func TestSSAAliasPreservesPolicyAndGenericBatchAcrossClones(t *testing.T) {
	common := &noQueryDB{}
	db, err := gorm.Open(ORMName, common)
	require.NoError(t, err)
	generic, ok := gorm.GetDialect(Dialect)
	require.True(t, ok)
	require.Equal(t, Dialect, generic.GetName())
	require.Equal(t, `"Mi""XeD"`, generic.Quote(`Mi"XeD`))

	for _, clone := range []*gorm.DB{db, db.New(), db.Table("ir_codes").Where("program_name = ?", "value")} {
		require.Equal(t, ORMName, clone.Dialect().GetName())
		require.True(t, IsDM(clone))
		require.Equal(t, `"MI""XED"`, clone.Dialect().Quote(`Mi"XeD`))
		require.Equal(t, 1, clone.Dialect().(gorm.BatchCreateDialect).BatchCreateMaxRows())
		query, err := clone.Dialect().(gorm.BatchSaveDialect).BatchSaveSQL(`"IR_CODES"`, []string{`"ID"`, `"STRING"`}, []string{`"ID"`}, []string{`"STRING"`}, [][]string{{"?", "?"}})
		require.NoError(t, err)
		require.Contains(t, query, `MERGE INTO "IR_CODES"`)
		require.Contains(t, query, `UPDATE SET "STRING"="gorm_source"."STRING"`)
		require.NotContains(t, query, "DELETE")
	}
	require.Zero(t, common.calls, "cloning policy must not initialize or ping another pool")
	require.False(t, IsDM(nil))
	require.False(t, IsDialect("postgres"))
}

func TestSSAQueryAndTransactionClonesRetainPolicy(t *testing.T) {
	pool := sql.OpenDB(policyConnector{})
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(ORMName, pool)
	require.NoError(t, err)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.IsType(t, &sql.Tx{}, tx.CommonDB())
	for _, clone := range []*gorm.DB{tx, tx.New(), tx.Where("program_name = ?", "value")} {
		require.Equal(t, ORMName, clone.Dialect().GetName())
		require.Equal(t, `"IR_CODES"`, clone.Dialect().Quote("ir_codes"))
		require.Implements(t, (*gorm.BatchCreateDialect)(nil), clone.Dialect())
		require.Implements(t, (*gorm.BatchSaveDialect)(nil), clone.Dialect())
		require.Same(t, tx.CommonDB(), clone.CommonDB())
	}
	require.NoError(t, tx.Rollback().Error)
}

func TestSSAFieldPolicyDoesNotChangeCachedMetadata(t *testing.T) {
	type row struct {
		ID          uint
		ProgramName string
		Body        string
		Indexed     string `gorm:"index"`
		Unique      string `gorm:"unique"`
		Short       string `gorm:"size:64"`
		ZeroSize    string `gorm:"size:0"`
		Text        string `gorm:"type:text;not null;default:'original'"`
		Default     string `gorm:"not null;default:'original';comment:'text remains unchanged'"`
		JSON        string `gorm:"type:jsonb"`
		Enabled     bool   `gorm:"default:true"`
	}
	d := &ssaDialect{}
	d.SetDB(&noQueryDB{})
	generic, ok := gorm.GetDialect(Dialect)
	require.True(t, ok)
	want := map[string]string{
		"id": "BIGINT IDENTITY(1,1)", "program_name": "VARCHAR(1024)", "body": "CLOB",
		"indexed": "VARCHAR(1024)", "unique": "VARCHAR(1024) UNIQUE", "short": "VARCHAR(64)",
		"zero_size": "VARCHAR(1024)", "text": "CLOB NOT NULL  DEFAULT 'original'",
		"default": "CLOB NOT NULL  DEFAULT 'original' COMMENT 'text remains unchanged'",
		"json":    "CLOB", "enabled": "BIT DEFAULT 1",
	}
	postgres, ok := gorm.GetDialect("postgres")
	require.True(t, ok)
	for _, field := range (&gorm.Scope{Value: &row{}}).GetModelStruct().StructFields {
		t.Run(field.DBName, func(t *testing.T) {
			original := generic.DataTypeOf(field)
			pgOriginal := postgres.DataTypeOf(field)
			require.Equal(t, want[field.DBName], d.DataTypeOf(field))
			require.Equal(t, original, generic.DataTypeOf(field))
			require.Equal(t, pgOriginal, postgres.DataTypeOf(field))
			// Cached field metadata is shared by dialects and compile workers.
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _ = d.DataTypeOf(field) }()
			}
			wg.Wait()
			require.Equal(t, original, generic.DataTypeOf(field))
		})
	}
}

func TestSSAConnectionOptionsPreserveSchemaAndLowercaseLabels(t *testing.T) {
	prepared, err := prepareDSN("dm://user:private-password@127.0.0.1:5236?schema=SSA_IR&schema=UNOWNED&SCHEMA=OTHER&columnNameCase=upper&COLUMNNAMECASE=unchanged&socketTimeout=30")
	require.NoError(t, err)
	u, err := url.Parse(prepared)
	require.NoError(t, err)
	require.Equal(t, []string{"SSA_IR"}, u.Query()["schema"])
	require.Equal(t, []string{"lower"}, u.Query()["columnNameCase"])
	require.NotContains(t, u.Query(), "SCHEMA")
	require.NotContains(t, u.Query(), "COLUMNNAMECASE")
	require.Equal(t, "30", u.Query().Get("socketTimeout"))
	password, ok := u.User.Password()
	require.True(t, ok)
	require.Equal(t, "private-password", password)
}
