package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestExternCollisionParameterPosition(t *testing.T) {
	code := `
	maxRetry = 3
	retryHandler = func(https, retryCount, req, rsp, retry) {
		if retryCount > maxRetry {
			return
		}
	}
	`

	results := yak.StaticAnalyze(code, yak.WithStaticAnalyzePluginType("yak"))
	require.NotEmpty(t, results)
	require.Equal(t, ssa.ContAssignExtern("retry"), results[0].Message)
	require.Equal(t, int64(3), results[0].StartLineNumber)
}
