package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newBlueprintCycleTestProgram(t *testing.T) *Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("blueprint-cycle"))
	require.NoError(t, err)
	return NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 1)
}

// Restored interface blueprints are appended straight to InterfaceBlueprints,
// bypassing the relation guard that rejects cycles at insertion time. Applying
// such a hierarchy used to recurse until the goroutine stack overflowed and the
// whole compile died (coremail's wmsvr.jar reproduced it).
func TestBlueprintApplyStopsOnParentCycle(t *testing.T) {
	prog := newBlueprintCycleTestProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	obj := builder.EmitConstInst(1)

	a := NewBlueprint("cycle.A")
	b := NewBlueprint("cycle.B")
	c := NewBlueprint("cycle.C")
	// Direct assignment mirrors the database-restore path.
	a.ParentBlueprints = []*Blueprint{b}
	b.ParentBlueprints = []*Blueprint{c}
	c.ParentBlueprints = []*Blueprint{a}

	var got Type
	require.NotPanics(t, func() { got = a.Apply(obj) })
	require.NotNil(t, got)
}

func TestBlueprintApplyStopsOnInterfaceCycle(t *testing.T) {
	prog := newBlueprintCycleTestProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	obj := builder.EmitConstInst(1)

	a := NewBlueprint("iface.A")
	b := NewBlueprint("iface.B")
	a.InterfaceBlueprints = []*Blueprint{b}
	b.InterfaceBlueprints = []*Blueprint{a}

	var got Type
	require.NotPanics(t, func() { got = a.Apply(obj) })
	require.NotNil(t, got)
}

// A shared ancestor reached twice through different branches must still work.
func TestBlueprintApplyAllowsDiamondHierarchy(t *testing.T) {
	prog := newBlueprintCycleTestProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	obj := builder.EmitConstInst(1)

	base := NewBlueprint("diamond.Base")
	left := NewBlueprint("diamond.Left")
	right := NewBlueprint("diamond.Right")
	top := NewBlueprint("diamond.Top")
	left.ParentBlueprints = []*Blueprint{base}
	right.ParentBlueprints = []*Blueprint{base}
	top.ParentBlueprints = []*Blueprint{left, right}

	var got Type
	require.NotPanics(t, func() { got = top.Apply(obj) })
	require.NotNil(t, got)
}
