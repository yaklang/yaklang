package fuzzx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestParams_FuzzPostJson(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	jsonPath := "$.b"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com
	
{"b":"0"}`)
	freq := MustNewFuzzHTTPRequest(raw)
	params := freq.GetPostJsonParams()
	require.Len(t, params, 1)
	results := params[0].Fuzz(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := string(lowhttp.GetHTTPPacketBody(r))
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}
