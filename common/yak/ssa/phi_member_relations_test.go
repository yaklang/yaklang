package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMethodPhiRetainsCommonReceiver(t *testing.T) {
	_, b := newTestBuilder(t)
	object := emitObject(b, "receiver")
	methodType := NewFunctionTypeDefine("debug", nil, nil, false)
	methodType.SetIsMethod(true, object.GetType())
	var branches []Value
	for range 2 {
		method := b.EmitUndefined("branch-debug")
		method.SetType(methodType)
		AddObjectKeyPair(method, object, b.EmitConstInstPlaceholder("debug"))
		branches = append(branches, method)
	}
	phi := generatePhi(b, b.CurrentBlock, nil)("merged-debug", branches)
	require.NotNil(t, phi)
	require.NotNil(t, GetLatestObject(phi))
	require.Same(t, object, GetLatestObject(phi))
	call := b.EmitCall(b.NewCall(phi, nil))
	require.NotNil(t, call)
	require.Equal(t, []int64{object.GetId()}, call.Args, "a merged bound method must keep its receiver argument")
}

func TestMethodPhiRetainsReceiverThroughNestedJoins(t *testing.T) {
	_, b := newTestBuilder(t)
	object := emitObject(b, "receiver")
	method := b.ReadMemberCallMethod(object, b.EmitConstInstPlaceholder("debug"))
	inner := b.EmitPhi("inner", []Value{method, method})
	inner.SetType(method.GetType())
	outer := generatePhi(b, b.CurrentBlock, nil)("outer", []Value{inner, method})
	require.NotNil(t, GetLatestObject(outer))
	require.Equal(t, object.GetId(), GetLatestObject(outer).GetId())
}

func TestMethodPhiDoesNotBindAnUnknownBranch(t *testing.T) {
	_, b := newTestBuilder(t)
	object := emitObject(b, "receiver")
	method := b.ReadMemberCallMethod(object, b.EmitConstInstPlaceholder("debug"))
	unknown := b.EmitUndefined("unknown")
	phi := generatePhi(b, b.CurrentBlock, nil)("merged", []Value{method, unknown})
	require.Nil(t, GetLatestObject(phi))
}

func TestMethodPhiDoesNotChooseBetweenDifferentReceivers(t *testing.T) {
	_, b := newTestBuilder(t)
	left, right := emitObject(b, "left"), emitObject(b, "right")
	methodType := NewFunctionTypeDefine("debug", nil, nil, false)
	methodType.SetIsMethod(true, left.GetType())
	var branches []Value
	for _, object := range []Value{left, right} {
		method := b.EmitUndefined("debug")
		method.SetType(methodType)
		AddObjectKeyPair(method, object, b.EmitConstInstPlaceholder("debug"))
		branches = append(branches, method)
	}
	phi := generatePhi(b, b.CurrentBlock, nil)("merged-debug", branches)
	require.NotNil(t, phi)
	require.Nil(t, GetLatestObject(phi), "different bound receivers need a receiver merge, not an arbitrary last value")
}
