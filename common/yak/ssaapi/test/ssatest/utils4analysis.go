package ssatest

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type checkFunction func(*ssaapi.Program) error

func CheckWithFS(fs filesys.FileSystem, t *testing.T, handler func(ssaapi.Programs) error, opt ...ssaapi.Option) {
	// only in memory
	{
		prog, err := ssaapi.ParseProject(fs, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		opt = append(opt, ssaapi.WithDatabaseProgramName(programID))
		prog, err := ssaapi.ParseProject(fs, opt...)
		defer func() {
			// if name == "" {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
			// }
		}()
		assert.Nil(t, err)
		// prog.Show()

		log.Infof("with database ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)

		log.Infof("only use database ")
		err = handler([]*ssaapi.Program{prog})
		assert.Nil(t, err)
	}
}

func CheckWithName(
	name string,
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	// only in memory
	{
		prog, err := ssaapi.Parse(code, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	if name != "" {
		programID = name
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		opt = append(opt, ssaapi.WithDatabaseProgramName(programID))
		prog, err := ssaapi.Parse(code, opt...)
		defer func() {
			// if name == "" {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
			// }
		}()
		assert.Nil(t, err)
		// prog.Show()

		log.Infof("with database ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)

		log.Infof("only use database ")
		err = handler(prog)
		assert.Nil(t, err)
	}
}

func Check(
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	CheckWithName("", t, code, handler, opt...)
}

func CheckFSWithProgram(
	t *testing.T, programName string,
	codeFS, ruleFS filesys.FileSystem, opt ...ssaapi.Option) {

	if programName == "" {
		programName = "test-" + uuid.New().String()
	}
	ssadb.DeleteProgram(ssadb.GetDB(), programName)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	opt = append(opt, ssaapi.WithDatabaseProgramName(programName))
	_, err := ssaapi.ParseProject(codeFS, opt...)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	program, err := ssaapi.FromDatabase(programName)
	if err != nil {
		t.Fatalf("get program from database failed: %v", err)
	}
	filesys.Recursive(".", filesys.WithFileSystem(ruleFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if !strings.HasSuffix(s, ".sf") {
			log.Infof("skip file: %s", s)
			return nil
		}

		t.Run(fmt.Sprintf("case in %v", s), func(t *testing.T) {
			log.Infof("start to check file: %s", s)
			raw, err := ruleFS.ReadFile(s)
			if err != nil {
				t.Fatalf("read file[%s] failed: %v", s, err)
			}
			i, err := program.SyntaxFlowWithError(string(raw))
			if err != nil {
				t.Fatalf("exec syntaxflow failed: %v", err)
			}
			if len(i.Errors) > 0 {
				log.Infof("result: %s", i.String())
				t.Fatalf("result has errors: %v", i.Errors)
			}
		})

		return nil
	}))
}

func CheckSyntaxFlowContain(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, true, wants, opt, nil)
}

func CheckSyntaxFlowWithFS(t *testing.T, fs filesys.FileSystem, sf string, wants map[string][]string, contain bool, opt ...ssaapi.Option) {
	CheckWithFS(fs, t, func(p ssaapi.Programs) error {
		results, err := p.SyntaxFlowWithError(sf)
		assert.Nil(t, err)
		assert.NotNil(t, results)
		compareResult(t, contain, results, wants)
		return nil
	}, opt...)
}
func CheckSyntaxFlow(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, false, wants, opt, nil)
}

func CheckSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, contain, wants, opt, nil)
}

func CheckSyntaxFlowWithSFOption(t *testing.T, code string, sf string, wants map[string][]string, opt ...sfvm.Option) {
	checkSyntaxFlowEx(t, code, sf, false, wants, nil, opt)
}

func checkSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, ssaOpt []ssaapi.Option, sfOpt []sfvm.Option) {
	Check(t, code, func(prog *ssaapi.Program) error {
		sfOpt = append(sfOpt, sfvm.WithEnableDebug(true))
		results, err := prog.SyntaxFlowWithError(sf, sfOpt...)
		assert.Nil(t, err)
		assert.NotNil(t, results)
		compareResult(t, contain, results, wants)
		return nil
	}, ssaOpt...)
}

func compareResult(t *testing.T, contain bool, results *ssaapi.SyntaxFlowResult, wants map[string][]string) {
	results.Show()
	for k, want := range wants {
		gotVs := results.GetValues(k)
		assert.Greater(t, len(gotVs), 0, "key[%s] not found", k)
		got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
		sort.Strings(got)
		sort.Strings(want)
		if contain {
			if !utils.ContainsAll(got, want...) {
				t.Fatalf("\nkey[%s] \ngot[%v] \nwant[%v]", k, strings.Join(got, ","), strings.Join(want, ","))
			}
		} else {
			assert.Equal(t, len(want), len(gotVs))
			assert.Equal(t, want, got)
		}
	}
}

func CheckBottomUser_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckBottomUserCall_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				lastIndex := strings.LastIndex(variable, ".")
				if lastIndex != -1 {
					member := variable[:lastIndex]
					key := variable[lastIndex+1:]
					return p.Ref(member).Ref(key)
				} else {
					return p.Ref(variable)
				}
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Equal(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return v1.String() == v2
			},
		)
	}
}

func checkFunctionEx(
	variable func() ssaapi.Values, // variable  for test
	get func(*ssaapi.Value) ssaapi.Values, // getTop / getBottom
	checkLength bool,
	want []string,
	compare func(*ssaapi.Value, string) bool,
) error {
	values := variable()
	if len(values) != 1 {
		return fmt.Errorf("variable[%s] not len(1): %d", values, len(values))
	}
	value := values[0]
	vs := get(value)
	vs = lo.UniqBy(vs, func(v *ssaapi.Value) int64 { return v.GetId() })
	if checkLength {
		if len(vs) != len(want) {
			return fmt.Errorf("variable[%v] not want len(%d): %d: %v", values, len(want), len(vs), vs)
		}
	}
	mark := make([]bool, len(want))
	for _, value := range vs {
		log.Infof("value: %s", value.String())
		for j, w := range want {
			mark[j] = mark[j] || compare(value, w)
		}
	}
	for i, m := range mark {
		if !m {
			return fmt.Errorf("want[%d] %s not found", i, want[i])
		}
	}
	return nil
}
