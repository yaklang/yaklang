package tests

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"testing"
	"time"
)

//go:embed image.pcapng
var sample1043 []byte

//go:embed aes_wtih_magic.pcapng
var sample1043_2 []byte

func TestIssue1043_2(t *testing.T) {
	for i := 0; i < 200; i++ {
		subtestIssue1043_2(t)
	}
}

func subtestIssue1043_2(t *testing.T) {
	filename := consts.TempFileFast(sample1043_2)
	var count = 0
	err := pcaputil.OpenPcapFile(
		filename,
		pcaputil.WithHTTPFlow(func(flow *pcaputil.TrafficFlow, req *http.Request, rsp *http.Response) {
			if req == nil {
				return
			}

			u, _ := lowhttp.ExtractURLFromHTTPRequest(req, false)
			if u == nil {
				return
			}

			urlStr := u.String()
			reqRaw, _ := utils.DumpHTTPRequest(req, true)
			method := lowhttp.GetHTTPRequestMethod(reqRaw)
			rspRaw, _ := utils.DumpHTTPResponse(rsp, true)
			code := lowhttp.ExtractStatusCodeFromResponse(rspRaw)
			fmt.Printf("%v [%v] %v\n", code, method, urlStr)
			reqTs := httpctx.GetRequestTimestamp(req)
			rspTs := httpctx.GetResponseTimestamp(rsp)

			nowBase := time.Now().Add(time.Minute)
			if reqTs.After(nowBase) {
				fmt.Println("reqTs", reqTs)
				fmt.Println("rspTs", rspTs)
				t.Fatal("reqTs > nowBase")
			}
			if rspTs.After(nowBase) {
				fmt.Println("reqTs", reqTs)
				fmt.Println("rspTs", rspTs)
				t.Fatal("rspTs > nowBase")
			}

			fmt.Printf(
				"    request ts: %v\n    response ts: %v\n\n",
				httpctx.GetRequestTimestamp(req).String(),
				httpctx.GetResponseTimestamp(rsp).String(),
			)
			count++
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("expect count != 2")
	}
}

func TestIssue1043(t *testing.T) {
	filename := consts.TempFileFast(sample1043)
	count := 0
	err := pcaputil.OpenPcapFile(
		filename, pcaputil.WithHTTPFlow(func(flow *pcaputil.TrafficFlow, req *http.Request, rsp *http.Response) {
			if req == nil {
				return
			}

			u, _ := lowhttp.ExtractURLFromHTTPRequest(req, false)
			if u == nil {
				return
			}
			urlStr := u.String()
			if !utils.IContains(urlStr, "shell.jsp") {
				return
			}
			rspRaw, _ := utils.DumpHTTPResponse(rsp, true)
			code := lowhttp.ExtractStatusCodeFromResponse(rspRaw)
			fmt.Println(urlStr + " RESPONSE: " + fmt.Sprint(code))
			if urlStr != "" && code == 200 {
				count++
			}

			reqTs := httpctx.GetRequestTimestamp(req)
			rspTs := httpctx.GetResponseTimestamp(rsp)

			nowBase := time.Now().Add(time.Minute)
			if reqTs.After(nowBase) {
				fmt.Println("reqTs", reqTs)
				fmt.Println("rspTs", rspTs)
				t.Fatal("reqTs > nowBase")
			}
			if rspTs.After(nowBase) {
				fmt.Println("reqTs", reqTs)
				fmt.Println("rspTs", rspTs)
				t.Fatal("rspTs > nowBase")
			}

			fmt.Printf("%v %v\n", code, urlStr)
			fmt.Printf(
				"    request ts: %v\n    response ts: %v\n\n",
				httpctx.GetRequestTimestamp(req).String(),
				httpctx.GetResponseTimestamp(rsp).String(),
			)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatal("expect http://localhost/example/shell.jsp count != 5")
	}
}
