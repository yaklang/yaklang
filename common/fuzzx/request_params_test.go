package fuzzx

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func compareParams(t *testing.T, wants, gots []*FuzzParam) {
	t.Helper()
	require.Len(t, gots, len(wants))

	for i, want := range wants {
		got := gots[i]
		require.Equalf(t, want.param, got.param, "[%d] param want: %v, got: %v", i, want.param, got.param)
		require.Equalf(t, want.position, got.position, "[%d] position want: %v, got: %v", i, want.position, got.position)
		if got.path != "" {
			require.Equalf(t, want.path, got.path, "[%d] path want: %v, got: %v", i, want.path, got.path)
		}
		if got.pathKey != "" {
			require.Equalf(t, want.pathKey, got.pathKey, "[%d] pathKey want: %v, got: %v", i, want.pathKey, got.pathKey)
		}
		if want.n != 0 {
			require.Equalf(t, want.n, got.n, "[%d] n want: %v, got: %v", i, want.n, got.n)
		}
	}
}

func TestGetPathParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`GET /a/b/c HTTP/1.1
Host: www.baidu.com`)).GetPathParams()
	wants := []*FuzzParam{
		{
			param:    "/a/b/c",
			position: lowhttp.PosPath,
		},
		{
			param:    "/a/b/c",
			position: lowhttp.PosPathAppend,
		},
		{
			param:    "/a/b/c",
			position: lowhttp.PosPathBlock,
		},
	}
	compareParams(t, wants, gots)
}

func TestGetMethodParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`GET / HTTP/1.1
Host: www.baidu.com`)).GetMethodParams()
	wants := []*FuzzParam{
		{
			position: lowhttp.PosMethod,
		},
	}
	compareParams(t, wants, gots)
}

func TestGetHeaderParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`GET / HTTP/1.1
AAA: BBB
CCC: DDD
Host: www.baidu.com`)).GetHeaderParams()
	sort.SliceStable(gots, func(i, j int) bool {
		if gots[i].param == gots[j].param {
			return gots[i].position < gots[j].position
		}
		return gots[i].param < gots[j].param
	})
	wants := []*FuzzParam{
		{
			param:    "AAA",
			position: lowhttp.PosHeader,
		},
		{
			param:    "CCC",
			position: lowhttp.PosHeader,
		},
		{
			param:    "Host",
			position: lowhttp.PosHeader,
		},
	}
	compareParams(t, wants, gots)
}

func TestGetRawBodyParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.baidu.com

{"a":"b"}`)).GetRawBodyParams()
	wants := []*FuzzParam{
		{
			position: lowhttp.PosBody,
		},
	}
	compareParams(t, wants, gots)
}

func TestGetQueryParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`GET /?a=1&a=2&b={"c":"d"}&c=MTIz&d=eyJjIjoiZCJ9 HTTP/1.1
Host: www.baidu.com`)).GetQueryParams()
	sort.SliceStable(gots, func(i, j int) bool {
		if gots[i].param == gots[j].param {
			if gots[i].position == gots[j].position {
				return gots[i].n < gots[j].n
			}
			return gots[i].position < gots[j].position
		}
		return gots[i].param < gots[j].param
	})
	wants := []*FuzzParam{
		{
			param:    "a",
			position: lowhttp.PosGetQuery,
			n:        0,
		},
		{
			param:    "a",
			position: lowhttp.PosGetQuery,
			n:        1,
		},
		{
			param:    "b",
			position: lowhttp.PosGetQuery,
		},
		{
			param:    "b",
			path:     "$.c",
			position: lowhttp.PosGetQueryJson,
			pathKey:  "c",
		},
		{
			param:    "c",
			position: lowhttp.PosGetQuery,
		},
		{
			param:    "c",
			position: lowhttp.PosGetQueryBase64,
		},
		{
			param:    "d",
			position: lowhttp.PosGetQuery,
		},
		{
			param:    "d",
			position: lowhttp.PosGetQueryBase64,
		},
		{
			param:    "d",
			path:     "$.c",
			position: lowhttp.PosGetQueryBase64Json,
			pathKey:  "c",
		},
	}
	compareParams(t, wants, gots)
}

func TestGetCookieParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`GET / HTTP/1.1
Cookie: a=1;b={"c":"d"};c=MTIz;d=eyJjIjoiZCJ9
Host: www.baidu.com`)).GetCookieParams()
	sort.SliceStable(gots, func(i, j int) bool {
		if gots[i].param == gots[j].param {
			return gots[i].position < gots[j].position
		}
		return gots[i].param < gots[j].param
	})
	wants := []*FuzzParam{
		{
			param:    "a",
			position: lowhttp.PosCookie,
		},
		{
			param:    "b",
			position: lowhttp.PosCookie,
		},
		{
			param:    "b",
			path:     "$.c",
			position: lowhttp.PosCookieJson,
			pathKey:  "c",
		},
		{
			param:    "c",
			position: lowhttp.PosCookie,
		},
		{
			param:    "c",
			position: lowhttp.PosCookieBase64,
		},
		{
			param:    "d",
			position: lowhttp.PosCookie,
		},
		{
			param:    "d",
			position: lowhttp.PosCookieBase64,
		},
		{
			param:    "d",
			path:     "$.c",
			position: lowhttp.PosCookieBase64Json,
			pathKey:  "c",
		},
	}
	compareParams(t, wants, gots)
}

func TestGetPostJsonParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.baidu.com

{"a":"b","c":{"d":"e"}}`)).GetPostJsonParams()
	sort.SliceStable(gots, func(i, j int) bool {
		if gots[i].param == gots[j].param {
			return gots[i].position < gots[j].position
		}
		return gots[i].param < gots[j].param
	})
	wants := []*FuzzParam{
		{
			param:    "a",
			position: lowhttp.PosPostJson,
			path:     "$.a",
		},
		{
			param:    "c",
			position: lowhttp.PosPostJson,
			path:     "$.c",
		},
		{
			param:    "d",
			position: lowhttp.PosPostJson,
			path:     "$.c.d",
		},
	}
	compareParams(t, wants, gots)
}

func TestGetPostXMLParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.baidu.com

<?xml version="1.0"?>
<bookstore>
  <book>
    <title lang="en">aaa</title>
    <author>bbb</author>
    <year>2005</year>
    <price>29.99</price>
  </book>
</bookstore>`)).GetPostXMLParams()
	sort.SliceStable(gots, func(i, j int) bool {
		return gots[i].path < gots[j].path
	})
	wants := []*FuzzParam{
		{
			param:    "bookstore",
			position: lowhttp.PosPostXML,
			path:     "/bookstore",
		},
		{
			param:    "book",
			position: lowhttp.PosPostXML,
			path:     "/bookstore/book",
		},
		{
			param:    "author",
			position: lowhttp.PosPostXML,
			path:     "/bookstore/book/author",
		},
		{
			param:    "price",
			position: lowhttp.PosPostXML,
			path:     "/bookstore/book/price",
		},
		{
			param:    "title",
			position: lowhttp.PosPostXML,
			path:     "/bookstore/book/title",
		},
		{
			param:    "year",
			position: lowhttp.PosPostXML,
			path:     "/bookstore/book/year",
		},
	}
	compareParams(t, wants, gots)
}

func TestGetPostParams(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.baidu.com

a=1&a=2&b={"c":"d"}&c=MTIz&d=eyJjIjoiZCJ9`)).GetPostParams()
	sort.SliceStable(gots, func(i, j int) bool {
		if gots[i].param == gots[j].param {
			if gots[i].position == gots[j].position {
				return gots[i].n == gots[j].n
			}
			return gots[i].position < gots[j].position
		}
		return gots[i].param < gots[j].param
	})
	wants := []*FuzzParam{
		{
			param:    "a",
			position: lowhttp.PosPostQuery,
			n:        0,
		},
		{
			param:    "a",
			position: lowhttp.PosPostQuery,
			n:        1,
		},
		{
			param:    "b",
			position: lowhttp.PosPostQuery,
		},
		{
			param:    "b",
			path:     "$.c",
			position: lowhttp.PosPostQueryJson,
			pathKey:  "c",
		},
		{
			param:    "c",
			position: lowhttp.PosPostQuery,
		},
		{
			param:    "c",
			position: lowhttp.PosPostQueryBase64,
		},
		{
			param:    "d",
			position: lowhttp.PosPostQuery,
		},
		{
			param:    "d",
			position: lowhttp.PosPostQueryBase64,
		},
		{
			param:    "d",
			path:     "$.c",
			position: lowhttp.PosPostQueryBase64Json,
			pathKey:  "c",
		},
	}
	compareParams(t, wants, gots)
}
func TestFuzzParams_CloneFuzzRequest(t *testing.T) {
	freq := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
	Host: www.baidu.com
	
	a=1&a=2`))
	params := freq.GetPostParams()
	for _, param := range params {
		rs := param.Fuzz("c").Fuzz("d").Results()
		require.Len(t, rs, 2)
	}
	params = freq.GetPostParams()
	for _, param := range params {
		rs := param.Fuzz("c", "d").Results()
		require.Len(t, rs, 2)
	}
	params = freq.GetPostParams()
	for _, param := range params {
		rs := param.Fuzz("c").Results()
		require.Len(t, rs, 1)
		rs = param.Fuzz("d").Results()
		require.Len(t, rs, 2)
	}
}

func TestFuzzParams_Duplicate(t *testing.T) {
	gots := MustNewFuzzHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.baidu.com

a=1&a=2`)).GetPostParams()
	for _, param := range gots {
		rs := param.Fuzz("3").Results()
		require.Len(t, rs, 1)
		body := lowhttp.GetHTTPPacketBody(rs[0])
		if param.n == 0 {
			require.Equal(t, "a=3&a=2", string(body))
		} else if param.n == 1 {
			require.Equal(t, "a=1&a=3", string(body))
		}
	}
}
