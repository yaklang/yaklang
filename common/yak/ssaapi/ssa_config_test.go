package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestDefaultProcess(t *testing.T) {
	config, err := DefaultConfig(
		WithFileSystem(filesys.NewLocalFs()),
		WithProcess(func(msg string, process float64) {
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.process)
}
