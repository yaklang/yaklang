package ssa

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestTypePersistenceBlueprintJSONAndMutableFields(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "types.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&ssadb.IrType{}).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_ir_types_program_type ON ir_types (program_name, type_id)`).Error)
	store := &typeStore{mode: ProgramCacheDBWrite, db: db, programName: "test",
		saveSize: 512, resident: utils.NewSafeMapWithKey[int64, Type]()}
	bp := NewBlueprint("A")
	bp.SetId(1)
	bp.SetKind(BlueprintClass)
	bp.SetFullTypeNames([]string{"pkg.A"})
	parent, iface := NewBlueprint("Parent"), NewBlueprint("Interface")
	parent.SetId(2)
	iface.SetId(3)
	bp.ParentBlueprints, bp.InterfaceBlueprints = []*Blueprint{parent}, []*Blueprint{iface}
	store.remember(bp)
	require.NoError(t, store.flush())
	load := func() *ssadb.IrType {
		r := new(ssadb.IrType)
		require.NoError(t, db.Where("program_name = ? AND type_id = ?", "test", 1).First(r).Error)
		return r
	}
	first := load()
	require.JSONEq(t, `{"name":"A","kind":"class","fullTypeName":["pkg.A"],"parentBlueprints":[2],"interfaceBlueprints":[3],"container":-1}`, first.ExtraInformation)
	require.NoError(t, store.flush())
	require.Equal(t, first.ID, load().ID, "unchanged types must not be rewritten")
	// Neither edit replaces the owning type pointer or calls remember again.
	// Fingerprints must cover contents, not slice/pointer identity.
	bp.SetFullTypeNames([]string{"other.A"})
	parent.SetId(4)
	require.NoError(t, store.flush())
	require.JSONEq(t, `{"name":"A","kind":"class","fullTypeName":["other.A"],"parentBlueprints":[4],"interfaceBlueprints":[3],"container":-1}`, load().ExtraInformation)
}

func TestTypePersistenceFailedWriteCanRetry(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "retry.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	store := &typeStore{mode: ProgramCacheDBWrite, db: db, programName: "retry",
		saveSize: 512, resident: utils.NewSafeMapWithKey[int64, Type]()}
	typ := CreateStringType()
	typ.SetId(1)
	store.remember(typ)
	// Missing table causes the initial write to fail. The next flush must
	// still encode and save the type once storage becomes available.
	require.Error(t, store.flush())
	require.NoError(t, db.AutoMigrate(&ssadb.IrType{}).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_ir_types_program_type ON ir_types (program_name, type_id)`).Error)
	require.NoError(t, store.flush())
	var count int
	require.NoError(t, db.Model(&ssadb.IrType{}).Count(&count).Error)
	require.Equal(t, 1, count)
}

func TestTypePersistenceParallelEncodingMatchesSerial(t *testing.T) {
	snapshots := make([]typePersistenceSnapshot, 2048)
	for i := range snapshots {
		typ := NewFunctionType(fmt.Sprintf("f%d", i), nil, nil, false)
		typ.SetId(int64(i + 1))
		typ.AddFullTypeName("pkg.Function")
		snapshots[i] = snapshotTypePersistence(typ)
	}
	serial := marshalTypeSnapshots("p", snapshots, nil, 1)
	parallel := marshalTypeSnapshots("p", snapshots, nil, 31)
	require.Equal(t, serial, parallel)
}

func TestTypePersistenceSharedPayloadKeepsDistinctIDs(t *testing.T) {
	shared := typePersistenceSnapshot{id: 7, shape: typePersistenceNamed, kind: 3, name: "T", text: "T", fullNames: []string{"pkg.T"}}
	twin := shared
	twin.id = 8
	different := shared
	different.id = 9
	different.name = "U"
	different.text = "U"
	rows := marshalTypeSnapshots("p", []typePersistenceSnapshot{shared, twin, different}, nil, 4)
	require.Equal(t, []uint64{7, 8, 9}, []uint64{rows[0].TypeId, rows[1].TypeId, rows[2].TypeId})
	require.Equal(t, rows[0].ExtraInformation, rows[1].ExtraInformation)
	require.Equal(t, rows[0].String, rows[1].String)
	require.Equal(t, rows[0].Kind, rows[1].Kind)
	require.Equal(t, "U", rows[2].String)
	require.NotEqual(t, rows[0].ExtraInformation, rows[2].ExtraInformation)
	require.NotSame(t, rows[0], rows[1])
	rows[0].ExtraInformation = "mutated"
	require.NotEqual(t, "mutated", rows[1].ExtraInformation)
}

func TestTypePersistenceFingerprintDistinguishesFields(t *testing.T) {
	base := typePersistenceSnapshot{shape: typePersistenceBlueprint, id: 1, name: "A", fullNames: []string{"ab", "c"}}
	for _, mutate := range []func(*typePersistenceSnapshot){
		func(s *typePersistenceSnapshot) { s.fullNames = []string{"a", "bc"} },
		func(s *typePersistenceSnapshot) { s.name = "B" },
		func(s *typePersistenceSnapshot) { s.text = "new" },
		func(s *typePersistenceSnapshot) { s.kind++ },
		func(s *typePersistenceSnapshot) { s.extraKind++ },
		func(s *typePersistenceSnapshot) { s.blueprintKind = "class" },
		func(s *typePersistenceSnapshot) { s.parents = []int64{2} },
		func(s *typePersistenceSnapshot) { s.ifaces = []int64{3} },
		func(s *typePersistenceSnapshot) { s.container = 4 },
	} {
		other := base
		mutate(&other)
		require.NotEqual(t, base.fingerprint(), other.fingerprint())
	}
	nilNames, emptyNames := base, base
	nilNames.fullNames, emptyNames.fullNames = nil, []string{}
	require.NotEqual(t, nilNames.fingerprint(), emptyNames.fingerprint(), "null and [] have different persisted JSON")
}
