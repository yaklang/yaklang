package ssadb

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupDedupeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "dedupe.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&IrTypeID{}, &IrTypePayload{},
		&IrCode{}, &IrIndex{}, &IrNamePool{}, &IrSource{}, &IrOffset{}).Error)
	require.NoError(t, ensureIrTypeDedupeIndexes(db))
	return db
}

// loadTypePayload returns the stored body for one type id.
func loadTypePayload(t *testing.T, db *gorm.DB, program string, typeID uint64) *IrTypePayload {
	t.Helper()
	var id IrTypeID
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, typeID).First(&id).Error)
	var payload IrTypePayload
	require.NoError(t, db.Where("id = ?", id.PayloadID).First(&payload).Error)
	return &payload
}

func countRows(t *testing.T, db *gorm.DB, model any, where string, args ...any) int64 {
	t.Helper()
	query := db.Model(model)
	if where != "" {
		query = query.Where(where, args...)
	}
	var count int64
	require.NoError(t, query.Count(&count).Error)
	return count
}

func dedupeItem(program string, id int64, text string) *IrType {
	return &IrType{
		ProgramName:      program,
		TypeId:           uint64(id),
		Kind:             1,
		String:           text,
		ExtraInformation: fmt.Sprintf(`{"name":%q}`, text),
	}
}

// A read must survive a handle that already carries model scopes. GORM's Raw
// path on such a handle rewrites FROM into "FROM <model>, <query>", which is a
// syntax error; that turned every type read into nil (and the ARTEX scan lost
// 97 reflected-XSS risks) before the read used database/sql directly.
func TestIrTypeDedupeReadPathSurvivesScopedHandle(t *testing.T) {
	db := setupDedupeDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{dedupeItem("scoped", 7, "number")}))

	scoped := db.Model(&IrTypeID{}).Where("program_name = ?", "scoped")
	loaded := GetIrTypeItemById(scoped, "scoped", 7)
	require.NotNil(t, loaded, "a scoped handle must still resolve the deduped type")
	require.Equal(t, uint64(7), loaded.TypeId)
	require.Equal(t, "number", loaded.String)

	var count int64
	require.NoError(t, scoped.Count(&count).Error)
	require.Equal(t, int64(1), count, "the caller's scope must not be consumed by the read")
}

// The point of the layout: 500 ids that share a payload store one payload.
func TestIrTypeDedupeStoresPayloadOnce(t *testing.T) {
	db := setupDedupeDB(t)
	items := make([]*IrType, 0, 500)
	for id := 1; id <= 500; id++ {
		items = append(items, dedupeItem("p", int64(id), "number"))
	}
	require.NoError(t, SaveIrTypeBatch(db, items))

	require.Equal(t, int64(1), countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"))
	require.Equal(t, int64(500), countRows(t, db, &IrTypeID{}, "program_name = ?", "p"))
	require.Equal(t, int64(1), countRows(t, db, &IrTypePayload{}, ""), "one payload serves every id")

	for _, id := range []int64{1, 250, 500} {
		got := GetIrTypeItemById(db, "p", id)
		require.NotNil(t, got)
		require.Equal(t, "number", got.String)
		require.Equal(t, uint64(id), got.TypeId)
		require.Equal(t, `{"name":"number"}`, got.ExtraInformation)
	}
}

// Duplicate IDs in a batch must follow the established last-write-wins rule.
func TestIrTypeDedupeDuplicateIDUsesLastPayload(t *testing.T) {
	db := setupDedupeDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{
		dedupeItem("p", 11, "first"),
		dedupeItem("p", 11, "last"),
	}))
	require.Equal(t, int64(1), countRows(t, db, &IrTypeID{}, "program_name = ? AND type_id = ?", "p", 11))
	got := GetIrTypeItemById(db, "p", 11)
	require.NotNil(t, got)
	require.Equal(t, "last", got.String)
}

// Transaction-scoped readers and writers use the same schema and visibility.
func TestIrTypeDedupeWorksInsideTransaction(t *testing.T) {
	db := setupDedupeDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := SaveIrTypeBatch(tx, []*IrType{dedupeItem("p", 21, "inside")}); err != nil {
			return err
		}
		got := GetIrTypeItemById(tx, "p", 21)
		if got == nil || got.String != "inside" {
			return fmt.Errorf("transaction cannot read its deduped type: %#v", got)
		}
		return nil
	}))
	require.Equal(t, "inside", GetIrTypeItemById(db, "p", 21).String)
}

// A payload that is not stored yet must be added, and an id that changes
// payload must move alone: other ids keep reading the old body.
func TestIrTypeDedupeTracksDistinctPayloads(t *testing.T) {
	db := setupDedupeDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{
		dedupeItem("p", 1, "number"),
		dedupeItem("p", 2, "string"),
		dedupeItem("p", 3, "number"),
	}))
	require.Equal(t, int64(2), countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"))

	require.NoError(t, SaveIrTypeBatch(db, []*IrType{dedupeItem("p", 1, "boolean")}))
	require.Equal(t, int64(3), countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"))

	require.Equal(t, "boolean", GetIrTypeItemById(db, "p", 1).String)
	require.Equal(t, "string", GetIrTypeItemById(db, "p", 2).String)
	require.Equal(t, "number", GetIrTypeItemById(db, "p", 3).String)
}

// Re-flushing unchanged types must not rewrite payload rows: that is what made
// every batch pay the full type body again.
func TestIrTypeDedupeDoesNotRewriteUnchangedPayloads(t *testing.T) {
	db := setupDedupeDB(t)
	first := []*IrType{dedupeItem("p", 1, "number"), dedupeItem("p", 2, "string")}
	require.NoError(t, SaveIrTypeBatch(db, first))
	require.Equal(t, int64(2), countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"))

	var before []uint
	require.NoError(t, db.Model(&IrTypePayload{}).Where("program_name = ?", "p").Pluck("id", &before).Error)

	second := []*IrType{dedupeItem("p", 3, "number"), dedupeItem("p", 4, "string")}
	require.NoError(t, SaveIrTypeBatch(db, second))
	require.Equal(t, int64(2), countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"), "payloads must be reused")

	var after []uint
	require.NoError(t, db.Model(&IrTypePayload{}).Where("program_name = ?", "p").Pluck("id", &after).Error)
	require.ElementsMatch(t, before, after, "payload row ids must stay stable")
	require.Equal(t, int64(4), countRows(t, db, &IrTypeID{}, "program_name = ?", "p"))
}

// Deleting a program must take its dedupe rows with it, or a recompile would
// read the previous run's payloads.
func TestDeleteProgramClearsTypeDedupeRows(t *testing.T) {
	db := setupDedupeDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{dedupeItem("p", 1, "number"), dedupeItem("p", 2, "string")}))
	require.NoError(t, deleteProgramCodeOnly(db, "p"))
	require.Zero(t, countRows(t, db, &IrTypeID{}, "program_name = ?", "p"))
	require.Zero(t, countRows(t, db, &IrTypePayload{}, "program_name = ?", "p"))
}
