package fuzzx

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/antchfx/xmlquery"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestRepeat(t *testing.T) {
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.Repeat(10).Results()
	require.Len(t, results, 10)
	for _, r := range results {
		require.Equal(t, raw, r)
	}
}

func TestFuzzMethod(t *testing.T) {
	iFuzztag := "GET{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzMethod(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], lowhttp.GetHTTPRequestMethod(r))
	}
}

func TestFuzzPath(t *testing.T) {
	iFuzztag := "/{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPath(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], lowhttp.GetHTTPRequestPathWithoutQuery(r))
	}
}

func TestFuzzPathAppend(t *testing.T) {
	iFuzztag := "/{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET /prefix HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPathAppend(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, "/prefix"+excepts[i], lowhttp.GetHTTPRequestPathWithoutQuery(r))
	}
}

func TestFuzzPathBlock(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	excepts := lo.Flatten([][]string{
		QuickMutateSimple(fmt.Sprintf("/%s/2/3", iFuzztag)),
		QuickMutateSimple(fmt.Sprintf("/1/%s/3", iFuzztag)),
		QuickMutateSimple(fmt.Sprintf("/1/2/%s", iFuzztag)),
	})
	raw := []byte(`GET /1/2/3 HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPathBlock(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], lowhttp.GetHTTPRequestPathWithoutQuery(r))
	}
}

func TestFuzzHeader(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	headerKey := utils.RandStringBytes(16)
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzHTTPHeader(headerKey, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], lowhttp.GetHTTPPacketHeader(r, headerKey))
	}
}

func TestFuzzCookie(t *testing.T) {
	key := "a"
	iFuzztag := "{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	t.Run("Append", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookie(key, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			require.Equal(t, excepts[i], lowhttp.GetHTTPPacketCookieFirst(r, key))
		}
	})

	t.Run("Replace", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=0`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookie(key, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			require.Equal(t, excepts[i], lowhttp.GetHTTPPacketCookieFirst(r, key))
		}
	})
}

func TestFuzzCookieBase64(t *testing.T) {
	key := "a"
	iFuzztag := "{{char(a-z)}}"
	excepts := QuickMutateSimple(fmt.Sprintf("{{base64(%s)}}", iFuzztag))
	t.Run("Append", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieBase64(key, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			require.Equal(t, excepts[i], lowhttp.GetHTTPPacketCookieFirst(r, key))
		}
	})

	t.Run("Replace", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=MA==`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieBase64(key, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			require.Equal(t, excepts[i], lowhttp.GetHTTPPacketCookieFirst(r, key))
		}
	})
}

func TestFuzzCookieJsonPath(t *testing.T) {
	key := "a"
	iFuzztag := "{{char(a-z)}}"
	jsonPath := "$.b"
	excepts := QuickMutateSimple(iFuzztag)
	t.Run("Append", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieJsonPath(key, jsonPath, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			v := lowhttp.GetHTTPPacketCookieFirst(r, key)
			v, ok := utils.IsJSON(v)
			require.True(t, ok)
			got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
			require.NotEmpty(t, got)
			require.Equal(t, excepts[i], got)
		}
	})

	t.Run("Replace", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a={"b":"0"}`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieJsonPath(key, jsonPath, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			v := lowhttp.GetHTTPPacketCookieFirst(r, key)
			v, ok := utils.IsJSON(v)
			require.True(t, ok)
			got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
			require.NotEmpty(t, got)
			require.Equal(t, excepts[i], got)
		}
	})
}

func TestFuzzCookieBase64JsonPath(t *testing.T) {
	key := "a"
	iFuzztag := "{{char(a-z)}}"
	rawParam := `{"b":"0"}`
	jsonPath := "$.b"
	excepts := lo.Map(QuickMutateSimple(fmt.Sprintf(`{"b":"%s"}`, iFuzztag)), func(s string, _ int) string {
		return codec.EncodeBase64(s)
	})
	t.Run("Append", func(t *testing.T) {
		raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieBase64JsonPath(key, jsonPath, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			v := lowhttp.GetHTTPPacketCookieFirst(r, key)
			_, ok := mutate.IsBase64JSON(v)
			require.True(t, ok)
			require.Equal(t, excepts[i], v)
		}
	})

	t.Run("Replace", func(t *testing.T) {
		raw := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=%s`, codec.EncodeBase64(rawParam)))
		freq := MustNewFuzzHTTPRequest(raw)
		results := freq.FuzzCookieBase64JsonPath(key, jsonPath, iFuzztag).Results()
		require.Len(t, results, len(excepts))
		for i, r := range results {
			v := lowhttp.GetHTTPPacketCookieFirst(r, key)
			_, ok := mutate.IsBase64JSON(v)
			require.True(t, ok)
			require.Equal(t, excepts[i], v)
		}
	})
}

func TestFuzzGetParamsRaw(t *testing.T) {
	iFuzztag := "a=%62%25&c={{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetParamsRaw(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, "/?"+excepts[i], lowhttp.GetHTTPRequestPath(r))
	}
}

func TestFuzzGetParams(t *testing.T) {
	iFuzztag := "%25{{char(a-z)}}"
	key := "a[]"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetParams(key, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, "/?"+key+"="+excepts[i], lowhttp.GetHTTPRequestPath(r))
	}
}

func TestDuplicate_FuzzGetParams(t *testing.T) {
	iFuzztag := "3"
	key := "a"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET /?a=1&a=2 HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetParams(key, iFuzztag, 1).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, "/?a=1&"+key+"="+excepts[i], lowhttp.GetHTTPRequestPath(r))
	}
}

func TestFuzzGetBase64Params(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a[]"
	excepts := QuickMutateSimple(fmt.Sprintf("{{base64(%s)}}", iFuzztag))
	raw := []byte(`GET / HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetBase64Params(key, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, "/?"+key+"="+excepts[i], lowhttp.GetHTTPRequestPath(r))
	}
}

func TestFuzzGetJsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	jsonPath := "$.c.d"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET /?a={"c":{"d":"123"}} HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetJsonPathParams(key, jsonPath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := lowhttp.GetHTTPRequestQueryParam(r, key)
		v, ok := utils.IsJSON(v)
		require.True(t, ok)
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}

func TestDuplicate_FuzzGetJsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	jsonPath := "$.c.d"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`GET /?a=1&a={"c":{"d":"123"}} HTTP/1.1
Host: www.baidu.com`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetJsonPathParams(key, jsonPath, iFuzztag, 1).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		vs := lowhttp.GetHTTPRequestQueryParamFull(r, key)
		require.Len(t, vs, 2)
		v := vs[1]
		v, ok := utils.IsJSON(v)
		require.True(t, ok)
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}

func TestFuzzGetBase64JsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	jsonPath := "$.c.d"
	rawParam := `{"c":{"d":"0"}}`
	excepts := lo.Map(QuickMutateSimple(fmt.Sprintf(`{"c":{"d":"%s"}}`, iFuzztag)), func(s string, _ int) string {
		return codec.EncodeBase64(s)
	})
	raw := []byte(fmt.Sprintf(`GET /?a=%s HTTP/1.1
Host: www.baidu.com`, codec.EncodeBase64(rawParam)))
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzGetBase64JsonPathParams(key, jsonPath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := lowhttp.GetHTTPRequestQueryParam(r, key)
		_, ok := mutate.IsBase64JSON(v)
		require.True(t, ok)
		require.NotEmpty(t, v)
		require.Equal(t, excepts[i], v)
	}
}

func TestFuzzPostRaw(t *testing.T) {
	iFuzztag := "%25{{char(a-z)}}"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

empty`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostRaw(iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], string(lowhttp.GetHTTPPacketBody(r)))
	}
}

func TestFuzzPostParams(t *testing.T) {
	iFuzztag := "%25{{char(a-z)}}"
	key := "c"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=b`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostParams(key, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		body := lowhttp.GetHTTPPacketBody(r)
		require.Equal(t, fmt.Sprintf("a=b&c=%s", excepts[i]), string(body))
	}
}

func TestDuplicate_FuzzPostParams(t *testing.T) {
	iFuzztag := "d"
	key := "a"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=b&a=c`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostParams(key, iFuzztag, 1).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		vs := lowhttp.GetHTTPRequestPostParamFull(r, key)
		require.Len(t, vs, 2)
		require.Equal(t, excepts[i], vs[1])
	}
}

func TestFuzzPostBase64Params(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "c"
	excepts := QuickMutateSimple(fmt.Sprintf("{{base64(%s)}}", iFuzztag))
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=b`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostBase64Params(key, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		require.Equal(t, excepts[i], lowhttp.GetHTTPRequestPostParam(r, key))
	}
}

func TestFuzzPostJson(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	jsonPath := "$.b"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

{"b":"0"}`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostJson(jsonPath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := string(lowhttp.GetHTTPPacketBody(r))
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}

func TestFuzzPostJsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	jsonPath := "$.c.d"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a={"c":{"d":"123"}}`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostJsonPathParams(key, jsonPath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := lowhttp.GetHTTPRequestPostParam(r, key)
		v, ok := utils.IsJSON(v)
		require.True(t, ok)
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}

func TestDuplicate_FuzzPostJsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	jsonPath := "$.c.d"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

a=1&a={"c":{"d":"123"}}`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostJsonPathParams(key, jsonPath, iFuzztag, 1).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		vs := lowhttp.GetHTTPRequestPostParamFull(r, key)
		require.Len(t, vs, 2)
		v := vs[1]
		v, ok := utils.IsJSON(v)
		require.True(t, ok)
		got := utils.InterfaceToString(jsonpath.Find(v, jsonPath))
		require.NotEmpty(t, got)
		require.Equal(t, excepts[i], got)
	}
}

func TestFuzzPostBase64JsonPathParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	key := "a"
	rawParam := `{"c":{"d":"0"}}`
	jsonPath := "$.c.d"
	excepts := lo.Map(QuickMutateSimple(fmt.Sprintf(`{"c":{"d":"%s"}}`, iFuzztag)), func(s string, _ int) string {
		return codec.EncodeBase64(s)
	})
	raw := []byte(fmt.Sprintf(`POST / HTTP/1.1
Host: www.baidu.com

a=%s`, rawParam))
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostBase64JsonPathParams(key, jsonPath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := lowhttp.GetHTTPRequestPostParam(r, key)
		_, ok := mutate.IsBase64JSON(v)
		require.True(t, ok)
		require.Equal(t, excepts[i], v)
	}
}

func TestFuzzPostXMLParams(t *testing.T) {
	iFuzztag := "{{char(a-z)}}"
	xpath := "/bookstore/book[1]/author"
	excepts := QuickMutateSimple(iFuzztag)
	raw := []byte(`POST / HTTP/1.1
Host: www.baidu.com

<?xml version="1.0"?>
<bookstore>
  <book>
    <title lang="en">Unknown</title>
    <author>Unknown</author>
    <year>2005</year>
    <price>29.99</price>
  </book>
  <book>
    <title lang="en">English Book</title>
    <author>Lang</author>
    <year>2000</year>
    <price>1.99</price>
  </book>
</bookstore>`)
	freq := MustNewFuzzHTTPRequest(raw)
	results := freq.FuzzPostXMLParams(xpath, iFuzztag).Results()
	require.Len(t, results, len(excepts))
	for i, r := range results {
		v := lowhttp.GetHTTPPacketBody(r)
		rootNode, err := xmlquery.Parse(bytes.NewReader(v))
		require.NoError(t, err)
		gotNode, err := xmlquery.Query(rootNode, xpath)
		require.NoError(t, err)
		got := gotNode.InnerText()
		require.Equal(t, excepts[i], got)
	}
}
