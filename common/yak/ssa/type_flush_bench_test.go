package ssa

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

// newTypeFlushBenchStore builds the production schema: SaveIrTypeBatch upserts
// through UNIQUE (program_name, type_id), so a bare AutoMigrate of IrType is
// not enough (the ON CONFLICT target must exist).
func newTypeFlushBenchStore(b *testing.B, typeCount int) *typeStore {
	b.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(b.TempDir(), "types.db"))
	require.NoError(b, err)
	b.Cleanup(func() { db.Close() })
	require.NoError(b, db.AutoMigrate(&ssadb.IrType{}).Error)
	require.NoError(b, ssadb.EnsureIrTypeIndexesForTest(db))
	store := &typeStore{mode: ProgramCacheDBWrite, db: db, programName: "bench",
		saveSize: 512, nextID: atomic.NewInt64(0), resident: utils.NewSafeMapWithKey[int64, Type]()}
	for i := 1; i <= typeCount; i++ {
		typ := NewObjectType()
		typ.SetId(int64(i))
		typ.Name = fmt.Sprintf("Type%d", i)
		typ.AddFullTypeName(strings.Repeat("org.example.package.Type", 20))
		store.remember(typ)
	}
	require.NoError(b, store.flush())
	return store
}

// Later compile batches revisit all prior types, although most are unchanged.
// Use real SQLite persistence so the benchmark covers that checkpoint path.
func BenchmarkTypeFlushUnchanged(b *testing.B) {
	store := newTypeFlushBenchStore(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.flush(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTypeFlushUnchangedSizes(b *testing.B) {
	for _, size := range []int{10000, 100000, 1000000} {
		b.Run(fmt.Sprintf("types=%d", size), func(b *testing.B) {
			store := newTypeFlushBenchStore(b, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTypeInternPrimitive covers the cost that decides whether
// typeStore.remember may hash every fresh type: a program creates millions of
// equal primitives and each one must resolve to the already stored id.
func BenchmarkTypeInternPrimitive(b *testing.B) {
	store := &typeStore{
		mode:        ProgramCacheDBWrite,
		programName: "bench",
		nextID:      atomic.NewInt64(0),
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.remember(CreateStringType())
	}
}
