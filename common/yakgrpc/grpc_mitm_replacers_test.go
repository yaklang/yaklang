package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	uri int = 1 << iota
	header
	body
	request
	response
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

	extractedData := replacer.hookColor(requestBytes, []byte(""), req, &schema.HTTPFlow{})
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

	extractedData := replacer.hookColor([]byte(""), responseBytes, req, &schema.HTTPFlow{})
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
Content-Length: 11

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
				extractedData := replacer.hookColor([]byte(reqRaw), responseBytes, req, &schema.HTTPFlow{})
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
		EnableForURI:      true,
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
			extractedData := replacer.hookColor([]byte(reqRaw), responseBytes, req, &schema.HTTPFlow{})
			assert.Equal(t, extractedData[0].Data, testCase.expect)
		})
	}
}

// TestReplaceWithScope verify the replacer can replace the matched content in the right scope
// Since matching and replacement are two implementation methods, the scope of replacement needs to be tested.
func TestGRPCMUSTPASS_ReplaceWithScope(t *testing.T) {
	replaceOkFlag := fmt.Sprintf("===%s===", utils.RandStringBytes(8))

	responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
testHeaderRsp: xxx
Content-Length: 11

testBodyRsp`)
	reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
testHeader: xxx
Accept-Language: zh-CN,zh;q=0.9
Content-Length: 23

testBody`
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
		},
		{
			name:   "test match response uri body header",
			flag:   response | uri | body | header,
			expect: ",testHeaderRsp,testBodyRsp",
		},
		{
			name:   "test match response uri",
			flag:   response | uri | header,
			expect: ",testHeaderRsp,",
		},
		// response has no uri
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
				NoReplace:   false,
				Result:      replaceOkFlag,
				Color:       "",
				Index:       0,
				ExtraTag:    nil,
				Disabled:    false,
				VerboseName: "",
			}
			ConfigRuleByFlags(rule, testCase.flag)
			expcetItems := strings.Split(testCase.expect, ",")
			var expectLines [3]int
			for i, item := range expcetItems {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				switch item {
				case "testUri":
					expectLines[i] = 1
				case "testHeader":
					expectLines[i] = 4
				case "testBody":
					expectLines[i] = 8
				case "testHeaderRsp":
					expectLines[i] = 4
				case "testBodyRsp":
					expectLines[i] = 7
				}
			}
			for i, re := range []string{"testUri\\w*", "testHeader\\w*", "testBody\\w*"} {
				rule.Rule = re
				replacer.SetRules(rule)
				var packet []byte
				if rule.EnableForRequest {
					packet = []byte(reqRaw)
				} else {
					packet = responseBytes
				}
				_, modified, _ := replacer.hook(rule.EnableForRequest, rule.EnableForResponse, packet)
				packetLines := strings.Split(string(modified), "\n")
				targetLine := expectLines[i] - 1
				if targetLine == -1 {
					assert.Equal(t, strings.Replace(string(modified), "\r\n", "\n", -1), string(packet))
				} else {
					assert.Contains(t, packetLines[targetLine], replaceOkFlag)
				}
			}
		})
	}
}

// TestGRPCMUSTPASS_ReplaceWhenMultiMatch verify the situation that replacer should replace all matched content
func TestGRPCMUSTPASS_ReplaceWhenMultiMatch(t *testing.T) {
	flag := "==ok=="
	reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
testHeader: xxx
Accept-Language: zh-CN,zh;q=0.9
Content-Length: 23

testBody`
	replacer := NewMITMReplacer()
	rule := &ypb.MITMContentReplacer{
		Rule:        `test.*`,
		NoReplace:   false,
		Result:      flag,
		Color:       "",
		Index:       0,
		ExtraTag:    nil,
		Disabled:    false,
		VerboseName: "",
	}
	for _, testCase := range []struct {
		name       string
		flags      int
		expectLine [3]int
	}{
		{
			name:       "test match uri",
			flags:      request | uri,
			expectLine: [3]int{1, 0, 0},
		},
		{
			name:       "test match header",
			flags:      request | header,
			expectLine: [3]int{1, 4, 0},
		},
		{
			name:       "test match body",
			flags:      request | body,
			expectLine: [3]int{0, 0, 8},
		},
		{
			name:       "test match packet",
			flags:      request | header | body,
			expectLine: [3]int{1, 4, 8},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ConfigRuleByFlags(rule, testCase.flags)
			replacer.SetRules(rule)
			_, modifiedPacket, _ := replacer.hook(true, false, []byte(reqRaw))
			packetLines := strings.Split(string(modifiedPacket), "\n")
			p := 0
			m := map[int]struct{}{}
			for _, l := range testCase.expectLine {
				m[l-1] = struct{}{}
			}
			for i, line := range packetLines {
				if _, ok := m[i]; ok {
					assert.Contains(t, line, flag)
					p++
				} else {
					assert.NotContains(t, line, flag)
				}
			}
		})
	}
}

// TestExtraHeaders if header content the config header, replace it. Otherwise, add the config header
func TestExtraHeaders(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              `POST`,
		NoReplace:         false,
		Result:            ``,
		Color:             "",
		EnableForResponse: false,
		EnableForRequest:  true,
		EnableForHeader:   true,
		EnableForBody:     true,
		Index:             0,
		ExtraTag:          nil,
		Disabled:          false,
		VerboseName:       "",
		ExtraHeaders: []*ypb.HTTPHeader{
			{
				Header: "aa",
				Value:  "a",
			},
		},
	})
	reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
aa: 1`
	_, modifiedPacket, _ := replacer.hook(true, false, []byte(reqRaw))
	assert.NotContains(t, string(modifiedPacket), "aa: 1")
	assert.Contains(t, string(modifiedPacket), "aa: a")
	reqRaw = `POST /testUri HTTP/1.1
Host: www.baidu.com`
	_, modifiedPacket, _ = replacer.hook(true, false, []byte(reqRaw))
	assert.NotContains(t, string(modifiedPacket), "aa: 1")
	assert.Contains(t, string(modifiedPacket), "aa: a")
}

// TestExtraCookie if cookie content the config cookie, replace it. Otherwise, add the config cookie
func TestExtraCookie(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              `POST`,
		NoReplace:         false,
		Result:            ``,
		Color:             "",
		EnableForResponse: false,
		EnableForRequest:  true,
		EnableForHeader:   true,
		EnableForBody:     true,
		Index:             0,
		ExtraTag:          nil,
		Disabled:          false,
		VerboseName:       "",
		ExtraCookies: []*ypb.HTTPCookieSetting{
			{
				Key:          "aa",
				Value:        "a",
				Path:         "/",
				Domain:       "www.baidu.com",
				Expires:      123,
				MaxAge:       123,
				Secure:       true,
				HttpOnly:     true,
				SameSiteMode: "strict",
			},
		},
	})
	reqRaw := `POST /testUri HTTP/1.1
Host: www.baidu.com
Cookie: cc=1`
	_, modifiedPacket, _ := replacer.hook(true, false, []byte(reqRaw))
	assert.Contains(t, string(modifiedPacket), "Cookie: cc=1; aa=a")
	reqRaw = `POST /testUri HTTP/1.1
Host: www.baidu.com
`
	_, modifiedPacket, _ = replacer.hook(true, false, []byte(reqRaw))
	assert.Contains(t, string(modifiedPacket), "Cookie: aa=a")
}

// TestMatchPatternMatchHeaderAndBody verify the situation that pattern matched data content header and body
func TestMatchPatternMatchHeaderAndBody(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              "\r\n\r\n.*",
		NoReplace:         false,
		Result:            `==ok==`,
		Color:             "",
		EnableForResponse: false,
		EnableForRequest:  true,
		EnableForHeader:   true,
		EnableForBody:     true,
		Index:             0,
		ExtraTag:          nil,
		Disabled:          false,
		VerboseName:       "",
	})
	reqRaw := lowhttp.FixHTTPRequest([]byte(`POST /testUri HTTP/1.1
Host: www.baidu.com
header: 1

body
`))
	_, modifiedPacket, _ := replacer.hook(true, false, []byte(reqRaw))
	require.Contains(t, string(modifiedPacket), "Content-Length: 6==ok==")
	// if !strings.Contains(string(modifiedPacket), "Content-Length: 6==ok==\n") {
	// 	t.Fatalf("replace failed: %s", string(modifiedPacket))
	// }
}

func ConfigRuleByFlags(rule *ypb.MITMContentReplacer, ruleFlag int) {
	rule.EnableForRequest = ruleFlag&request != 0
	rule.EnableForResponse = ruleFlag&response != 0
	rule.EnableForHeader = ruleFlag&header != 0
	rule.EnableForBody = ruleFlag&body != 0
	rule.EnableForURI = ruleFlag&uri != 0
}

func TestGRPCMUSTPASS_HookColorWithRequestAndResponse(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              `test`,
		NoReplace:         true,
		Result:            ``,
		Color:             "",
		EnableForRequest:  true,
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
Content-Length: 4

test`)
	req, err := http.NewRequest("GET", "https://www.baidu.com?a=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	reqRaw, err := utils.DumpHTTPRequest(req, true)
	require.NoError(t, err)

	extractedData := replacer.hookColor(reqRaw, responseBytes, req, &schema.HTTPFlow{})
	require.Len(t, extractedData, 2)
}

func TestGRPCMUSTPASS_HookColorOffset(t *testing.T) {
	headerBytes, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
Content-Length: 4`))
	bodyBytes := []byte(`test`)
	responseBytes := []byte(fmt.Sprintf("%s%s", headerBytes, bodyBytes))
	req, err := http.NewRequest("POST", "https://www.baidu.com?a=test", bytes.NewBuffer([]byte("test")))
	if err != nil {
		t.Fatal(err)
	}
	reqRaw, err := utils.DumpHTTPRequest(req, true)
	require.NoError(t, err)
	reqHeaderRaw, _ := lowhttp.SplitHTTPPacketFast(reqRaw)

	testOffset := func(t *testing.T, name string, rule *ypb.MITMContentReplacer, wantLen, wantOffset int) {
		replacer := NewMITMReplacer()
		replacer.SetRules(rule)
		extractedData := replacer.hookColor(reqRaw, responseBytes, req, &schema.HTTPFlow{})
		require.Lenf(t, extractedData, wantLen, "testcase name: %s", name)
		require.Equalf(t, wantOffset, extractedData[0].DataIndex, "testcase name: %s", name)
	}
	testOffset(t,
		"URI",
		&ypb.MITMContentReplacer{
			Rule:             `\/\?a=test`,
			NoReplace:        true,
			Result:           ``,
			Color:            "",
			EnableForRequest: true,
			EnableForURI:     true,
			Index:            0,
			ExtraTag:         nil,
			Disabled:         false,
			VerboseName:      "",
		}, 1, len("POST")+1)

	testOffset(t,
		"Response Body", &ypb.MITMContentReplacer{
			Rule:              `test`,
			NoReplace:         true,
			Result:            ``,
			Color:             "",
			EnableForResponse: true,
			EnableForBody:     true,
			Index:             0,
			ExtraTag:          nil,
			Disabled:          false,
			VerboseName:       "",
		}, 1, len(headerBytes))
	testOffset(t, "Request Body", &ypb.MITMContentReplacer{
		Rule:             `test`,
		NoReplace:        true,
		Result:           ``,
		Color:            "",
		EnableForRequest: true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	}, 1, len(reqHeaderRaw))
}

var defaultRule = "[\n    {\n        \"Rule\": \"(?i)(jsonp_[a-z0-9]+)|((_?callback|_cb|_call|_?jsonp_?)=)\",\n        \"NoReplace\": true,\n        \"Color\": \"yellow\",\n        \"EnableForRequest\": true,\n        \"EnableForHeader\": true,\n        \"Index\": 1,\n        \"ExtraTag\": [\n            \"疑似JSONP\"\n        ]\n    },\n    {\n        \"Rule\": \"(?i)((password)|(pass)|(secret)|(mima))['\\\"]?\\\\s*[\\\\:\\\\=]\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 2,\n        \"ExtraTag\": [\n            \"登陆/密码传输\"\n        ]\n    },\n    {\n        \"Rule\": \"(?i)((access|admin|api|debug|auth|authorization|gpg|ops|ray|deploy|s3|certificate|aws|app|application|docker|es|elastic|elasticsearch|secret)[-_]{0,5}(key|token|secret|secretkey|pass|password|sid|debug))|(secret|password)([\\\"']?\\\\s*:\\\\s*|\\\\s*=\\\\s*)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 3,\n        \"ExtraTag\": [\n            \"敏感信息\"\n        ]\n    },\n    {\n        \"Rule\": \"(BEGIN PUBLIC KEY).*?(END PUBLIC KEY)\",\n        \"NoReplace\": true,\n        \"Color\": \"purple\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 4,\n        \"ExtraTag\": [\n            \"公钥传输\"\n        ]\n    },\n    {\n        \"Rule\": \"(?is)(\\u003cform.*type=.*?text.*?type=.*?password.*?\\u003c/form.*?\\u003e)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 5,\n        \"ExtraTag\": [\n            \"登陆点\"\n        ],\n        \"VerboseName\": \"登陆点\"\n    },\n    {\n        \"Rule\": \"(?is)(\\u003cform.*type=.*?text.*?type=.*?password.*?onclick=.*?\\u003c/form.*?\\u003e)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 6,\n        \"ExtraTag\": [\n            \"登陆（验证码）\"\n        ],\n        \"VerboseName\": \"登陆（验证码）\"\n    },\n    {\n        \"Rule\": \"(?is)\\u003cform.*enctype=.*?multipart/form-data.*?type=.*?file.*?\\u003c/form\\u003e\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 7,\n        \"ExtraTag\": [\n            \"文件上传点\"\n        ],\n        \"VerboseName\": \"文件上传点\"\n    },\n    {\n        \"Rule\": \"(file=|path=|url=|lang=|src=|menu=|meta-inf=|web-inf=|filename=|topic=|page=｜_FilePath=|target=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 8,\n        \"ExtraTag\": [\n            \"文件包含参数\"\n        ],\n        \"VerboseName\": \"文件包含参数\"\n    },\n    {\n        \"Rule\": \"((cmd=)|(exec=)|(command=)|(execute=)|(ping=)|(query=)|(jump=)|(code=)|(reg=)|(do=)|(func=)|(arg=)|(option=)|(load=)|(process=)|(step=)|(read=)|(function=)|(feature=)|(exe=)|(module=)|(payload=)|(run=)|(daemon=)|(upload=)|(dir=)|(download=)|(log=)|(ip=)|(cli=))|(ipaddress=)|(txt=)|(case=)|(count=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 9,\n        \"ExtraTag\": [\n            \"命令注入参数\"\n        ],\n        \"VerboseName\": \"命令注入参数\"\n    },\n    {\n        \"Rule\": \"\\\\b(([^\\u003c\\u003e()[\\\\]\\\\\\\\.,;:\\\\s@\\\"]+(\\\\.[^\\u003c\\u003e()[\\\\]\\\\\\\\.,;:\\\\s@\\\"]+)*)|(\\\".+\\\"))@((\\\\[[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}\\\\])|(([a-zA-Z\\\\-0-9]+\\\\.)+(cn|com|edu|gov|int|mil|net|org|biz|info|pro|name|museum|coop|aero|xxx|idv)))\\\\b\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 10,\n        \"ExtraTag\": [\n            \"email泄漏\"\n        ],\n        \"VerboseName\": \"email泄漏\"\n    },\n    {\n        \"Rule\": \"\\\\b(?:(?:\\\\+|00)86)?1(?:(?:3[\\\\d])|(?:4[5-79])|(?:5[0-35-9])|(?:6[5-7])|(?:7[0-8])|(?:8[\\\\d])|(?:9[189]))\\\\d{8}\\\\b\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 11,\n        \"ExtraTag\": [\n            \"手机号泄漏\"\n        ],\n        \"VerboseName\": \"手机号泄漏\"\n    },\n    {\n        \"Rule\": \"((\\\\[client\\\\])|\\\\[(mysql\\\\])|(\\\\[mysqld\\\\]))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 12,\n        \"ExtraTag\": [\n            \"MySQL配置\"\n        ],\n        \"VerboseName\": \"MySQL配置\"\n    },\n    {\n        \"Rule\": \"\\\\b[1-9]\\\\d{5}(?:18|19|20)\\\\d{2}(?:0[1-9]|10|11|12)(?:0[1-9]|[1-2]\\\\d|30|31)\\\\d{3}[\\\\dXx]\\\\b\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 13,\n        \"ExtraTag\": [\n            \"身份证\"\n        ],\n        \"VerboseName\": \"身份证\"\n    },\n    {\n        \"Rule\": \"[-]+BEGIN [^\\\\s]+ PRIVATE KEY[-]\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 14,\n        \"ExtraTag\": [\n            \"RSA私钥\"\n        ],\n        \"VerboseName\": \"RSA私钥\"\n    },\n    {\n        \"Rule\": \"([A|a]ccess[K|k]ey[S|s]ecret)|([A|a]ccess[K|k]ey[I|i][d|D])|([Aa](ccess|CCESS)_?[Kk](ey|EY))|([Aa](ccess|CCESS)_?[sS](ecret|ECRET))|(([Aa](ccess|CCESS)_?(id|ID|Id)))|([Ss](ecret|ECRET)_?[Kk](ey|EY))\",\n        \"NoReplace\": true,\n        \"Color\": \"yellow\",\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 15,\n        \"ExtraTag\": [\n            \"OSS Key\"\n        ],\n        \"VerboseName\": \"OSS Key\"\n    },\n    {\n        \"Rule\": \"[\\\\w-.]+\\\\.oss\\\\.aliyuncs\\\\.com\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 16,\n        \"ExtraTag\": [\n            \"AliyunOSS\"\n        ],\n        \"VerboseName\": \"AliyunOSS\"\n    },\n    {\n        \"Rule\": \"\\\\b((127\\\\.0\\\\.0\\\\.1)|(localhost)|(10\\\\.\\\\d{1,3}\\\\.\\\\d{1,3}\\\\.\\\\d{1,3})|(172\\\\.((1[6-9])|(2\\\\d)|(3[01]))\\\\.\\\\d{1,3}\\\\.\\\\d{1,3})|(192\\\\.168\\\\.\\\\d{1,3}\\\\.\\\\d{1,3}))\\\\b\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 17,\n        \"ExtraTag\": [\n            \"IP地址\"\n        ],\n        \"VerboseName\": \"IP地址\"\n    },\n    {\n        \"Rule\": \"(=deleteMe|rememberMe=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"Index\": 18,\n        \"ExtraTag\": [\n            \"Shiro\"\n        ],\n        \"VerboseName\": \"Shiro\"\n    },\n    {\n        \"Rule\": \"(?is)^{.*}$\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 19,\n        \"ExtraTag\": [\n            \"JSON传输\"\n        ],\n        \"VerboseName\": \"JSON传输\"\n    },\n    {\n        \"Rule\": \"(?is)^\\u003c\\\\?xml.*\\u003csoap:Body\\u003e\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForBody\": true,\n        \"Index\": 20,\n        \"ExtraTag\": [\n            \"SOAP请求\"\n        ],\n        \"VerboseName\": \"SOAP请求\"\n    },\n    {\n        \"Rule\": \"(?is)^\\u003c\\\\?xml.*\\u003e$\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForBody\": true,\n        \"Index\": 21,\n        \"ExtraTag\": [\n            \"XML请求\"\n        ],\n        \"VerboseName\": \"XML请求\"\n    },\n    {\n        \"Rule\": \"(?i)(Authorization: .*)|(www-Authenticate: ((Basic)|(Bearer)|(Digest)|(HOBA)|(Mutual)|(Negotiate)|(OAuth)|(SCRAM-SHA-1)|(SCRAM-SHA-256)|(vapid)))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"Index\": 22,\n        \"ExtraTag\": [\n            \"HTTP认证头\"\n        ],\n        \"VerboseName\": \"HTTP认证头\"\n    },\n    {\n        \"Rule\": \"(GET.*\\\\w+=\\\\w+)|(?is)(POST.*\\\\n\\\\n.*\\\\w+=\\\\w+)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 23,\n        \"ExtraTag\": [\n            \"SQL注入测试点\"\n        ],\n        \"VerboseName\": \"SQL注入测试点\"\n    },\n    {\n        \"Rule\": \"(GET.*\\\\w+=\\\\w+)|(?is)(POST.*\\\\n\\\\n.*\\\\w+=\\\\w+)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 24,\n        \"ExtraTag\": [\n            \"XPath注入测试点\"\n        ],\n        \"VerboseName\": \"XPath注入测试点\"\n    },\n    {\n        \"Rule\": \"((POST.*?wsdl)|(GET.*?wsdl)|(xml=)|(\\u003c\\\\?xml )|(\\u0026lt;\\\\?xml))|((POST.*?asmx)|(GET.*?asmx))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 25,\n        \"ExtraTag\": [\n            \"XXE测试点\"\n        ],\n        \"VerboseName\": \"XXE测试点\"\n    },\n    {\n        \"Rule\": \"(file=|path=|url=|lang=|src=|menu=|meta-inf=|web-inf=|filename=|topic=|page=｜_FilePath=|target=｜filepath=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 26,\n        \"ExtraTag\": [\n            \"文件下载参数\"\n        ],\n        \"VerboseName\": \"文件下载参数\"\n    },\n    {\n        \"Rule\": \"((ueditor\\\\.(config|all)\\\\.js))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 27,\n        \"ExtraTag\": [\n            \"UEditor测试点\"\n        ],\n        \"VerboseName\": \"UEditor测试点\"\n    },\n    {\n        \"Rule\": \"(kindeditor\\\\-(all\\\\-min|all)\\\\.js)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 28,\n        \"ExtraTag\": [\n            \"KindEditor测试点\"\n        ],\n        \"VerboseName\": \"KindEditor测试点\"\n    },\n    {\n        \"Rule\": \"((callback=)|(url=)|(request=)|(redirect_to=)|(jump=)|(to=)|(link=)|(domain=))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 29,\n        \"ExtraTag\": [\n            \"Url重定向参数\"\n        ],\n        \"VerboseName\": \"Url重定向参数\"\n    },\n    {\n        \"Rule\": \"(wap=|url=|link=|src=|source=|display=|sourceURl=|imageURL=|domain=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 30,\n        \"ExtraTag\": [\n            \"SSRF测试参数\"\n        ],\n        \"VerboseName\": \"SSRF测试参数\"\n    },\n    {\n        \"Rule\": \"((GET|POST|http[s]?).*\\\\.(do|action))[^a-zA-Z]\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 31,\n        \"ExtraTag\": [\n            \"Struts2测试点\"\n        ],\n        \"VerboseName\": \"Struts2测试点\"\n    },\n    {\n        \"Rule\": \"((GET|POST|http[s]?).*?\\\\?.*?(token=|session\\\\w+=))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 32,\n        \"ExtraTag\": [\n            \"Session/Token测试点\"\n        ],\n        \"VerboseName\": \"Session/Token测试点\"\n    },\n    {\n        \"Rule\": \"((AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[a-zA-Z0-9]{16})\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 33,\n        \"ExtraTag\": [\n            \"Amazon AK\"\n        ],\n        \"VerboseName\": \"Amazon AK\"\n    },\n    {\n        \"Rule\": \"(Directory listing for|Parent Directory|Index of|folder listing:)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 34,\n        \"ExtraTag\": [\n            \"目录枚举点\"\n        ],\n        \"VerboseName\": \"目录枚举点\"\n    },\n    {\n        \"Rule\": \"(\\u003c.*?Unauthorized)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 35,\n        \"ExtraTag\": [\n            \"非授权页面点\"\n        ],\n        \"VerboseName\": \"非授权页面点\"\n    },\n    {\n        \"Rule\": \"((\\\"|')?[u](ser|name|ame|sername)(\\\"|'|\\\\s)?(:|=).*?,)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 36,\n        \"ExtraTag\": [\n            \"用户名泄漏点\"\n        ],\n        \"VerboseName\": \"用户名泄漏点\"\n    },\n    {\n        \"Rule\": \"((\\\"|')?[p](ass|wd|asswd|assword)(\\\"|'|\\\\s)?(:|=).*?,)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 37,\n        \"ExtraTag\": [\n            \"密码泄漏点\"\n        ],\n        \"VerboseName\": \"密码泄漏点\"\n    },\n    {\n        \"Rule\": \"(((([a-zA-Z0-9._-]+\\\\.s3|s3)(\\\\.|\\\\-)+[a-zA-Z0-9._-]+|[a-zA-Z0-9._-]+\\\\.s3|s3)\\\\.amazonaws\\\\.com)|(s3:\\\\/\\\\/[a-zA-Z0-9-\\\\.\\\\_]+)|(s3.console.aws.amazon.com\\\\/s3\\\\/buckets\\\\/[a-zA-Z0-9-\\\\.\\\\_]+)|(amzn\\\\.mws\\\\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})|(ec2-[0-9-]+.cd-[a-z0-9-]+.compute.amazonaws.com)|(us[_-]?east[_-]?1[_-]?elb[_-]?amazonaws[_-]?com))\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 38,\n        \"ExtraTag\": [\n            \"Amazon AWS URL\"\n        ],\n        \"VerboseName\": \"Amazon AWS URL\"\n    },\n    {\n        \"Rule\": \"(?is)(\\u003cform.*type=.*?text.*?\\u003c/form.*?\\u003e)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 39,\n        \"ExtraTag\": [\n            \"HTTP XSS测试点\"\n        ],\n        \"VerboseName\": \"HTTP XSS测试点\"\n    },\n    {\n        \"Rule\": \"(?i)(\\u003ctitle\\u003e.*?(后台|admin).*?\\u003c/title\\u003e)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 40,\n        \"ExtraTag\": [\n            \"后台登陆\"\n        ],\n        \"VerboseName\": \"后台登陆\"\n    },\n    {\n        \"Rule\": \"((ghp|ghu)\\\\_[a-zA-Z0-9]{36})\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 41,\n        \"ExtraTag\": [\n            \"GithubAccessToken\"\n        ],\n        \"VerboseName\": \"GithubAccessToken\"\n    },\n    {\n        \"Rule\": \"((access=)|(adm=)|(admin=)|(alter=)|(cfg=)|(clone=)|(config=)|(create=)|(dbg=)|(debug=)|(delete=)|(disable=)|(edit=)|(enable=)|(exec=)|(execute=)|(grant=)|(load=)|(make=)|(modify=)|(rename=)|(reset=)|(root=)|(shell=)|(test=)|(toggl=))\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 42,\n        \"ExtraTag\": [\n            \"调试参数\"\n        ],\n        \"VerboseName\": \"调试参数\"\n    },\n    {\n        \"Rule\": \"(jdbc:[a-z:]+://[A-Za-z0-9\\\\.\\\\-_:;=/@?,\\u0026]+)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 43,\n        \"ExtraTag\": [\n            \"JDBC连接参数\"\n        ],\n        \"VerboseName\": \"JDBC连接参数\"\n    },\n    {\n        \"Rule\": \"(ey[A-Za-z0-9_-]{10,}\\\\.[A-Za-z0-9._-]{10,}|ey[A-Za-z0-9_\\\\/+-]{10,}\\\\.[A-Za-z0-9._\\\\/+-]{10,})\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 44,\n        \"ExtraTag\": [\n            \"JWT 测试点\"\n        ],\n        \"VerboseName\": \"JWT 测试点\"\n    },\n    {\n        \"Rule\": \"(?i)(jsonp_[a-z0-9]+)|((_?callback|_cb|_call|_?jsonp_?)=)\",\n        \"NoReplace\": true,\n        \"Color\": \"green\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 45,\n        \"ExtraTag\": [\n            \"JSONP 测试点\"\n        ],\n        \"VerboseName\": \"jsonp_pre_test\"\n    },\n    {\n        \"Rule\": \"([c|C]or[p|P]id|[c|C]orp[s|S]ecret)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 46,\n        \"ExtraTag\": [\n            \"Wecom Key(Secret)\"\n        ],\n        \"VerboseName\": \"Wecom Key(Secret)\"\n    },\n    {\n        \"Rule\": \"(https://outlook\\\\.office\\\\.com/webhook/[a-z0-9@-]+/IncomingWebhook/[a-z0-9-]+/[a-z0-9-]+)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 47,\n        \"ExtraTag\": [\n            \"MicrosoftTeams Webhook\"\n        ],\n        \"VerboseName\": \"MicrosoftTeams Webhook\"\n    },\n    {\n        \"Rule\": \"https://creator\\\\.zoho\\\\.com/api/[A-Za-z0-9/\\\\-_\\\\.]+\\\\?authtoken=[A-Za-z0-9]+\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 48,\n        \"ExtraTag\": [\n            \"Zoho Webhook\"\n        ],\n        \"VerboseName\": \"Zoho Webhook\"\n    },\n    {\n        \"Rule\": \"([a-zA-Z]:\\\\\\\\(\\\\w+\\\\\\\\)+|[a-zA-Z]:\\\\\\\\\\\\\\\\(\\\\w+\\\\\\\\\\\\\\\\)+)|(/(bin|dev|home|media|opt|root|sbin|sys|usr|boot|data|etc|lib|mnt|proc|run|srv|tmp|var)/[^\\u003c\\u003e()[\\\\],;:\\\\s\\\"]+/)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForBody\": true,\n        \"Index\": 49,\n        \"ExtraTag\": [\n            \"操作系统路径\"\n        ],\n        \"VerboseName\": \"操作系统路径\"\n    },\n    {\n        \"Rule\": \"(javax\\\\.faces\\\\.ViewState)\",\n        \"NoReplace\": true,\n        \"Color\": \"blue\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 50,\n        \"ExtraTag\": [\n            \"Java反序列化测试点\"\n        ],\n        \"VerboseName\": \"Java反序列化测试点\"\n    },\n    {\n        \"Rule\": \"(sonar.{0,50}(?:\\\"|\\\\'|`)?[0-9a-f]{40}(?:\\\"|\\\\'|`)?)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 51,\n        \"ExtraTag\": [\n            \"Sonarqube Token\"\n        ],\n        \"VerboseName\": \"Sonarqube Token\"\n    },\n    {\n        \"Rule\": \"((us(-gov)?|ap|ca|cn|eu|sa)-(central|(north|south)?(east|west)?)-\\\\d)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 52,\n        \"ExtraTag\": [\n            \"Amazon AWS Region泄漏\"\n        ],\n        \"VerboseName\": \"Amazon AWS Region泄漏\"\n    },\n    {\n        \"Rule\": \"(=(https?://.*|https?%3(a|A)%2(f|F)%2(f|F).*))\",\n        \"NoReplace\": true,\n        \"Color\": \"blue\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 53,\n        \"ExtraTag\": [\n            \"URL作为参数\"\n        ],\n        \"VerboseName\": \"URL作为参数\"\n    },\n    {\n        \"Rule\": \"(ya29\\\\.[0-9A-Za-z_-]+)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 54,\n        \"ExtraTag\": [\n            \"Oauth Access Key\"\n        ],\n        \"VerboseName\": \"Oauth Access Key\"\n    },\n    {\n        \"Rule\": \"(Error report|in your SQL syntax|mysql_fetch_array|mysql_connect()|org.apache.catalina)\",\n        \"NoReplace\": true,\n        \"Color\": \"red\",\n        \"EnableForRequest\": true,\n        \"EnableForResponse\": true,\n        \"EnableForHeader\": true,\n        \"EnableForBody\": true,\n        \"Index\": 55,\n        \"ExtraTag\": [\n            \"网站出错\"\n        ],\n        \"VerboseName\": \"网站出错\"\n    }\n]"

func TestGRPCMUSTPASS_HookColorTimeout(t *testing.T) {
	err := yakit.SetKey(consts.GetGormProfileDatabase(), MITMReplacerKeyRecords, defaultRule)
	if err != nil {
		return
	}
	defer yakit.DelKey(consts.GetGormProfileDatabase(), MITMReplacerKeyRecords)
	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-length:10000\r\n\r\n" + strings.Repeat("a", 10000))
	})
	testReq := []byte(`POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

{"key": "value"}`)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	time.Sleep(time.Second)
	_, err = lowhttp.HTTP(lowhttp.WithPacketBytes(testReq), lowhttp.WithProxy(fmt.Sprintf("http://127.0.0.1:%d", mitmPort)), lowhttp.WithHost(mockHost), lowhttp.WithPort(mockPort), lowhttp.WithTimeout(2*time.Second))
	require.NoError(t, err)
}
