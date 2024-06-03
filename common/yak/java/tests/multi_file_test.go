package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"gotest.tools/v3/assert"
)

func Test_Multi_File(t *testing.T) {
	progs, err := ssaapi.ParseProjectFromPath(
		"./code/mutiFileDemo",
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithFileSystemEntry("org/main/Main.java"),
	)
	require.NoError(t, err)
	_ = progs
	assert.Equal(t, 1, len(progs))
	progs[0].Show()
}
