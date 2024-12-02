package httptpl

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

/*
Test case
*/

func TestNewNucleiDSLSandbox(t *testing.T) {
	results, err := NewNucleiDSLYakSandbox().Execute("concat(abc, 1, 2, 3)", map[string]interface{}{
		"abc": "11111111",
	})
	if err != nil {
		panic(err)
	}
	if results != "11111111123" {
		panic("cancat exec error!")
	}
	spew.Dump([]byte(results.(string)))
}

func TestNewNucleiDSLYakSandbox1(t *testing.T) {
	box := NewNucleiDSLYakSandbox()
	results := box.GetUndefinedVarNames("contains(concat(abc, 1, 2, 3), `11123`)", nil)
	spew.Dump(results)
	if len(results) <= 0 {
		panic(1)
	}
}

func TestNewNucleiDSLSandbox2(t *testing.T) {
	box := NewNucleiDSLYakSandbox()
	results, err := box.ExecuteAsBool("contains(concat(abc, 1, 2, 3), `11123`)", map[string]interface{}{
		"abc": "11111111",
	})
	if err != nil {
		panic(err)
	}
	if !results {
		panic("contains failed")
	}
	spew.Dump(results)
}

// cve-2016-3347
func TestNewNucleiDSLSandbox3(t *testing.T) {
	raw := `base64(concat(base64_decode("QUVTL0NCQy9QS0NTNVBhZA=="),aes_cbc(base64_decode(generate_java_gadget("dns", "http://{{interactsh-url}}", "base64")), base64_decode("kPH+bIxk5D2deZiIxcaaaA=="), base64_decode("QUVTL0NCQy9QS0NTNVBhZA=="))))`
	box := NewNucleiDSLYakSandbox()
	results, err := box.Execute(raw, map[string]interface{}{})
	if err != nil {
		panic(err)
	}
	spew.Dump(results)
}

func TestNewNucleiDSLSandbox_GenerateJWT(t *testing.T) {
	t.Run("generate_jwt usually", func(t *testing.T) {
		raw := `generate_jwt("{\"name\":\"John Doe\",\"foo\":\"bar\"}", "HS256", "hello-world")`
		box := NewNucleiDSLYakSandbox()
		results, err := box.Execute(raw, map[string]interface{}{})
		if err != nil {
			panic(err)
		}
		if toString(results) != "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYW1lIjoiSm9obiBEb2UifQ.EsrL8lIcYJR_Ns-JuhF3VCllCP7xwbpMCCfHin_WT6U" {
			panic("generate_jwt failed")
		}
	})

	t.Run("generate_jwt with none alg", func(t *testing.T) {
		raw := `generate_jwt("{\"name\":\"John Doe\",\"foo\":\"bar\"}", "")`
		box := NewNucleiDSLYakSandbox()
		results, err := box.Execute(raw, map[string]interface{}{})
		if err != nil {
			panic(err)
		}
		if toString(results) != "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJmb28iOiJiYXIiLCJuYW1lIjoiSm9obiBEb2UifQ." {
			panic("generate_jwt failed")
		}
	})

}

func TestNewNucleiDSLSandbox2_Response(t *testing.T) {
	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 
Connection: close
Accept-Ranges: bytes
Content-Type: text/html
Date: Fri, 24 Feb 2023 05:29:40 GMT
Etag: W/"202-1254499436000"
Last-Modified: Fri, 02 Oct 2009 16:03:56 GMT
Content-Length: 202

<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.0 Transitional//EN">
<html>
<head>
    <META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

<body>
<p>Loading ...</p>
</body>
</html>
`))
	var a = LoadVarFromRawResponse(rsp, 0)
	spew.Dump(a)
}
