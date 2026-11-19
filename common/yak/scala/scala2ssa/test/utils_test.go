package test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/scala/scala2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage(ssaconfig.SCALA, scala2ssa.CreateBuilder)
}

func checkPrintlnValue(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValue(scalaWithPrintln(code), want, t, ssaapi.WithMemory())
}

func checkPrintlnValueContain(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValueContain(scalaWithPrintln(code), want, t, ssaapi.WithMemory())
}

func checkPrintlnValueRegexp(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintlnValueRegexp(scalaWithPrintln(code), want, t, ssaapi.WithMemory())
}

func checkValues(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintf(t, test.TestCase{
		Code:   scalaWithPrintln(code),
		Want:   want,
		Option: []ssaconfig.Option{ssaapi.WithMemory()},
	})
}

func checkValuesContain(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckPrintf(t, test.TestCase{
		Code:   scalaWithPrintln(code),
		Want:   want,
		Option: []ssaconfig.Option{ssaapi.WithMemory()},
	}, true)
}

func scalaWithPrintln(code string) string {
	if strings.Contains(code, "def println") {
		return code
	}
	return "def println(a: Int): Unit = {}\n" + code
}

func checkSymbols(t *testing.T, code string, names ...string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		for _, name := range names {
			require.NotEmpty(t, prog.Ref(name), "missing SSA symbol %q", name)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
}

func checkFreeValues(t *testing.T, code string, want ...string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, scalaWithPrintln(code), func(prog *ssaapi.Program) error {
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
	}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
}

func checkTypeCast(t *testing.T, code, name string) {
	t.Helper()
	test.CheckWithNameOnlyInMemory("", t, scalaWithPrintln(code), func(prog *ssaapi.Program) error {
		for _, value := range prog.Ref(name) {
			if value.IsTypeCast() {
				return nil
			}
		}
		t.Fatalf("no typecast for %s", name)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
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
	}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
}
