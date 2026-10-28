package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestPlaceholderConstantsReuseWithinSourceOccurrence(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	b := p.GetAndCreateFunctionBuilder("", "placeholder-reuse")
	ed := memedit.NewMemEditor("object.field; other.field")
	b.CurrentRange = memedit.NewRangeFromOffsets(ed, 0, 12)
	first := b.EmitConstInstPlaceholder("field")
	before := len(b.CurrentBlock.Insts)
	for i := 0; i < 100; i++ {
		b.CurrentRange = memedit.NewRangeFromOffsets(ed, 0, 12)
		require.Same(t, first, b.EmitConstInstPlaceholder("field"))
	}
	require.Len(t, b.CurrentBlock.Insts, before, "reuse must not append the same instruction repeatedly")
	require.NotSame(t, first, b.EmitConstInst("field"), "a source literal has a separate identity from an internal key")
	first.SetExtern(true)
	require.NotSame(t, first, b.EmitConstInstPlaceholder("field"), "mutated placeholders must not be reused")
	b.CurrentRange = memedit.NewRangeFromOffsets(ed, 14, 25)
	other := b.EmitConstInstPlaceholder("field")
	require.NotEqual(t, first.GetId(), other.GetId(), "different source occurrences must retain precise ranges")
	require.Equal(t, 0, first.GetRange().GetStartOffset())
	require.Equal(t, 14, other.GetRange().GetStartOffset())
	otherBuilder := p.GetAndCreateFunctionBuilder("", "other")
	otherBuilder.CurrentRange = memedit.NewRangeFromOffsets(ed, 0, 12)
	require.NotEqual(t, first.GetId(), otherBuilder.EmitConstInstPlaceholder("field").GetId(), "owners must remain distinct")
	otherBuilder.Finish()
	b.Finish()
}

func TestProgramLineCountCountsSourceOnce(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	ed := memedit.NewMemEditor("package main\nfunc main() {}\n")
	ed.SetUrl("main.go")
	for i := 0; i < 3; i++ {
		p.PushEditor(ed)
		p.PopEditor(true)
	}
	require.Equal(t, ed.GetLineCount(), p.LineCount, "lazy import linking must not count the same source a second time")
}

func TestReloadedConstantKeepsClassificationAndUnary(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	b := p.GetAndCreateFunctionBuilder("", "const-codec")
	placeholder := b.EmitConstInstPlaceholder("field")
	normal := b.EmitConstInstWithUnary(42, -1)
	b.Finish()
	require.NoError(t, p.Cache.instructions.residentCache.Flush(false))
	for _, v := range []*ConstInst{placeholder, normal} {
		loaded, ok := ToConstInst(p.Cache.GetInstruction(v.GetId()))
		require.True(t, ok)
		require.Equal(t, v.ConstType, loaded.ConstType)
		require.Equal(t, v.Unary, loaded.Unary)
		loaded.SetExtern(true)
	}
	require.NoError(t, p.Cache.instructions.residentCache.Flush(false))
	row := ssadb.GetIrCodeItemById(ssadb.GetDB(), p.Name, placeholder.GetId())
	require.Equal(t, string(ConstTypePlaceholder), row.ConstType, "late updates must not erase classification")
}
