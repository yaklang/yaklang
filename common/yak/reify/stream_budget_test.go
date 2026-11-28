package reify

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestBuildBatchesByteBudgetAndExplicitCount(t *testing.T) {
	boundaries := []core.FunctionBoundary{
		{Address: 0x1000, Size: 1000},
		{Address: 0x2000, Size: 2000},
		{Address: 0x3000, Size: 30000},
		{Address: 0x4000, Size: 50000},
		{Address: 0x5000, Size: 1000},
		{Address: 0x6000, Size: 2000},
	}

	explicit := buildBatches(boundaries, 2, 64, 32<<10)
	require.Equal(t, []funcBatch{
		{start: 0, end: 2, codeBytes: 3000},
		{start: 2, end: 4, codeBytes: 80000},
		{start: 4, end: 6, codeBytes: 3000},
	}, explicit)

	budgeted := buildBatches(boundaries, 0, 64, 32<<10)
	require.Equal(t, []funcBatch{
		{start: 0, end: 2, codeBytes: 3000},
		{start: 2, end: 3, codeBytes: 30000},
		{start: 3, end: 4, codeBytes: 50000},
		{start: 4, end: 6, codeBytes: 3000},
	}, budgeted)
}
