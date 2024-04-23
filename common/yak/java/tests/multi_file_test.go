package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func Test_Multi_File(t *testing.T) {
	progs, err := ssaapi.ParseProjectFromPath(
		"./code/mutiFileDemo",
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
	require.NoError(t, err)
	for _, prog := range progs {
		prog.Show()
	}
}
