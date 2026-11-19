package test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/kotlin/kotlin2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage(ssaconfig.KOTLIN, kotlin2ssa.CreateBuilder)
}

func checkValues(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintf(t, test.TestCase{
		Code:   kotlinWithPrintln(code),
		Want:   want,
		Option: []ssaconfig.Option{ssaapi.WithMemory()},
	})
}

func checkValuesContain(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintf(t, test.TestCase{
		Code:   kotlinWithPrintln(code),
		Want:   want,
		Option: []ssaconfig.Option{ssaapi.WithMemory()},
	}, true)
}

func kotlinWithPrintln(code string) string {
	if strings.Contains(code, "fun println") {
		return code
	}
	return "fun println(a: Int) {}\n" + code
}

func checkSymbols(t *testing.T, code string, names ...string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		for _, name := range names {
			require.NotEmpty(t, prog.Ref(name), "missing SSA symbol %q", name)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
}

func checkPhi(t *testing.T, code, name string, want ...string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		for _, value := range prog.Ref(name) {
			phi, ok := ssa.ToPhi(value.GetSSAInst())
			if !ok {
				continue
			}
			got := make([]string, 0, len(phi.GetValues()))
			for _, input := range phi.GetValues() {
				got = append(got, input.String())
			}
			slices.Sort(got)
			slices.Sort(want)
			require.Equal(t, want, got, "phi inputs for %s", name)
			return nil
		}
		t.Fatalf("no phi found for %s", name)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
}
