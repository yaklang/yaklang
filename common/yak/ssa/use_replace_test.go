package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func userContains(v Value, id int64) bool {
	for _, user := range v.GetUsers() {
		if user != nil && user.GetId() == id {
			return true
		}
	}
	return false
}

func TestCallReplaceValueKeepsUnrelatedOperands(t *testing.T) {
	_, builder := newTestBuilder(t)

	method := builder.EmitUndefined("callee")
	arg := builder.EmitUndefined("arg")
	other := builder.EmitUndefined("other")
	member := builder.EmitUndefined("member")
	untouchedMember := builder.EmitUndefined("untouched-member")
	bound := builder.EmitUndefined("bound")
	to := builder.EmitUndefined("replacement")

	call := builder.NewCall(method, []Value{arg, other})
	call.ArgMember = []int64{member.GetId(), untouchedMember.GetId()}
	call.Binding = map[string]int64{"x": bound.GetId()}
	builder.EmitCall(call)

	gotIDs := make(map[int64]struct{})
	for _, val := range call.GetValues() {
		gotIDs[val.GetId()] = struct{}{}
	}
	require.Contains(t, gotIDs, method.GetId())
	require.Contains(t, gotIDs, arg.GetId())
	require.Contains(t, gotIDs, other.GetId())
	require.Contains(t, gotIDs, bound.GetId())
	require.NotContains(t, gotIDs, member.GetId())
	require.NotContains(t, gotIDs, untouchedMember.GetId())

	ReplaceAllValue(arg, to)
	require.Equal(t, []int64{to.GetId(), other.GetId()}, call.Args)
	require.Equal(t, []int64{member.GetId(), untouchedMember.GetId()}, call.ArgMember)
	require.Equal(t, bound.GetId(), call.Binding["x"])
	require.Equal(t, method.GetId(), call.Method)
	require.False(t, userContains(arg, call.GetId()))
	require.True(t, userContains(to, call.GetId()))

	ReplaceAllValue(member, to)
	require.Equal(t, []int64{to.GetId(), untouchedMember.GetId()}, call.ArgMember)
	require.Equal(t, other.GetId(), call.Args[1])
	require.False(t, userContains(member, call.GetId()))
	require.True(t, userContains(to, call.GetId()))

	ReplaceAllValue(bound, to)
	require.Equal(t, to.GetId(), call.Binding["x"])
	require.False(t, userContains(bound, call.GetId()))

	ReplaceAllValue(method, to)
	require.Equal(t, to.GetId(), call.Method)
	require.False(t, userContains(method, call.GetId()))
	require.True(t, userContains(to, call.GetId()))
}

func TestSwitchReplaceValueWritesLabel(t *testing.T) {
	_, builder := newTestBuilder(t)

	cond := builder.EmitConstInst(1)
	caseVal := builder.EmitConstInst(2)
	other := builder.EmitConstInst(3)
	to := builder.EmitConstInst(9)
	dest := builder.NewBasicBlock("case")
	def := builder.NewBasicBlock("default")
	sw := builder.EmitSwitch(cond, def, []SwitchLabel{
		NewSwitchLabel(caseVal, dest),
		NewSwitchLabel(other, dest),
	})
	require.NotNil(t, sw)

	sw.ReplaceValue(caseVal, to)
	require.Equal(t, to.GetId(), sw.Label[0].Value)
	require.Equal(t, other.GetId(), sw.Label[1].Value)
	require.Equal(t, cond.GetId(), sw.Cond)

	sw.ReplaceValue(cond, to)
	require.Equal(t, to.GetId(), sw.Cond)
	require.Equal(t, other.GetId(), sw.Label[1].Value)
}
