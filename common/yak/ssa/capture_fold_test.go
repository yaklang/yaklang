package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitBinOpKeepsCapturedConst(t *testing.T) {
	_, builder := newTestBuilder(t)

	plain := builder.EmitBinOp(OpEq, builder.EmitConstInst(1), builder.EmitConstInst(1))
	_, folded := ToConstInst(plain)
	require.True(t, folded)

	captured := builder.EmitConstInst(1)
	noteCapturedByClosure(captured, "flag")
	kept := builder.EmitBinOp(OpEq, captured, builder.EmitConstInst(1))
	_, folded = ToConstInst(kept)
	require.False(t, folded)
	_, isBin := ToBinOp(kept)
	require.True(t, isBin)

	capturedBool := builder.EmitConstInst(true)
	noteCapturedByClosure(capturedBool, "flag")
	keptUn := builder.EmitUnOp(OpNot, capturedBool)
	_, folded = ToConstInst(keptUn)
	require.False(t, folded)
}

func TestAssignMarksValueWhenNameAlreadyCaptured(t *testing.T) {
	_, builder := newTestBuilder(t)
	current := builder.EmitConstInst(1)
	noteCapturedByClosure(current, "flag")

	next := builder.EmitConstInst(2)
	variable := builder.CreateVariable("flag")
	builder.AssignVariable(variable, next)
	require.True(t, valueCapturedByClosure(next))
	require.False(t, valueCapturedByClosure(builder.EmitConstInst(3)))
}
