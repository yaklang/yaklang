package tests

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCSharp_CollectionInitializerInvokesAddOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class SingleBag {
    public void Add(object value) { singleAddSink(value); }
}

public class PairBag {
    public void Add(object key, object value) {
        pairKeySink(key);
        pairValueSink(value);
    }
}

public class Program {
    public static void Main() {
        var single = new SingleBag { singleSource() };
        var pair = new PairBag { { pairKeySource(), pairValueSource() } };
        var external = new ExternalBag { externalSource() };
        var externalMap = new ExternalMap { { "key", externalValueSource() } };

        singleStorageSink(single[0]);
        externalStorageSink(external[0]);
        externalMapStorageSink(externalMap["key"]);
    }
}
`)

	assertFlow := func(source, sink string) {
		t.Helper()
		result, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
		require.NoError(t, err)
		require.Contains(t, result.GetValues("origin").String(), "Undefined-"+source,
			"%s must reach %s", source, sink)
	}
	assertFlow("singleSource", "singleAddSink")
	assertFlow("pairKeySource", "pairKeySink")
	assertFlow("pairValueSource", "pairValueSink")
	assertFlow("singleSource", "singleStorageSink")
	assertFlow("externalSource", "externalStorageSink")
	assertFlow("externalValueSource", "externalMapStorageSink")

	for _, source := range []string{
		"singleSource", "pairKeySource", "pairValueSource", "externalSource", "externalValueSource",
	} {
		require.Len(t, csharpCallsToMethod(t, prog, source), 1,
			"collection initializer expression %s() must be evaluated exactly once", source)
	}

	addArities := make(map[string][]int)
	prog.Program.EachFunction(func(function *ssa.Function) {
		function.Build()
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok {
					continue
				}
				call, ok := ssa.ToCall(instruction)
				if !ok || len(call.Args) == 0 {
					continue
				}
				callee, ok := call.GetValueById(call.Method)
				if !ok || callee == nil {
					continue
				}
				receiver, ok := call.GetValueById(call.Args[0])
				if !ok || receiver == nil {
					continue
				}
				blueprint, ok := ssa.ToBluePrintType(receiver.GetType())
				if !ok || blueprint == nil {
					continue
				}
				if callee.GetName() != "Add" && !strings.HasSuffix(callee.GetName(), ".Add") {
					continue
				}
				addArities[blueprint.Name] = append(addArities[blueprint.Name], len(call.Args)-1)
			}
		}
	})
	for _, arities := range addArities {
		sort.Ints(arities)
	}
	require.Equal(t, []int{1}, addArities["SingleBag"])
	require.Equal(t, []int{2}, addArities["PairBag"])
	require.Equal(t, []int{1}, addArities["ExternalBag"])
	require.Equal(t, []int{2}, addArities["ExternalMap"])
}
