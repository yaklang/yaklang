package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCompileUnitNestedInitializerUsesLiveDeclarationBlock(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("p/First.java", `package p;
class First {
 static boolean condition;
 static Object value = condition ? new Second() : new Object();
}`)
	fs.AddFile("p/Second.java", `package p;
class Second implements External.Listener {}`)
	programs, err := ParseProjectWithFS(fs, WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(2))
	require.NoError(t, err)
	require.Len(t, programs, 1)
	lib := programs[0].Program.ProgramForCompileUnit("java:p")
	require.NotNil(t, lib)
	parent := lib.GetBluePrint("External.Listener")
	require.NotNil(t, parent)
	container := parent.Container()
	require.NotNil(t, container)
	main := lib.GetFunction(string(ssa.MainFunctionName), "")
	require.NotNil(t, main)
	require.NotEqual(t, main.EnterBlock, container.GetBlock().GetId(),
		"the nested interface declaration must not be appended after the entry block's branch terminator")
}
