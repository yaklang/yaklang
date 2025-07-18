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

func TestParams_FuzzPostParams_Clone(t *testing.T) {
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=!@&b=2`)
	freq := MustNewFuzzHTTPRequest(raw)
	params := freq.GetPostParams()
	require.Len(t, params, 2)
	p := params[0]
	results := p.Fuzz("1").Results()
	require.Len(t, results, 1)
	results = p.Fuzz("2").Results()
	require.Len(t, results, 1)
}

func TestParams_FuzzQueryParams_Encoding(t *testing.T) {
	raw := []byte(`GET /?a=!@&b=2 HTTP/1.1
Host: www.baidu.com
`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetParams("b", "3").Results()
	require.Len(t, results, 1)
	require.Equal(t, "GET /?a=!@&b=3 HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n", string(results[0]))
}

func TestParams_FuzzPostParams_Encoding(t *testing.T) {
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=!@&b=2`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostParams("b", "3").Results()
	require.Len(t, results, 1)
	require.Equal(t, "POST / HTTP/1.1\r\nHost: www.baidu.com\r\nContent-Length: 8\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\na=!@&b=3", string(results[0]))
}
