package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
)

func TestVariableBindingPatternIR(t *testing.T) {
	for _, language := range []ssaconfig.Language{ssaconfig.JS, ssaconfig.TS} {
		for _, tc := range []struct {
			name string
			code string
			want string
		}{
			{"object alias", `const { value: selected } = { value: 42 };`, "42"},
			{"array holes", `let [, selected] = [11, 42];`, "42"},
			{"nested patterns", `var { rows: [, { value: selected }] } = { rows: [11, { value: 42 }] };`, "42"},
			{"default", `const { value: selected = 42 } = {};`, "42"},
			{"commonjs alias", `const { exec: selected } = require("child_process");`, ""},
		} {
			t.Run(fmt.Sprintf("%s/%s", language, tc.name), func(t *testing.T) {
				prog, err := ssaapi.Parse(tc.code+` function sink(value) {} sink(selected);`, ssaapi.WithLanguage(language), ssaapi.WithMemory(true))
				require.NoError(t, err)
				for _, diagnostic := range prog.GetErrors() {
					require.NotEqual(t, ts2ssa.BindPatternDeclarationWithoutInitializer(), diagnostic.Message)
				}
				values := prog.Ref("selected")
				require.Len(t, values, 1)
				if tc.want != "" {
					require.Equal(t, ssa.SSAOpcodeConstInst, values[0].GetSSAInst().GetOpcode())
					require.Equal(t, tc.want, values[0].String())
				} else {
					member := values[0]
					require.True(t, member.IsMember())
					require.Equal(t, "exec", member.GetKey().GetConstValue())
					require.True(t, member.GetObject().IsCall())
					require.Equal(t, "child_process", member.GetObject().GetCallArgs()[0].GetConstValue())
				}
			})
		}
	}
}

func TestVariableBindingPatternSyntaxFlow(t *testing.T) {
	for _, language := range []ssaconfig.Language{ssaconfig.JS, ssaconfig.TS} {
		for _, workers := range []int{1, 4} {
			for _, module := range []string{"values", "lib/values"} {
				t.Run(fmt.Sprintf("%s/workers=%d/module=%s", language, workers, module), func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					fs.AddFile(module+"."+string(language), `
function source() { return "user-input"; }
export const { nested: { value: renamed }, list: [, second], ...rest } = {
  nested: { value: source() }, list: [11, 42], remaining: 99
};
export const [first, , third] = [21, 22, 23];
export const [...all] = [31, 32];
export const { nested: exportedObject } = { nested: { value: 71 } };
`)
					// In the same unit main sorts before values; across directories
					// the units can run concurrently. Both must resolve member reads.
					fs.AddFile("main."+string(language), `
import { renamed, second, first, third, rest, all, exportedObject } from './`+module+`';
function sink(value) {}
sink(renamed);
sink(second);
sink(first);
sink(third);
sink(rest.remaining);
sink(all[1]);
sink(exportedObject.value);
const { exec: run } = require("child_process");
run(renamed);
`)
					// Check the IR in memory, after persistence and after a database reload.
					ssatest.CheckWithFS(fs, t, func(programs ssaapi.Programs) error {
						require.Len(t, programs, 1)
						for _, diagnostic := range programs[0].GetErrors() {
							require.NotEqual(t, ts2ssa.BindPatternDeclarationWithoutInitializer(), diagnostic.Message)
						}
						result, err := programs.SyntaxFlowWithError(`
sink(* #-> as $values)
run(* #-> as $commands)
source() as $sources
`)
						require.NoError(t, err)
						ssatest.CompareResult(t, false, result, map[string][]string{
							"values":   {`"user-input"`, "42", "21", "23", "99", "32", "71"},
							"commands": {`"user-input"`},
						})
						sources := result.GetValues("sources")
						require.Len(t, sources, 1, "the initializer must only be evaluated once")
						require.NotNil(t, sources[0].GetRange())
						require.Equal(t, "values."+string(language), sources[0].GetRange().GetEditor().GetFilename())
						return nil
					}, ssaapi.WithLanguage(language), ssaconfig.WithCompileConcurrency(workers))
				})
			}
		}
	}
}

func TestVariableBindingPatternMissingInitializer(t *testing.T) {
	for _, language := range []ssaconfig.Language{ssaconfig.JS, ssaconfig.TS} {
		for _, code := range []string{`let { missing };`, `let [missing];`} {
			t.Run(fmt.Sprintf("%s/%s", language, code), func(t *testing.T) {
				prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(language), ssaapi.WithMemory(true))
				require.NoError(t, err)
				found := false
				for _, diagnostic := range prog.GetErrors() {
					if diagnostic.Message == ts2ssa.BindPatternDeclarationWithoutInitializer() {
						found = true
					}
				}
				require.True(t, found, "a truly absent initializer must still be diagnosed")
			})
		}
	}
}
