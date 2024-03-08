package tests

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
	"testing"
)

//go:embed image.pcapng
var sample1043 []byte

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
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatal("expect http://localhost/example/shell.jsp count != 5")
	}
}
