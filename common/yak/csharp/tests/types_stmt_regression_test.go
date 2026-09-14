package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func typeStmtRegressionCallArgs(prog *ssaapi.Program, name string) ssaapi.Values {
	return prog.Ref(name).GetUsers().Flat(func(user *ssaapi.Value) ssaapi.Values {
		if user.GetOpcode() != "Call" {
			return nil
		}
		arg := user.GetOperand(1)
		if arg == nil || arg.IsNil() {
			return nil
		}
		return ssaapi.Values{arg}
	})
}

func TestCSharp_Type_QualifiedNameResolvesExactBlueprint(t *testing.T) {
	prog := parseCSharpSemantics(t, `
namespace A {
    public class Input { public string Tainted; }
}
namespace B {
    public class Input { public string Safe; }
    public class Consumer {
        public static void Use(A.Input x) { sink(x.Tainted); }
    }
}
`)

	var parameter *ssaapi.Value
	for _, value := range prog.Ref("x") {
		if value.IsParameter() {
			parameter = value
			break
		}
	}
	require.NotNil(t, parameter, "method parameter x must be emitted")
	bp, ok := ssa.ToBluePrintType(ssaapi.GetBareType(parameter.GetType()))
	require.True(t, ok, "A.Input parameter must retain its blueprint type: %s", parameter.GetType())
	require.Contains(t, bp.GetFullTypeNames(), "A.Input")
	require.NotContains(t, bp.GetFullTypeNames(), "B.Input")
	require.NotNil(t, bp.GetNormalMember("Tainted"), "A.Input member must be available")
	require.Nil(t, bp.GetNormalMember("Safe"), "B.Input members must not leak into A.Input")
}

func TestCSharp_ControlFlow_DoWhileExecutesBodyBeforeCondition(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        int value = 0;
        do { value = 1; } while (false);
        sink(value);
    }
}
`)

	args := typeStmtRegressionCallArgs(prog, "sink")
	require.Len(t, args, 1)
	require.Contains(t, args[0].GetTopDefs().String(), "1",
		"the guaranteed first iteration must reach the value after the loop: %s", args[0])
	require.NotEqual(t, "0", args[0].String(), "do/while must not preserve only the pre-loop value")

	conditionProg := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        do { work(); } while (again());
    }
}
`)
	conditionCalls := conditionProg.Ref("again").GetUsers().Filter(func(value *ssaapi.Value) bool {
		return value.GetOpcode() == "Call"
	})
	require.Len(t, conditionCalls, 1)
	conditionID := conditionCalls[0].GetId()
	conditionFeedsLoop := false
	conditionProg.Program.EachFunction(func(function *ssa.Function) {
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, phiID := range block.Phis {
				instruction, ok := function.GetInstructionById(phiID)
				if !ok {
					continue
				}
				phi, ok := ssa.ToPhi(instruction)
				if !ok {
					continue
				}
				for _, edgeID := range phi.Edge {
					if edgeID == conditionID {
						conditionFeedsLoop = true
					}
				}
			}
		}
	})
	require.True(t, conditionFeedsLoop,
		"the post-body condition must feed the loop condition phi instead of being emitted as a dead latch value")
}

func TestCSharp_Try_UnnamedCatchDoesNotShadowOuterVariable(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        object ex = source();
        try { mayFail(); }
        catch (NeverSeenException) { sink(ex); }
    }
}
`)

	args := typeStmtRegressionCallArgs(prog, "sink")
	require.Len(t, args, 1)
	require.Contains(t, args[0].GetTopDefs().String(), "source",
		"an unnamed catch must leave the outer ex binding visible: %s", args[0].GetTopDefs())
	require.NotContains(t, args[0].String(), "Undefined-ex")

	// NeverSeenException is first encountered in the catch clause. Resolving it
	// before TryBuilder enters its finished dispatch block must still produce a type.
	require.NotEmpty(t, prog.Ref("NeverSeenException"))
	typedCatch := false
	prog.Program.EachFunction(func(function *ssa.Function) {
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
				caught, ok := instruction.(*ssa.ErrorCatch)
				if !ok {
					continue
				}
				exception, ok := function.GetValueById(caught.Exception)
				if ok && exception.GetType().GetTypeKind() == ssa.ClassBluePrintTypeKind {
					typedCatch = true
				}
			}
		}
	})
	require.True(t, typedCatch, "the first-seen catch type must be applied to the hidden exception value")

	generalCatchProg := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        object ex = source();
        try { mayFail(); }
        catch { sink(ex); }
    }
}
`)
	generalArgs := typeStmtRegressionCallArgs(generalCatchProg, "sink")
	require.Len(t, generalArgs, 1)
	require.Contains(t, generalArgs[0].GetTopDefs().String(), "source",
		"a general catch must not introduce a source-visible ex variable")
}

func TestCSharp_ControlFlow_SwitchDefaultEmittedOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run(int value) {
        switch (value) {
        case 1:
            sink("case");
            break;
        default:
            sink("default");
            break;
        }
    }
}
`)

	args := typeStmtRegressionCallArgs(prog, "sink")
	require.Len(t, args, 2, "each source switch section must be emitted exactly once: %s", args)
	var defaultCount int
	for _, arg := range args {
		if arg.String() == `"default"` {
			defaultCount++
		}
	}
	require.Equal(t, 1, defaultCount, "default body must not be duplicated: %s", args)
}

func TestCSharp_PatternDesignationBindsSubject(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "switch var",
			body: `
                switch (subject) {
                case var captured:
                    sink(captured);
                    break;
                }`,
		},
		{
			name: "is declaration",
			body: `
                if (subject is string captured) {
                    sink(captured);
                }`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run(object subject) {
`+test.body+`
    }
}
`)
			args := typeStmtRegressionCallArgs(prog, "sink")
			require.Len(t, args, 1)
			topDefs := args[0].GetTopDefs().String()
			require.True(t, strings.Contains(topDefs, "Parameter-subject") || strings.Contains(topDefs, "parameter[0]"),
				"pattern designation must be bound to the matched subject, got %s (arg %s)", topDefs, args[0])
			require.NotContains(t, args[0].String(), "Undefined-captured")
		})
	}
}
