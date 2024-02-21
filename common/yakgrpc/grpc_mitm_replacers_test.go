package yakgrpc

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HookColorWithRequest(t *testing.T) {
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

func TestGRPCMUSTPASS_HookColorWithResponse(t *testing.T) {
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

// TestMatchScope match scope rule: scope =  opt1 ∩ ( ∪ { opt2s... } ), opt1 ∈ {request, response}, opt2 ∈ {uri, header, body}
func TestGRPCMUSTPASS_MatchScope(t *testing.T) {
	const (
		uri int = 1 << iota
		header
		body
		request
		response
	)

	for _, testCase := range []struct {
		flag   int
		expect string
		name   string
	}{
		// test match request single item
		{
			name:   "test match uri",
			flag:   request | uri,
			expect: "testUri,,",
		},
		{
			name:   "test match header",
			flag:   request | header,
			expect: "testUri,testHeader,",
		},
		{
			name:   "test match body",
			flag:   request | body,
			expect: ",,testBody",
		},

		// test match request multi item
		{
			name:   "test match header and uri",
			flag:   request | uri | header,
			expect: "testUri,testHeader,",
		}, // should be same as match header
		{
			name:   "test match body and uri",
			flag:   request | uri | body,
			expect: "testUri,,testBody",
		},
		{
			name:   "test match body and header",
			flag:   request | body | header,
			expect: "testUri,testHeader,testBody",
		},
		{
			name:   "test match header and uri",
			flag:   request | uri | body | header,
			expect: "testUri,testHeader,testBody",
		}, // should be same as match header and body
		{
			name:   "test match response uri",
			flag:   response | uri,
			expect: ",,",
		}, // response has no uri
		{
			name:   "test match response header",
			flag:   response | header,
			expect: ",testHeaderRsp,",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			replacer := NewMITMReplacer()
			rule := &ypb.MITMContentReplacer{
				Rule:        `test.*`,
				NoReplace:   true,
				Result:      ``,
				Color:       "",
				Index:       0,
				ExtraTag:    nil,
				Disabled:    false,
				VerboseName: "",
			}
			ruleFlag := testCase.flag
			rule.EnableForRequest = ruleFlag&request != 0
			rule.EnableForResponse = ruleFlag&response != 0
			rule.EnableForHeader = ruleFlag&header != 0
			rule.EnableForBody = ruleFlag&body != 0
			rule.EnableForURI = ruleFlag&uri != 0

			responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
testHeaderRsp: xxx
Content-Length: 23

`)
			reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
testHeader: xxx
Accept-Language: zh-CN,zh;q=0.9

testBody`
			req, err := http.NewRequest("GET", "https://www.baidu.com/testUri", nil)
			if err != nil {
				t.Fatal(err)
			}
			var matchRes []string
			for _, re := range []string{"testUri\\w*", "testHeader\\w*", "testBody\\w*"} {
				rule.Rule = re
				replacer.SetRules(rule)
				extractedData := replacer.hookColor([]byte(reqRaw), responseBytes, req, &yakit.HTTPFlow{})
				if len(extractedData) == 1 {
					matchRes = append(matchRes, extractedData[0].Data)
				} else {
					matchRes = append(matchRes, "")
				}
			}
			assert.Equal(t, testCase.expect, strings.Join(matchRes, ","))
		})
	}
}

// TestMatchGroup if pattern has group, hookColor method should return group 1
func TestGRPCMUSTPASS_MatchGroup(t *testing.T) {
	rule := &ypb.MITMContentReplacer{
		NoReplace:         true,
		Result:            ``,
		Color:             "",
		Index:             0,
		ExtraTag:          nil,
		Disabled:          false,
		VerboseName:       "",
		EnableForHeader:   true,
		EnableForResponse: true,
	}
	replacer := NewMITMReplacer()
	responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
testHeaderRsp: xxx
Content-Length: 23

`)
	reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
testHeader: xxx
Accept-Language: zh-CN,zh;q=0.9

testBody`
	req, err := http.NewRequest("GET", "https://www.baidu.com/testUri", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name   string
		re     string
		expect string
	}{
		{
			name:   "group 0",
			re:     "test\\w+",
			expect: "testHeaderRsp",
		},
		{
			name:   "group 1",
			re:     "test(\\w+)",
			expect: "HeaderRsp",
		},
		{
			name:   "group 2",
			re:     "test(Header(\\w+))",
			expect: "HeaderRsp",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule.Rule = testCase.re
			replacer.SetRules(rule)
			extractedData := replacer.hookColor([]byte(reqRaw), responseBytes, req, &yakit.HTTPFlow{})
			assert.Equal(t, extractedData[0].Data, testCase.expect)
		})
	}
}

func TestGRPCMUSTPASS_ReplaceWithScope(t *testing.T) {
	const (
		uri int = 1 << iota
		header
		body
		request
		response
	)

	for _, testCase := range []struct {
		flag   int
		expect string
		name   string
	}{
		// test match request single item
		{
			name:   "test match uri",
			flag:   request | uri,
			expect: "testUri,,",
		},
		{
			name:   "test match header",
			flag:   request | header,
			expect: "testUri,testHeader,",
		},
		{
			name:   "test match body",
			flag:   request | body,
			expect: ",,testBody",
		},

		// test match request multi item
		{
			name:   "test match header and uri",
			flag:   request | uri | header,
			expect: "testUri,testHeader,",
		}, // should be same as match header
		{
			name:   "test match body and uri",
			flag:   request | uri | body,
			expect: "testUri,,testBody",
		},
		{
			name:   "test match body and header",
			flag:   request | body | header,
			expect: "testUri,testHeader,testBody",
		},
		{
			name:   "test match header and uri",
			flag:   request | uri | body | header,
			expect: "testUri,testHeader,testBody",
		}, // should be same as match header and body
		{
			name:   "test match response uri",
			flag:   response | uri,
			expect: ",,",
		}, // response has no uri
		{
			name:   "test match response header",
			flag:   response | header,
			expect: ",testHeaderRsp,",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			replacer := NewMITMReplacer()
			rule := &ypb.MITMContentReplacer{
				Rule:        `test.*`,
				NoReplace:   true,
				Result:      ``,
				Color:       "",
				Index:       0,
				ExtraTag:    nil,
				Disabled:    false,
				VerboseName: "",
			}
			ruleFlag := testCase.flag
			rule.EnableForRequest = ruleFlag&request != 0
			rule.EnableForResponse = ruleFlag&response != 0
			rule.EnableForHeader = ruleFlag&header != 0
			rule.EnableForBody = ruleFlag&body != 0
			rule.EnableForURI = ruleFlag&uri != 0

			responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
testHeaderRsp: xxx
Content-Length: 23

`)
			reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
testHeader: xxx
Accept-Language: zh-CN,zh;q=0.9

testBody`
			req, err := http.NewRequest("GET", "https://www.baidu.com/testUri", nil)
			if err != nil {
				t.Fatal(err)
			}
			var matchRes []string
			for _, re := range []string{"testUri\\w*", "testHeader\\w*", "testBody\\w*"} {
				rule.Rule = re
				replacer.SetRules(rule)
				extractedData := replacer.hookColor([]byte(reqRaw), responseBytes, req, &yakit.HTTPFlow{})
				if len(extractedData) == 1 {
					matchRes = append(matchRes, extractedData[0].Data)
				} else {
					matchRes = append(matchRes, "")
				}
			}
			assert.Equal(t, testCase.expect, strings.Join(matchRes, ","))
		})
	}
}
