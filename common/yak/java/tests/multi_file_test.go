package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"gotest.tools/v3/assert"
)

func Test_Multi_File(t *testing.T) {
	progs, err := ssaapi.ParseProjectFromPath(
		"./code/mutiFileDemo",
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithFileSystemEntry("org/main/Main.java"),
	)
	require.NoError(t, err)
	applicationLen := 0
	_ = progs
	for _, p := range progs {
		if p.GetProgramKind() == ssa.Application {
			applicationLen++
		}
		p.Show()
	}
	assert.Equal(t, 1, applicationLen)
	progs[0].Show()
}
