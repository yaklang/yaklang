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
)

// Later compile batches revisit all prior types, although most are unchanged.
// Use real SQLite persistence so the benchmark covers that checkpoint path.
func BenchmarkTypeFlushUnchanged(b *testing.B) {
	db, err := gorm.Open("sqlite3", filepath.Join(b.TempDir(), "types.db"))
	require.NoError(b, err)
	b.Cleanup(func() { db.Close() })
	require.NoError(b, db.AutoMigrate(&ssadb.IrType{}).Error)
	store := &typeStore{mode: ProgramCacheDBWrite, db: db, programName: "bench",
		saveSize: 512, resident: utils.NewSafeMapWithKey[int64, Type]()}
	for i := 1; i <= 10000; i++ {
		typ := NewObjectType()
		typ.SetId(int64(i))
		typ.Name = fmt.Sprintf("Type%d", i)
		typ.AddFullTypeName(strings.Repeat("org.example.package.Type", 20))
		store.remember(typ)
	}
	require.NoError(b, store.flush())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.flush(); err != nil {
			b.Fatal(err)
		}
	}
}
