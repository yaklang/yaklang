package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"testing"
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
