package yakcmds_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func addCommands(app *cli.App, cmds ...*cli.Command) {
	for _, i := range cmds {
		app.Commands = append(app.Commands, *i)
	}
}

func TestRecompileWarn(t *testing.T) {
	tmpDir := t.TempDir()
	log.Infof("tmpDir: %s", tmpDir)

	// create a test yak file
	file, err := os.Create(tmpDir + "/test.yak")
	require.NoError(t, err)
	file.WriteString(`
	a = 1
	`)
	file.Close()

	programName := uuid.NewString()

	// compile the yak file
	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	// compile with program name
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	// recompile
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "please use `re-compile` flag to re-compile or change program name")
}
