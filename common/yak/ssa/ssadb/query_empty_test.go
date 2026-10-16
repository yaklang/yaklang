package ssadb

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func countGORMReadQueries(db *gorm.DB) *atomic.Int64 {
	count := new(atomic.Int64)
	record := func(*gorm.Scope) { count.Add(1) }
	db.Callback().Query().Before("gorm:query").Register("test:empty_query", record)
	db.Callback().RowQuery().Before("gorm:row_query").Register("test:empty_query", record)
	return count
}

func TestSearchVariableKnownEmptyDoesNotQuery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    CompareMode
		pattern string
	}{
		{"exact", ExactCompare, "absent"},
		{"glob", GlobCompare, "absent*"},
		{"regexp", RegexpCompare, "^absent.*$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupA2LoaderDB(t)
			require.NoError(t, db.AutoMigrate(&IrIndex{}).Error)
			cache := NewNameCache(t.Name(), false)
			cache.GetID("present")
			count := countGORMReadQueries(db)
			for _, filter := range []struct{ include, exclude []string }{
				{}, {include: []string{"src/*.go"}}, {exclude: []string{"ignored.go"}},
			} {
				ch := searchVariableWithFileFilter(db, context.Background(), t.Name(), cache, tc.mode, NameMatch|KeyMatch, tc.pattern, filter.exclude, filter.include)
				for row := range ch {
					t.Fatalf("empty candidate set returned IR %d", row.CodeID)
				}
			}
			require.Zero(t, count.Load(), "a known-empty name pool result must not execute SELECT ... WHERE 1 = 0")
		})
	}
}

func TestIrCodeNativeMissDoesNotRetryGORM(t *testing.T) {
	db := setupA2LoaderDB(t)
	count := countGORMReadQueries(db)
	require.Nil(t, GetIrCodeItemById(db, t.Name(), 99))
	require.Zero(t, count.Load(), "native ErrNoRows is a normal miss, not a reason to query the same key again")
}

func TestIrCodeNativeMissBecomesHitAfterInsert(t *testing.T) {
	db := setupA2LoaderDB(t)
	count := countGORMReadQueries(db)
	require.Nil(t, GetIrCodeItemById(db, t.Name(), 7))
	require.NoError(t, db.Create(&IrCode{CodeID: 7, ProgramName: t.Name(), Name: "late"}).Error)
	row := GetIrCodeItemById(db, t.Name(), 7)
	require.NotNil(t, row, "a normal miss must not become a negative cache entry during compilation")
	require.Equal(t, "late", row.Name)
	require.Zero(t, count.Load())
}

func TestSearchVariableKnownNamesPreserveMatches(t *testing.T) {
	for _, mode := range []CompareMode{ExactCompare, GlobCompare, RegexpCompare} {
		t.Run([]string{"exact", "glob", "regexp"}[mode], func(t *testing.T) {
			db := setupA2LoaderDB(t)
			require.NoError(t, db.AutoMigrate(&IrIndex{}).Error)
			cache := NewNameCache(t.Name(), false)
			nameID := cache.GetID("present")
			require.NoError(t, db.Create(&IrCode{CodeID: 7, ProgramName: t.Name(), Name: "present"}).Error)
			require.NoError(t, db.Create(&IrIndex{ProgramName: t.Name(), ValueID: 7, VariableID: &nameID}).Error)
			var got []int64
			for row := range SearchVariable(db, context.Background(), t.Name(), cache, mode, NameMatch, "present") {
				got = append(got, row.CodeID)
			}
			require.Equal(t, []int64{7}, got)
		})
	}
}

func TestIrCodeNativeFailureStillFallsBack(t *testing.T) {
	// A legacy/minimal schema cannot satisfy the native fixed column list.
	// GORM SELECT * can still read its available fields; preserve that fallback.
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE ir_codes (id INTEGER PRIMARY KEY, code_id INTEGER, program_name TEXT, name TEXT, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO ir_codes(id,code_id,program_name,name) VALUES (1,7,?,?)`, t.Name(), "legacy").Error)
	count := countGORMReadQueries(db)
	row := GetIrCodeItemById(db, t.Name(), 7)
	require.NotNil(t, row)
	require.Equal(t, "legacy", row.Name)
	require.Equal(t, int64(1), count.Load(), "a real native schema error must retain the compatible GORM read")
}
