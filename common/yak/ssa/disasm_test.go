package ssa_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
)

func TestDisAsmDefaultAndWithSourceFormatting(t *testing.T) {
	src := strings.Join([]string{
		"demo <i64> (1) arg0",
		"type: (i64) -> i64",
		"entry_0:",
		"\t<i64> t2 = <i64> 7",
		"\t<i64> t3 = <i64> arg0 add <i64> t2",
		"\tIf [<i64> t3] true -> then_1, false -> else_2",
		"then_1: <- entry_0 ",
		"\t<i64> t4 = load <i64> t3",
		"\tjump -> merge_3",
		"else_2: <- entry_0 ",
		"\t<i64> t5 = <i64> 99",
		"\tjump -> merge_3",
		"merge_3: <- then_1 else_2 ",
		"\tret <i64> t4",
	}, "\n")

	prog, err := ssairtext.Parse(src)
	require.NoError(t, err)
	fn, ok := prog.Funcs.Get("demo")
	require.True(t, ok)

	mergeBlock, ok := fn.GetBasicBlockByID(fn.Blocks[len(fn.Blocks)-1])
	require.True(t, ok)
	phi := ssa.NewPhi(mergeBlock, "m")
	phi.Edge = []int64{4, 5}
	mergeBlock.Phis = append(mergeBlock.Phis, phi.GetId())

	defaultText := fn.DisAsm(ssa.DisAsmDefault)
	require.Contains(t, defaultText, "demo <i64> (")
	require.Contains(t, defaultText, ") arg0")
	require.Contains(t, defaultText, " = <i64> arg0 add <i64> ")
	require.Contains(t, defaultText, " = load <i64> ")
	require.Contains(t, defaultText, "If [")
	require.Contains(t, defaultText, "jump -> merge_3")
	require.Contains(t, defaultText, "= phi [")
	require.Contains(t, defaultText, "\tret <i64> ")

	withSource := fn.DisAsm(ssa.DisAsmWithSource)
	require.Contains(t, withSource, "entry-0:")
	require.Contains(t, withSource, "ret <i64> ")
}

func TestDisAsmLargeBlockLinearPerformance(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("big\ntype: () -> i64\nentry_0:\n")
	sb.WriteString("\t<i64> t1 = <i64> 1\n")
	sb.WriteString("\t<i64> t2 = <i64> 2\n")
	prev := 1
	nextID := 3
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&sb, "\t<i64> t%d = <i64> t%d add <i64> t2\n", nextID, prev)
		prev = nextID
		nextID++
	}
	fmt.Fprintf(&sb, "\tret <i64> t%d\n", prev)

	prog, err := ssairtext.Parse(sb.String())
	require.NoError(t, err)
	fn, ok := prog.Funcs.Get("big")
	require.True(t, ok)

	out := fn.DisAsm(ssa.DisAsmDefault)
	require.Equal(t, 4000, strings.Count(out, " add "))
	require.Contains(t, out, "\tret <i64> ")
}
