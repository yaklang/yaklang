package yakgrpc

import (
	"net/http"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHookColorWithRequest(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `百度`,
		NoReplace:        true,
		Result:           ``,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	})
	requestBytes := []byte(`GET /content-search.xml HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36

{"product": "百度"}`)
	req, err := lowhttp.ParseBytesToHttpRequest(requestBytes)
	if err != nil {
		t.Fatal(err)
	}

	extractedData := replacer.hookColor(requestBytes, []byte(""), req, &yakit.HTTPFlow{})
	if len(extractedData) == 0 {
		t.Fatal("no data extracted")
	}

}

func TestHookColorWithResponse(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              `(?i)(access[_-]?(key|secret|id)|accesskey(secret|id)|secret[_-]?(key|id))`,
		NoReplace:         true,
		Result:            ``,
		Color:             "",
		EnableForResponse: true,
		EnableForHeader:   true,
		EnableForBody:     true,
		Index:             0,
		ExtraTag:          nil,
		Disabled:          false,
		VerboseName:       "",
	})
	responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
Content-Length: 23

secret-key:
secret-id:`)
	req, err := http.NewRequest("GET", "https://www.baidu.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	extractedData := replacer.hookColor([]byte(""), responseBytes, req, &yakit.HTTPFlow{})
	if len(extractedData) == 0 {
		t.Fatal("no data extracted")
	}
}
