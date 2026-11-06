package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestStructBoundCacheReusesUnitFilter(t *testing.T) {
	p := &Program{}
	unit := &ssa.CompileUnit{
		Key:   "unit-a",
		Files: []string{"/src/a.py", "/src/b.py"},
	}

	first := p.getStructBound(unit)
	second := p.getStructBound(unit)
	require.Same(t, first, second)
	_, ok := first.files["/src/a.py"]
	require.True(t, ok)

	other := p.getStructBound(&ssa.CompileUnit{
		Key:   "unit-b",
		Files: []string{"/src/c.py"},
	})
	require.NotSame(t, first, other)
	_, ok = other.files["/src/c.py"]
	require.True(t, ok)
}
