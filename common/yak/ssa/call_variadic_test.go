package ssa_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
		ssaapi.WithLanguage(ssaapi.Yak),
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
