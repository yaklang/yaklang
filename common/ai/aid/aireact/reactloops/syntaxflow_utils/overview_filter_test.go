package syntaxflow_utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

// mapLoop is a minimal LoopStringGetter for tests.
type mapLoop map[string]string

func (m mapLoop) Get(k string) string {
	if m == nil {
		return ""
	}
	return m[k]
}

func TestBuildSSARisksFilterFromLoop_JSON(t *testing.T) {
	f0 := &ypb.SSARisksFilter{Search: "x"}
	b, err := protojson.Marshal(f0)
	require.NoError(t, err)
	loop := mapLoop{LoopVarSSARisksFilterJSON: string(b)}
	f := BuildSSARisksFilterFromLoop(loop, "")
	require.Equal(t, "x", f.Search)
}

func TestBuildSSARisksFilterFromLoop_UserSearchOnly(t *testing.T) {
	loop := mapLoop{}
	f := BuildSSARisksFilterFromLoop(loop, "只看高危 SQL")
	require.Equal(t, "只看高危 SQL", f.Search)
}
