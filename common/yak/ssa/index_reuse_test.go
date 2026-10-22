package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestIndexRelationshipsDeduplicateAcrossFlushes(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	b := p.GetAndCreateFunctionBuilder("", "index-reuse")
	v := b.EmitUndefined("value")
	s := p.Cache.indexes
	for i := 0; i < 10; i++ {
		s.AddVariable("value", v)
		s.AddVariable("object.field", v)
		s.AddClassInstance("Class", v)
	}
	require.NoError(t, s.Flush())
	for i := 0; i < 10; i++ {
		s.AddVariable("value", v)
		s.AddVariable("object.field", v)
		s.AddClassInstance("Class", v)
	}
	require.NoError(t, s.Flush())
	var rows []ssadb.IrIndex
	require.NoError(t, ssadb.GetDB().Where("program_name = ?", p.Name).Find(&rows).Error)
	require.Equal(t, 3, len(rows), "exact relationships must be enqueued only once, including across batches")
	ids, _ := s.variable.Get("value")
	require.Equal(t, []int64{v.GetId()}, ids, "memory indexes must also store one relationship")
	s.RemoveVariable("value", v)
	s.AddVariable("value", v)
	ids, _ = s.variable.Get("value")
	require.Equal(t, []int64{v.GetId()}, ids, "removing and re-adding a memory index must work")
	b.Finish()
}

func TestIndexReusePreservesEveryRelationshipField(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	s := p.Cache.indexes
	zero, one := int64(0), int64(1)
	rows := []*ssadb.IrIndex{
		{ProgramName: p.Name, ValueID: 10},
		{ProgramName: p.Name, ValueID: 11},
		{ProgramName: p.Name, ValueID: 10, VariableID: &zero},
		{ProgramName: p.Name, ValueID: 10, ClassID: &zero},
		{ProgramName: p.Name, ValueID: 10, FieldID: &zero},
		{ProgramName: p.Name, ValueID: 10, OwnerValueID: &zero},
		{ProgramName: p.Name, ValueID: 10, VariableID: &one},
		{ProgramName: p.Name, ValueID: 10, ScopeName: "other"},
		{ProgramName: p.Name, ValueID: 10, VersionID: 1},
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, row := range rows {
				s.saveIndexDedup(row)
			}
		}()
	}
	wg.Wait()
	require.NoError(t, s.Flush())
	var got []ssadb.IrIndex
	require.NoError(t, ssadb.GetDB().Where("program_name = ?", p.Name).Find(&got).Error)
	require.Equal(t, len(rows), len(got), "NULL, zero, scope, version and each ID belong to the identity")
}

func TestIndexSaverPropagatesWriteFailure(t *testing.T) {
	p := newResidentDeltaProgram(t)
	store := p.Cache.indexes
	defer func() {
		p.Cache.CloseWithoutSave()
		require.Error(t, store.Close(), "the failed saver must also report its recorded error on close")
	}()
	db := ssadb.GetDB()
	// The test program has no index records yet, so this trigger affects
	// only its newly enqueued relationship and is removed before cleanup.
	const trigger = "reject_index_reuse_write"
	require.NoError(t, db.Exec("CREATE TRIGGER "+trigger+" BEFORE INSERT ON ir_indices_v1 BEGIN SELECT RAISE(ABORT, 'index write rejected'); END").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS "+trigger).Error) })
	p.Cache.indexes.saveIndexDedup(&ssadb.IrIndex{ProgramName: p.Name, ValueID: 10})
	require.Error(t, p.Cache.indexes.Flush(), "the saver must report failed SQL instead of marking the run successful")
}
