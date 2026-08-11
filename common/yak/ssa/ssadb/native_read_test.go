package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

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

	native := nativeGetIrTypeItemById(db, prog, 42)
	require.NotNil(t, native)
	require.Equal(t, item.TypeId, native.TypeId)
	require.Equal(t, item.Kind, native.Kind)
	require.Equal(t, item.ProgramName, native.ProgramName)
	require.Equal(t, item.String, native.String)
	require.Equal(t, item.ExtraInformation, native.ExtraInformation)

	// nonexistent id
	require.Nil(t, nativeGetIrTypeItemById(db, prog, 999))
	// negative id
	require.Nil(t, nativeGetIrTypeItemById(db, prog, -1))
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

	native := nativeGetIrCodeItemById(db, prog, 7)
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
	require.Nil(t, nativeGetIrCodeItemById(db, prog, 999))
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
