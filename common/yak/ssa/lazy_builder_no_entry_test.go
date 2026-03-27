package ssa

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestProgramLazyBuild_LibraryWithoutEntry_NoErrorLog(t *testing.T) {
	var buf bytes.Buffer

	prevLevel := ssalog.Log.Level
	ssalog.Log.SetLevel("error")
	ssalog.Log.SetOutput(&buf)
	t.Cleanup(func() {
		ssalog.Log.Level = prevLevel
		ssalog.Log.SetOutput(os.Stdout)
	})

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	require.NoError(t, err)
	app := NewProgram(cfg, ProgramCacheMemory, Application, filesys.NewVirtualFs(), "", 0)
	editor := memedit.NewMemEditor("")
	editor.SetFileName("main.yak")
	app.PushEditor(editor)
	lib := app.NewLibrary("empty-lib", []string{})
	lib.LazyBuild()

	require.NotContains(t, buf.String(), "main function is not found", "library programs may be empty; should not emit error logs")
}
