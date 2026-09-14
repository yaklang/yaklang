package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func requireCSharpLiteralTopDef(t *testing.T, prog *ssaapi.Program, name string, want any) {
	t.Helper()
	defs := prog.Ref(name).GetTopDefs()
	for _, def := range defs {
		if def.GetConstValue() != want {
			continue
		}
		require.IsType(t, want, def.GetConstValue(), "%s must retain the signed/unsigned Go constant representation", name)
		require.Equal(t, ssa.NumberTypeKind, def.GetTypeKind())
		return
	}
	require.Failf(t, "integer literal top definition not found", "%s want=%v (%T), defs=%s", name, want, want, defs)
}

func TestCSharp_Literal_IntegerRadixSuffixAndRange(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Literals {
    public static void Run() {
        long leadingZero = 0123;
        long decimalEighteen = 018;
        long binary = 0B111_1011L;
        ulong smallUnsigned = 1_23uL;
        ulong maxUnsigned = 18_446_744_073_709_551_615UL;
        ulong maxHexUnsigned = 0XFFFF_FFFF_FFFF_FFFFuL;
        sink(leadingZero, decimalEighteen, binary, smallUnsigned, maxUnsigned, maxHexUnsigned);
    }
}
`)

	requireCSharpLiteralTopDef(t, prog, "leadingZero", int64(123))
	requireCSharpLiteralTopDef(t, prog, "decimalEighteen", int64(18))
	requireCSharpLiteralTopDef(t, prog, "binary", int64(123))
	requireCSharpLiteralTopDef(t, prog, "smallUnsigned", uint64(123))
	requireCSharpLiteralTopDef(t, prog, "maxUnsigned", ^uint64(0))
	requireCSharpLiteralTopDef(t, prog, "maxHexUnsigned", ^uint64(0))
}
