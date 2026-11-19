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

func checkPrintlnValue(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValue(kotlinWithPrintln(code), want, t, ssaapi.WithMemory())
}

func checkPrintlnValueContain(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValueContain(kotlinWithPrintln(code), want, t, ssaapi.WithMemory())
}

func checkPrintlnValueRegexp(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValueRegexp(kotlinWithPrintln(code), want, t, ssaapi.WithMemory())
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

func checkFreeValues(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, kotlinWithPrintln(code), func(prog *ssaapi.Program) error {
		found := make(map[string]bool)
		var visit func(*ssa.Function)
		visit = func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			for variable := range fn.FreeValues {
				if variable != nil {
					found[variable.GetName()] = true
				}
			}
			for _, id := range fn.ChildFuncs {
				if child, ok := fn.GetValueById(id); ok {
					if childFn, ok := ssa.ToFunction(child); ok {
						visit(childFn)
					}
				}
			}
		}
		if prog.Program != nil {
			prog.Program.Funcs.ForEach(func(_ string, fn *ssa.Function) bool {
				visit(fn)
				return true
			})
		}
		for _, name := range want {
			require.Truef(t, found[name], "free value %q not found; got %v", name, found)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
}

func checkTypeCast(t *testing.T, code, name string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, kotlinWithPrintln(code), func(prog *ssaapi.Program) error {
		for _, value := range prog.Ref(name) {
			if value.IsTypeCast() {
				return nil
			}
		}
		t.Fatalf("no typecast for %s", name)
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
