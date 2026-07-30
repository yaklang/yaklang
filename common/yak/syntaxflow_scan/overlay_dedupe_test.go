package syntaxflow_scan

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestDedupeProgramsCoveredByOverlay_Empty(t *testing.T) {
	t.Parallel()
	require.Nil(t, dedupeProgramsCoveredByOverlay(nil))
	require.Empty(t, dedupeProgramsCoveredByOverlay([]*ssaapi.Program{}))
}

func TestDedupeProgramsCoveredByOverlay_NoOverlayUnchanged(t *testing.T) {
	t.Parallel()
	// Programs without overlay must all be kept (filter is a no-op).
	// We cannot construct a full Program without compile; nil entries are dropped.
	in := []*ssaapi.Program{nil, nil}
	out := dedupeProgramsCoveredByOverlay(in)
	require.Empty(t, out)
}
