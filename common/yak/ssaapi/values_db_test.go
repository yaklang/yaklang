package ssaapi_test

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestValuesDB_Save_Audit_Node(t *testing.T) {
	t.Run("test save entry node", func(t *testing.T) {
		code := `
		a = {}
		a.c=1
		`
		programName := uuid.NewString()
		prog, err := ssaapi.Parse(code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.Yak))
		t.Cleanup(func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		})

		require.NoError(t, err)
		res, err := prog.SyntaxFlowWithError(`a.c<getObject> as $res;`)
		require.NoError(t, err)
		_, err = res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		nodes, err := ssadb.GetResultNodeByVariable(ssadb.GetDB(), res.GetResultID(), "res")
		require.NoError(t, err)
		require.Equal(t, 1, len(nodes))

	})
}
