package tests

import (
	_ "embed"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	prog, err := ssaapi.Parse(DynamicSecurityMetadataSource, ssaapi.WithLanguage(ssaapi.JAVA))
	assert.NoErrorf(t, err, "parse error: %v", err)
	prog.Show()
}

func TestREalJava_ThisFunction(t *testing.T) {
	fs := filesys.NewRelLocalFs("/Users/wlz/Developer/Target/yakssaExample/文件/代码/cwzt-fi-manager/cwzt-bill-account/src/main/java/com/pansoft/billacc/server/services/restful/")
	progName := uuid.NewString()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(fs),
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(progName),
	)
	ssa.ShowDatabaseCacheCost()
	// defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	log.Infof("program : %s", progName)

	// prog, err := ssaapi.FromDatabase("801bed0b-5689-4536-a5cd-1ed80a002ceb")
	require.NoError(t, err)
	require.NotNil(t, prog)

}

func TestA(t *testing.T) {
	prog, err := ssaapi.FromDatabase("56ccca57-4c7d-4969-9148-b911bae80780")
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(`
	getDigest as $func 
	$func() as $call 
	$func(*?{!(opcode: param)} as $para)
	`)
	require.NoError(t, err)
	require.NotNil(t, res)
	res.Show()
	require.Equal(t, res.GetValues("call").Len(), 2)
	require.Equal(t, res.GetValues("para").Len(), 2)
	// lo.Map[]()
	ssatest.CompareResult(t, false, res, map[string][]string{
		"para": {`"MD5"`, `"SHA"`},
	})
}
