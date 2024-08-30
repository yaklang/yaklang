package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
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

//go:embed default_mitm_rule
var defaultRule string

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

// TestGRPCMUSTPASS_ReplaceRuleAndMirrorRule fix the bug that the mirror rule not work when the replace rule is enable
func TestGRPCMUSTPASS_ReplaceRuleAndMirrorRule(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `百度`,
		NoReplace:        false,
		Result:           ``,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         []string{"tag1"},
		Disabled:         false,
		VerboseName:      "",
	}, &ypb.MITMContentReplacer{
		Rule:             `百度`,
		NoReplace:        true,
		Result:           ``,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         []string{"tag2"},
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
	rules, _, _ := replacer.hook(true, false, requestBytes)
	httpctx.AppendMatchedRule(req, rules...)
	flow := &schema.HTTPFlow{}
	replacer.hookColor(requestBytes, []byte(""), req, flow)
	tags := strings.Split(flow.Tags, "|")
	assert.Equal(t, 2, len(tags))
}

// TestGRPCMUSTPASS_ExtraRepeat fix the bug that the ExtraRepeat flow does not have the right tags and color
func TestGRPCMUSTPASS_ExtraRepeat(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `百度`,
		NoReplace:        false,
		Result:           ``,
		Color:            "red",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         []string{"tag1"},
		Disabled:         false,
		ExtraRepeat:      true,
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
	var tags string
	lowhttp.RegisterSaveHTTPFlowHandler(func(lowhttpResponse *lowhttp.LowhttpResponse, b bool) {
		tags = strings.Join(lowhttpResponse.Tags, "|")
	})
	replacer.hook(true, false, requestBytes)
	replacer.WaitTasks()
	assert.Equal(t, "[重发]tag1|YAKIT_COLOR_red", tags)
}
