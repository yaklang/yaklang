package ssa_test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestVariadicParameterMemberAccess(t *testing.T) {
	code := `
func getPacketPipeline(name, f, op...) {
    List = []
    switch name {
        case "coverPath":
            List = ["123"]
            if len(op) && op[0] {
                op = ["123"]
            }
    }
    return false
}

println(getPacketPipeline("123", func(){
    return "123"
}, true))
`

	programName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(programName),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	if err != nil {
		t.Fatal(err)
	}

	// 检查是否有错误
	errors := prog.GetErrors()
	t.Logf("Found %d errors", len(errors))

	for _, err := range errors {
		t.Logf("Error: %s", err.String())
		// 检查是否包含我们期望修复的错误
		if err.String() == "The value op unable to access the member with name or index {0} at the call." {
			t.Errorf("Bug still exists: %s", err.String())
		}
	}
}

func TestVariadicParameterInClosure(t *testing.T) {
	code := `
(op...) => {
    wg = sync.NewSizedWaitGroup(20)
    wg.Add()
    wg.Wait()
}
`

	programName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	if err != nil {
		t.Fatal(err)
	}

	// 检查是否有错误
	errors := prog.GetErrors()
	for _, err := range errors {
		t.Logf("Error: %s", err.String())
		// 检查是否包含我们期望修复的错误 对于不包含variadic param的函数调用，不应该将函数调用传参中额外封装make
		if err.String() == "[]any" {
			t.Errorf("Bug still exists: %s", err.String())
		}
	}
}

func TestVariadicParameter(t *testing.T) {
	ssatest.CheckNoError(t, `
Merge_And_Ded = (A,B,C,D,D) => {
    result = []
    return result
}

Get_Metadata_Group = (group_Name,count...) => {
    A = 0
    B = 0
    C = 0
    D = 0
    E = Merge_And_Ded(A,B,C,D,D)

    return E
}
`)
}
