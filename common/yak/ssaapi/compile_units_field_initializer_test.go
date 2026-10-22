package ssaapi

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"testing"
)

func TestCompileUnitParallelFieldInitializerControlFlow(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for _, pkg := range []string{"a", "b"} {
		vf.AddFile(pkg+"/C.java", fmt.Sprintf(`package %s;
class C {
 static boolean flag;
 static Object first = flag ? new Object() : new Object();
 static Object second = new Object();
}`, pkg))
	}
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(2))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	for _, pkg := range []string{"a", "b"} {
		lib := program.Program.ProgramForCompileUnit("java:" + pkg)
		require.NotNil(t, lib)
		class := lib.GetBluePrint("C")
		require.NotNil(t, class)
		value := class.GetStaticMember("second")
		require.NotNil(t, value)
		require.Positive(t, value.GetId(), "the next initializer must be emitted after the previous initializer's branch")
	}
}
