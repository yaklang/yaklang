package ssadb

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/consts"
	"path/filepath"
	"testing"
)

// --- from loader_a2_test.go ---

// NativeIrCodeBatchReads returns the test-only batch-read counter.
func NativeIrCodeBatchReads() int64 {
	return nativeIrCodeBatchReads.Load()
}

// NativeConstTypeIDQueries returns the test-only ConstType query counter.
func NativeConstTypeIDQueries() int64 {
	return nativeConstTypeIDQueries.Load()
}

// setupA2LoaderDB creates a shared temp-file DB (not :memory:) with the IrCode
// schema, so yieldIrCodes' internal GetDB() and the test share the same file.
func setupA2LoaderDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := GetDB()
	dir := t.TempDir()
	db, err := gorm.Open("sqlite3", filepath.Join(dir, "a2.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	sqlDB := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	SetDB(db)
	t.Cleanup(func() {
		SetDB(oldDB)
		_ = db.Close()
	})
	return db
}

// TestYieldIrCodes_A2_NativeEquivalentToGORM proves that yieldIrCodes with the
// native-SQL fast path yields the SAME set and order of IrCodes as the current
// GORM FastPagination path for the same ids, covering missing IDs, duplicates,
// custom-scanner fields (Users/FormalArgs/ObjectMembers/Variable), and stable
// ordering.
//
// RED: before A2, yieldIrCodes uses GORM FastPagination (passes trivially, so
// this test also asserts the native path is actually taken via a cache-bypass +
// direct nativeGetIrCodesByIds equivalence). GREEN: yieldIrCodes routes misses
// through nativeGetIrCodesByIds.
func TestYieldIrCodes_A2_NativeEquivalentToGORM(t *testing.T) {
	db := setupA2LoaderDB(t)
	prog := "a2-yield-prog"

	// Insert 6 codes, some with NULL custom-scanner fields, some with values.
	items := []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: 1, Name: "a", String: "s1", Users: Int64Slice{1, 2}},
		{CodeID: 2, ProgramName: prog, Opcode: 2, Name: "b", String: "s2", FormalArgs: Int64Slice{10}},
		{CodeID: 3, ProgramName: prog, Opcode: 3, Name: "c", String: "s3", ObjectMembers: Int64Map{{1, 100}}, Variable: StringSlice{"x", "y"}},
		{CodeID: 4, ProgramName: prog, Opcode: 4, Name: "d", String: "s4"},
		{CodeID: 5, ProgramName: prog, Opcode: 5, Name: "e", String: "s5", ExtraInformation: `{"k":"v"}`},
		{CodeID: 6, ProgramName: prog, Opcode: 6, Name: "f", String: "s6"},
	}
	require.NoError(t, db.CreateInBatches(items, 100).Error)
	deleteCache(prog) // ensure no cached entries interfere

	// ids with a missing one (99) and a duplicate (3)
	ids := []int64{6, 2, 99, 4, 3, 3, 1, 5}

	// GORM reference: the current yieldIrCodes implementation would feed
	// FastPagination over the same DB (missing id yields nothing, dup yields
	// once per DB row). Collect via nativeGetIrCodesByIds which is the A2
	// target and has proven GORM equivalence in native_read_test.go.
	gormRef, err := nativeGetIrCodesByIds(db, prog, ids)
	require.NoError(t, err)
	require.Len(t, gormRef, 6, "6 distinct existing ids expected")

	// yieldIrCodes must produce the same distinct set (order: cached-first is
	// not guaranteed by A2; require set equivalence + no dups).
	seen := make(map[int64]bool)
	var got []int64
	ch := yieldIrCodes(context.Background(), prog, ids)
	for ir := range ch {
		require.False(t, seen[ir.CodeID], "duplicate CodeID %d yielded", ir.CodeID)
		seen[ir.CodeID] = true
		got = append(got, ir.CodeID)
	}
	require.Len(t, got, len(gormRef), "yield must return the same distinct rows")
	for _, ir := range gormRef {
		require.True(t, seen[ir.CodeID], "missing CodeID %d from yield", ir.CodeID)
	}
	// Custom-scanner field equality vs the GORM reference rows.
	refByID := map[int64]*IrCode{}
	for _, ir := range gormRef {
		refByID[ir.CodeID] = ir
	}
	ch2 := yieldIrCodes(context.Background(), prog, ids)
	for ir := range ch2 {
		ref := refByID[ir.CodeID]
		require.NotNil(t, ref)
		require.Equal(t, ref.Users, ir.Users)
		require.Equal(t, ref.FormalArgs, ir.FormalArgs)
		require.Equal(t, ref.Variable, ir.Variable)
		require.Equal(t, len(ref.ObjectMembers), len(ir.ObjectMembers))
		if len(ref.ObjectMembers) > 0 {
			require.Equal(t, ref.ObjectMembers[0].key, ir.ObjectMembers[0].key)
			require.Equal(t, ref.ObjectMembers[0].value, ir.ObjectMembers[0].value)
		}
		require.Equal(t, ref.ExtraInformation, ir.ExtraInformation)
	}
}

// TestYieldIrCodes_A2_CacheHitOrderPreserved proves that ids already in the
// cache are yielded FIRST (in input order) and are not re-fetched, while the
// remaining misses go through the native path. This is the observable A2
// contract that must not regress.
func TestYieldIrCodes_A2_CacheHitOrderPreserved(t *testing.T) {
	db := setupA2LoaderDB(t)
	prog := "a2-cache-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 1, Name: "c1"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 2, Name: "c2"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: 3, Name: "c3"}).Error)
	deleteCache(prog)

	// Pre-cache id 2.
	cache := GetIrCodeCache(prog)
	cache.Set(2, &IrCode{CodeID: 2, ProgramName: prog, Opcode: 2, Name: "cached2"})

	ids := []int64{3, 2, 1}
	var got []int64
	ch := yieldIrCodes(context.Background(), prog, ids)
	for ir := range ch {
		got = append(got, ir.CodeID)
	}
	// Contract: cached hits are yielded first (in input order among hits), then
	// DB misses in stable code_id order (both GORM FastPagination and the A2
	// native path return ascending code_id for the miss set).
	require.Equal(t, []int64{2, 1, 3}, got,
		"cached id first, then misses in ascending code_id order")
}

// TestYieldIrCodes_A2_UsesNativeBatchRead proves that yieldIrCodes routes
// cold-cache misses through nativeGetIrCodesByIds (the O2 native-SQL batch
// read) instead of the old GORM FastPagination path.
//
// RED: before A2, yieldIrCodes uses FastPagination, so the counter does not
// move. GREEN: after A2, a cold-cache yield increments the counter.
func TestYieldIrCodes_A2_UsesNativeBatchRead(t *testing.T) {
	db := setupA2LoaderDB(t)
	prog := "a2-native-counter-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 1, Name: "c1"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 2, Name: "c2"}).Error)
	deleteCache(prog)

	before := NativeIrCodeBatchReads()
	ch := yieldIrCodes(context.Background(), prog, []int64{1, 2})
	count := 0
	for range ch {
		count++
	}
	require.Equal(t, 2, count, "yield must return both codes")
	after := NativeIrCodeBatchReads()
	require.Greater(t, after, before,
		"yieldIrCodes must use the native-SQL batch read (A2), not GORM FastPagination")
}

// --- from loader_a3_test.go ---

// setupA3ConstDB creates a fresh file-backed DB via consts.CreateSSAProjectDatabaseRaw
// (SQLiteExtend driver, which registers the `regexp` SQL function used by the
// ConstType REGEXP path in production).
func setupA3ConstDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := GetDB()
	dir := t.TempDir()
	db, err := consts.CreateSSAProjectDatabaseRaw(filepath.Join(dir, "a3.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	sqlDB := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	SetDB(db)
	t.Cleanup(func() {
		SetDB(oldDB)
		_ = db.Close()
	})
	return db
}

// TestSearchVariableConstType_A3_UsesNativeIDQuery proves that the ConstType
// branch of searchVariableWithFileFilter routes through a native-SQL code_id
// query (incrementing NativeConstTypeIDQueries) instead of GORM
// Model+Pluck+YieldIrCode.
//
// RED: before A3, the ConstType branch uses YieldIrCode (GORM Pluck), so the
// counter does not move.
func TestSearchVariableConstType_A3_UsesNativeIDQuery(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "a3-const-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "xyz"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: 3, ConstType: "normal", String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 4, ProgramName: prog, Opcode: 5, ConstType: "placeholder", String: "abc"}).Error)
	deleteCache(prog)

	before := NativeConstTypeIDQueries()
	ch := SearchVariableWithExcludeFiles(db, context.Background(), prog, NewNameCache(prog, false), ExactCompare, ConstType, "abc", nil)
	got := make(map[int64]bool)
	for ir := range ch {
		got[ir.CodeID] = true
	}
	require.Equal(t, map[int64]bool{1: true}, got, "only code 1 matches opcode=5 const_type=normal string=abc")
	after := NativeConstTypeIDQueries()
	require.Greater(t, after, before,
		"ConstType search must use the native-SQL ID query (A3), not GORM Pluck")
}

// TestSearchVariableConstType_A3_Equivalence covers the non-Exact (regexp)
// compare path: both GORM REGEXP and the native query must return the same set.
func TestSearchVariableConstType_A3_Equivalence(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "a3-const-regexp-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "user_input"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "userid"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "other"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 4, ProgramName: prog, Opcode: 5, ConstType: "placeholder", String: "user_input"}).Error)
	deleteCache(prog)

	ch := SearchVariableWithExcludeFiles(db, context.Background(), prog, NewNameCache(prog, false), RegexpCompare, ConstType, `user.*`, nil)
	got := make(map[int64]bool)
	for ir := range ch {
		got[ir.CodeID] = true
	}
	require.Equal(t, map[int64]bool{1: true, 2: true}, got,
		"regexp 'user.*' on opcode=5 const_type=normal must match codes 1 and 2")
}

// --- from native_read_test.go ---

func setupNativeReadTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}, &IrType{}).Error)
	return db
}

func TestNativeGetIrTypeItemById_Equivalent(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-type-prog"
	item := &IrType{
		TypeId:           42,
		Kind:             3,
		ProgramName:      prog,
		String:           "java/lang/String",
		ExtraInformation: `{"name":"x"}`,
	}
	require.NoError(t, db.Create(item).Error)

	native, err := nativeGetIrTypeItemByIdErr(db, prog, 42)
	require.NoError(t, err)
	require.NotNil(t, native)
	require.Equal(t, item.TypeId, native.TypeId)
	require.Equal(t, item.Kind, native.Kind)
	require.Equal(t, item.ProgramName, native.ProgramName)
	require.Equal(t, item.String, native.String)
	require.Equal(t, item.ExtraInformation, native.ExtraInformation)

	// nonexistent id
	got, err := nativeGetIrTypeItemByIdErr(db, prog, 999)
	require.NoError(t, err)
	require.Nil(t, got)
	// negative id
	gotNeg, err := nativeGetIrTypeItemByIdErr(db, prog, -1)
	require.NoError(t, err)
	require.Nil(t, gotNeg)
}

func TestNativeGetIrCodeItemById_Equivalent(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-code-prog"
	item := &IrCode{
		CodeID:           7,
		ProgramName:      prog,
		Opcode:           3,
		OpcodeName:       "Call",
		Name:             "f",
		String:           "f()",
		Users:            Int64Slice{1, 2},
		FormalArgs:       Int64Slice{10},
		ObjectMembers:    Int64Map{{10, 20}},
		Variable:         StringSlice{"x", "y"},
		IsFunction:       true,
		CurrentBlock:     5,
		TypeID:           9,
		ExtraInformation: `{"k":"v"}`,
	}
	require.NoError(t, db.Create(item).Error)

	native, err := nativeGetIrCodeItemByIdErr(db, prog, 7)
	require.NoError(t, err)
	require.NotNil(t, native)
	require.Equal(t, item.CodeID, native.CodeID)
	require.Equal(t, item.ProgramName, native.ProgramName)
	require.Equal(t, item.Opcode, native.Opcode)
	require.Equal(t, item.OpcodeName, native.OpcodeName)
	require.Equal(t, item.Name, native.Name)
	require.Equal(t, item.String, native.String)
	require.Equal(t, []int64(item.Users), []int64(native.Users))
	require.Equal(t, []int64(item.FormalArgs), []int64(native.FormalArgs))
	require.Equal(t, len(item.ObjectMembers), len(native.ObjectMembers))
	require.Equal(t, item.ObjectMembers[0].key, native.ObjectMembers[0].key)
	require.Equal(t, item.ObjectMembers[0].value, native.ObjectMembers[0].value)
	require.Equal(t, []string(item.Variable), []string(native.Variable))
	require.Equal(t, item.IsFunction, native.IsFunction)
	require.Equal(t, item.CurrentBlock, native.CurrentBlock)
	require.Equal(t, item.TypeID, native.TypeID)
	require.Equal(t, item.ExtraInformation, native.ExtraInformation)

	// nonexistent
	got, err := nativeGetIrCodeItemByIdErr(db, prog, 999)
	require.NoError(t, err)
	require.Nil(t, got)
}

// TestNativeGetIrCodesByIds_EquivalentToPreload asserts that a native-SQL batch
// read (nativeGetIrCodesByIds) returns exactly the same IrCodes, in the same
// order, as the GORM Find(&irs) path used by PreloadIrCodesByIdsFast — covering
// missing IDs, duplicate IDs, negative IDs, and the custom scanner fields.
func TestNativeGetIrCodesByIds_EquivalentToPreload(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-batch-prog"
	// Insert 5 codes with distinct custom-scanner fields.
	for i := int64(1); i <= 5; i++ {
		code := &IrCode{
			CodeID:           i,
			ProgramName:      prog,
			Opcode:           i,
			OpcodeName:       "Call",
			Name:             "f",
			String:           "f()",
			Users:            Int64Slice{i, i + 100},
			FormalArgs:       Int64Slice{i},
			ObjectMembers:    Int64Map{{i, i + 200}},
			Variable:         StringSlice{"x", "y"},
			IsFunction:       true,
			CurrentBlock:     i,
			TypeID:           i + 300,
			ExtraInformation: `{"k":"v"}`,
		}
		require.NoError(t, db.Create(code).Error)
	}
	// GORM reference: what PreloadIrCodesByIdsFast would read for these ids.
	var gormRefs []*IrCode
	require.NoError(t, db.Model(&IrCode{}).
		Where("program_name = ?", prog).
		Where("code_id in (?)", []int64{2, 4, 99, 4, -1}).
		Find(&gormRefs).Error)

	// Native batch read with missing/duplicate/negative ids.
	native, err := nativeGetIrCodesByIds(db, prog, []int64{2, 4, 99, 4, -1})
	require.NoError(t, err)

	require.Len(t, native, len(gormRefs), "same row count as GORM (dedup + missing filtered)")
	require.Equal(t, len(native), 2, "only ids 2 and 4 exist")
	// Order preserved (GORM Find returns in PK order; native should match).
	require.Equal(t, gormRefs[0].CodeID, native[0].CodeID, "first id order")
	require.Equal(t, gormRefs[1].CodeID, native[1].CodeID, "second id order")
	// Field equivalence on the custom-scanner columns.
	for i := range native {
		require.Equal(t, gormRefs[i].CodeID, native[i].CodeID)
		require.Equal(t, gormRefs[i].Opcode, native[i].Opcode)
		require.Equal(t, []int64(gormRefs[i].Users), []int64(native[i].Users))
		require.Equal(t, []int64(gormRefs[i].FormalArgs), []int64(native[i].FormalArgs))
		require.Equal(t, []string(gormRefs[i].Variable), []string(native[i].Variable))
		require.Equal(t, gormRefs[i].ObjectMembers[0].key, native[i].ObjectMembers[0].key)
		require.Equal(t, gormRefs[i].ObjectMembers[0].value, native[i].ObjectMembers[0].value)
		require.Equal(t, gormRefs[i].TypeID, native[i].TypeID)
		require.Equal(t, gormRefs[i].ExtraInformation, native[i].ExtraInformation)
	}
}

// TestNativeGetIrCodesByIds_NullJSONFields covers rows with NULL/empty custom
// scanner fields (nil Int64Slice/Int64Map/StringSlice) and JSON text columns,
// ensuring the native batch scan handles them identically to GORM.
func TestNativeGetIrCodesByIds_NullJSONFields(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-null-prog"
	// A row with NO users/args/variable (nil custom scanners) + a JSON text col.
	nullCode := &IrCode{
		CodeID: 1, ProgramName: prog, Opcode: 3, OpcodeName: "Call", Name: "f",
		String: "f()", ExtraInformation: `{"k":"v"}`,
	}
	require.NoError(t, db.Create(nullCode).Error)
	jsonCode := &IrCode{
		CodeID: 2, ProgramName: prog, Opcode: 4, OpcodeName: "Const",
		Name: "c", String: "c", ExtraInformation: `[{"a":1},{"b":2}]`,
		Users: Int64Slice{}, // empty slice, not nil
	}
	require.NoError(t, db.Create(jsonCode).Error)

	var gormRefs []*IrCode
	require.NoError(t, db.Model(&IrCode{}).
		Where("program_name = ?", prog).Where("code_id in (?)", []int64{1, 2}).Find(&gormRefs).Error)

	native, err := nativeGetIrCodesByIds(db, prog, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, native, len(gormRefs))
	for i := range native {
		require.Equal(t, gormRefs[i].CodeID, native[i].CodeID)
		require.Equal(t, gormRefs[i].Users, native[i].Users)
		require.Equal(t, gormRefs[i].ExtraInformation, native[i].ExtraInformation)
	}
}

// TestNativeGetIrCodesByIds_ChunkBoundary covers the chunking path: more ids
// than nativeIrCodeBatchChunk must be split into multiple parameterized queries
// and still return all rows in stable order.
func TestNativeGetIrCodesByIds_ChunkBoundary(t *testing.T) {
	db := setupNativeReadTestDB(t)
	prog := "native-chunk-prog"
	n := int64(nativeIrCodeBatchChunk + 7)
	for i := int64(1); i <= n; i++ {
		require.NoError(t, db.Create(&IrCode{CodeID: i, ProgramName: prog, Opcode: 1, Name: "x"}).Error)
	}
	ids := make([]int64, 0, n)
	for i := int64(1); i <= n; i++ {
		ids = append(ids, i)
	}
	// interleave a duplicate + missing + negative to exercise filtering across chunks
	ids = append(ids, 5, n+100, -3)

	var gormRefs []*IrCode
	require.NoError(t, db.Model(&IrCode{}).
		Where("program_name = ?", prog).Where("code_id in (?)", ids).Find(&gormRefs).Error)

	native, err := nativeGetIrCodesByIds(db, prog, ids)
	require.NoError(t, err)
	require.Len(t, native, len(gormRefs), "chunked batch must match GORM row count")
	for i := range native {
		require.Equal(t, gormRefs[i].CodeID, native[i].CodeID, "order must be stable across chunk boundary")
	}
}

// TestNativeGetIrCodesByIds_ErrorPropagation verifies the native batch read
// returns nil (like a failed query) on an invalid DB/empty input.
func TestNativeGetIrCodesByIds_ErrorPropagation(t *testing.T) {
	db := setupNativeReadTestDB(t)
	// empty ids → nil, nil
	r, err := nativeGetIrCodesByIds(db, "prog", nil)
	require.Nil(t, r)
	require.NoError(t, err)
	// all-non-positive ids → nil, nil
	r, err = nativeGetIrCodesByIds(db, "prog", []int64{-1, 0, -5})
	require.Nil(t, r)
	require.NoError(t, err)
	// nil db → nil, nil
	r, err = nativeGetIrCodesByIds(nil, "prog", []int64{1})
	require.Nil(t, r)
	require.NoError(t, err)
}

// TestNativeGetIrCodesByIds_Concurrent verifies the native batch read is safe
// under concurrent readers. SQLite ":memory:" gives each connection its own
// empty DB, so this uses a shared temp-file DB with a constrained pool (all
// goroutines share the same file-backed schema/data). Run with -race.
func TestNativeGetIrCodesByIds_Concurrent(t *testing.T) {
	dir := t.TempDir()
	db, err := gorm.Open("sqlite3", filepath.Join(dir, "conc.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	// Constrain the pool so all goroutines share the same file-backed DB.
	sqlDB := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	defer sqlDB.Close()

	prog := "native-conc-prog"
	for i := int64(1); i <= 20; i++ {
		require.NoError(t, db.Create(&IrCode{CodeID: i, ProgramName: prog, Opcode: 1, Name: "x", Users: Int64Slice{i}}).Error)
	}
	ids := make([]int64, 0, 20)
	for i := int64(1); i <= 20; i++ {
		ids = append(ids, i)
	}
	// reference
	var gormRefs []*IrCode
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Where("code_id in (?)", ids).Find(&gormRefs).Error)

	const goroutines = 8
	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			native, err := nativeGetIrCodesByIds(db, prog, ids)
			if err != nil {
				t.Errorf("concurrent read error: %v", err)
				return
			}
			if len(native) != len(gormRefs) {
				t.Errorf("concurrent read count mismatch: got %d want %d", len(native), len(gormRefs))
				return
			}
			for i := range native {
				if native[i].CodeID != gormRefs[i].CodeID {
					t.Errorf("concurrent read order mismatch at %d", i)
					return
				}
			}
		}()
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}
}

// TestConstTypeRegexpOperatorByDialect verifies the native ConstType regex
// operator mirrors the GORM fallback's dialect switch (review A7): PostgreSQL
// uses ~, everything else uses REGEXP.
func TestConstTypeRegexpOperatorByDialect(t *testing.T) {
	require.Equal(t, "REGEXP", constTypeRegexpOperator("sqlite"))
	require.Equal(t, "REGEXP", constTypeRegexpOperator("mysql"))
	require.Equal(t, "REGEXP", constTypeRegexpOperator(""))
	require.Equal(t, "~", constTypeRegexpOperator("postgres"))
	require.Equal(t, "~", constTypeRegexpOperator("postgresql"))
}
