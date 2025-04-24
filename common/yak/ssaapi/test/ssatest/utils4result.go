package ssatest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func CheckResult(t *testing.T, fs filesys_interface.FileSystem, rule string, handler func(*ssaapi.SyntaxFlowResult), opt ...ssaapi.Option) {
	progName := uuid.NewString()
	opt = append(opt, ssaapi.WithProgramName(progName))
	prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err)

	// memory
	log.Infof("only in memory")
	handler(res)

	// database
	id, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	res, err = ssaapi.LoadResultByID(id)
	require.NoError(t, err)
	log.Infof("with database")
	handler(res)
}
