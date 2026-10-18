package ssa

import (
	"sync"
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

// One goroutine records closure captures while another assigns those names.
// After "flag" is noted, every later assignment of flag must stay marked, and
// a name that was never captured must not pick up the bit.
func TestCapturedNamesConcurrentNoteAndAssign(t *testing.T) {
	_, builder := newTestBuilder(t)
	seed := builder.EmitConstInst(1)
	noteCapturedByClosure(seed, "flag")

	const n = 128
	flagValues := make([]Value, n)
	otherValues := make([]Value, n)
	flagVars := make([]*Variable, n)
	otherVars := make([]*Variable, n)
	extras := make([]Value, n)
	for i := 0; i < n; i++ {
		flagValues[i] = builder.EmitConstInst(i + 2)
		otherValues[i] = builder.EmitConstInst(i + 1000)
		flagVars[i] = builder.CreateVariable("flag")
		otherVars[i] = builder.CreateVariable("other")
		extras[i] = builder.EmitConstInst(i + 2000)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < n; i++ {
			noteCapturedByClosure(extras[i], "flag")
			noteCapturedByClosure(extras[i], "extra")
		}
	}()
	missed := make([]bool, n)
	polluted := make([]bool, n)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < n; i++ {
			builder.AssignVariable(flagVars[i], flagValues[i])
			missed[i] = !valueCapturedByClosure(flagValues[i])
			builder.AssignVariable(otherVars[i], otherValues[i])
			polluted[i] = valueCapturedByClosure(otherValues[i])
		}
	}()
	close(start)
	wg.Wait()

	for i := 0; i < n; i++ {
		require.Falsef(t, missed[i], "assignment %d of a captured name lost the capture bit", i)
		require.Falsef(t, polluted[i], "assignment %d of an uncaptured name became captured", i)
	}
}
