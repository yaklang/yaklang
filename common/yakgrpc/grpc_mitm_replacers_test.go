package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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
	replacer := yakit.NewMITMReplacer()
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

	extractedData := replacer.HookColor(requestBytes, []byte(""), req, &schema.HTTPFlow{})
	if len(extractedData) == 0 {
		t.Fatal("no data extracted")
	}
}

func TestGRPCMUSTPASS_HookColorWithResponse(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
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

	extractedData := replacer.HookColor([]byte(""), responseBytes, req, &schema.HTTPFlow{})
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
			replacer := yakit.NewMITMReplacer()
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
				extractedData := replacer.HookColor([]byte(reqRaw), responseBytes, req, &schema.HTTPFlow{})
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
	replacer := yakit.NewMITMReplacer()
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
			extractedData := replacer.HookColor([]byte(reqRaw), responseBytes, req, &schema.HTTPFlow{})
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
			replacer := yakit.NewMITMReplacer()
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
				_, modified, _ := replacer.Hook(rule.EnableForRequest, rule.EnableForResponse, "", packet)
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
	replacer := yakit.NewMITMReplacer()
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
			_, modifiedPacket, _ := replacer.Hook(true, false, "", []byte(reqRaw))
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
	replacer := yakit.NewMITMReplacer()
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
	_, modifiedPacket, _ := replacer.Hook(true, false, "", []byte(reqRaw))
	assert.NotContains(t, string(modifiedPacket), "aa: 1")
	assert.Contains(t, string(modifiedPacket), "aa: a")
	reqRaw = `POST /testUri HTTP/1.1
Host: www.baidu.com`
	_, modifiedPacket, _ = replacer.Hook(true, false, "", []byte(reqRaw))
	assert.NotContains(t, string(modifiedPacket), "aa: 1")
	assert.Contains(t, string(modifiedPacket), "aa: a")
}

// TestExtraCookie if cookie content the config cookie, replace it. Otherwise, add the config cookie
func TestExtraCookie(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
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
	_, modifiedPacket, _ := replacer.Hook(true, false, "", []byte(reqRaw))
	assert.Contains(t, string(modifiedPacket), "Cookie: cc=1; aa=a")
	reqRaw = `POST /testUri HTTP/1.1
Host: www.baidu.com
`
	_, modifiedPacket, _ = replacer.Hook(true, false, "", []byte(reqRaw))
	assert.Contains(t, string(modifiedPacket), "Cookie: aa=a")
}

// TestMatchPatternMatchHeaderAndBody verify the situation that pattern matched data content header and body
func TestMatchPatternMatchHeaderAndBody(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
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
	_, modifiedPacket, _ := replacer.Hook(true, false, "", []byte(reqRaw))
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
	replacer := yakit.NewMITMReplacer()
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

	extractedData := replacer.HookColor(reqRaw, responseBytes, req, &schema.HTTPFlow{})
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
		replacer := yakit.NewMITMReplacer()
		replacer.SetRules(rule)
		extractedData := replacer.HookColor(reqRaw, responseBytes, req, &schema.HTTPFlow{})
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
	err := yakit.SetKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords, defaultRule)
	if err != nil {
		return
	}
	defer yakit.DelKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords)
	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
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

func TestGRPCMUSTPASS_MITMV2_HookColorTimeout(t *testing.T) {
	err := yakit.SetKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords, defaultRule)
	if err != nil {
		return
	}
	defer yakit.DelKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords)
	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
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
	stream, err := client.MITMV2(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	time.Sleep(time.Second)
	_, err = lowhttp.HTTP(lowhttp.WithPacketBytes(testReq), lowhttp.WithProxy(fmt.Sprintf("http://127.0.0.1:%d", mitmPort)), lowhttp.WithHost(mockHost), lowhttp.WithPort(mockPort), lowhttp.WithTimeout(2*time.Second))
	require.NoError(t, err)
}

// TestGRPCMUSTPASS_ReplaceRuleAndMirrorRule fix the bug that the mirror rule not work when the replace rule is enable
func TestGRPCMUSTPASS_ReplaceRuleAndMirrorRule(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
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
	rules, _, _ := replacer.Hook(true, false, "", requestBytes)
	httpctx.AppendMatchedRule(req, rules...)
	flow := &schema.HTTPFlow{}
	replacer.HookColor(requestBytes, []byte(""), req, flow)
	tags := strings.Split(flow.Tags, "|")
	assert.Equal(t, 2, len(tags))
}

// TestGRPCMUSTPASS_ExtraRepeat fix the bug that the ExtraRepeat flow does not have the right tags and color
func TestGRPCMUSTPASS_ExtraRepeat(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
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
	defer func() {
		yakit.RegisterLowHTTPSaveCallback()
	}()
	replacer.Hook(true, false, "", requestBytes)
	replacer.WaitTasks()
	assert.Equal(t, "[重发]tag1|YAKIT_COLOR_red", tags)
	replacer.GetRawRules()[0].ExtraTag = nil
	replacer.Hook(true, false, "", requestBytes)
	tags = ""
	replacer.WaitTasks()
	assert.Equal(t, "[重发]|YAKIT_COLOR_red", tags)
}

func TestGRPCMUSTPASS_HookColorWithNoColorBefore(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
	replacer.SetRules(
		&ypb.MITMContentReplacer{
			Rule:             `example.com`,
			NoReplace:        true,
			Result:           ``,
			Color:            "",
			EnableForRequest: true,
			EnableForHeader:  true,
			EnableForBody:    true,
			Index:            0,
			ExtraTag:         []string{"example"},
			Disabled:         false,
			VerboseName:      "",
		},
		&ypb.MITMContentReplacer{
			Rule:             `file=`,
			NoReplace:        false,
			Result:           `replaced=`,
			Color:            "red",
			EnableForRequest: true,
			EnableForHeader:  true,
			EnableForBody:    true,
			Index:            0,
			ExtraTag:         nil,
			Disabled:         false,
			VerboseName:      "",
		},
	)
	requestBytes := []byte(`GET /file=a.txt HTTP/1.1
Host: example.com
`)
	req, err := lowhttp.ParseBytesToHttpRequest(requestBytes)
	require.NoError(t, err)
	flow := &schema.HTTPFlow{}
	matchRules, modifiedBytes, isDrop := replacer.Hook(true, false, "", requestBytes)
	require.False(t, isDrop)
	require.Len(t, matchRules, 1)
	require.Equal(t, "file=", matchRules[0].Rule)
	require.Contains(t, string(modifiedBytes), "replaced=")
	// 模拟 hook替换完成后，添加tag
	httpctx.SetMatchedRule(req, matchRules)

	extractedData := replacer.HookColor(requestBytes, []byte(""), req, flow)
	require.Len(t, extractedData, 1)
	require.Equal(t, "YAKIT_COLOR_RED|example", flow.Tags)
}

func TestGRPCMUSTPASS_ReplaceWithEffectiveURL(t *testing.T) {
	urlToken := utils.RandStringBytes(10)
	token := utils.RandStringBytes(10)
	replacer := yakit.NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `百度`,
		Result:           token,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
		EffectiveURL:     urlToken,
	})
	requestBytes := []byte(`GET / HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36

{"product": "百度"}`)

	_, res, drop := replacer.Hook(true, false, "http://www.baidu.com/", requestBytes)
	require.NotContains(t, string(res), token)
	require.False(t, drop)

	_, res, drop = replacer.Hook(true, false, "http://www.baidu.com/"+urlToken, requestBytes)
	require.Contains(t, string(res), token)
	require.False(t, drop)
}

func TestGRPCMUSTPASS_ReplaceWithHeaderCookie(t *testing.T) {
	// extra header > extra cookie

	oldCookieValue := "BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C"
	wantCookieValue := fmt.Sprintf("%s=%s", utils.RandStringBytes(10), utils.RandStringBytes(10))
	extraCookieKey, extraCookieValue := utils.RandStringBytes(10), utils.RandStringBytes(10)
	replacer := yakit.NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `www\.baidu\.com`,
		Result:           ``,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
		ExtraHeaders: []*ypb.HTTPHeader{
			{
				Header: `Cookie`,
				Value:  wantCookieValue,
			},
		},
		ExtraCookies: []*ypb.HTTPCookieSetting{
			{
				Key:   extraCookieKey,
				Value: extraCookieValue,
			},
		},
	})
	requestBytes := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: %s

`, oldCookieValue))

	_, res, drop := replacer.Hook(true, false, "http://www.baidu.com/", requestBytes)
	require.False(t, drop)
	require.Contains(t, string(res), wantCookieValue)
	require.NotContains(t, string(res), oldCookieValue)
	require.NotContains(t, string(res), extraCookieKey)
	require.NotContains(t, string(res), extraCookieValue)
}

func TestGRPCMUSTPASS_HookColorWithRegexpGroup(t *testing.T) {
	replacer := yakit.NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:              `(a)(b)(c)`,
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
		RegexpGroups:      []int64{1, 2, 3},
	})
	responseBytes := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 10 Oct 2023 07:28:15 GMT
Content-Length: 23

abc`)
	req, err := http.NewRequest("GET", "https://www.baidu.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	extractedData := replacer.HookColor([]byte(""), responseBytes, req, &schema.HTTPFlow{})
	require.Len(t, extractedData, 1)
	require.Equal(t, "a, b, c", extractedData[0].Data)
}

func TestGRPCMUSTPASS_QueryMITMReplacerRules(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := context.Background()

	// 清理现有规则
	_, err = client.SetCurrentRules(ctx, &ypb.MITMContentReplacers{Rules: []*ypb.MITMContentReplacer{}})
	require.NoError(t, err)

	// 创建测试规则
	testRules := []*ypb.MITMContentReplacer{
		{
			Index:            1,
			VerboseName:      "测试规则1 - API密钥检测",
			Rule:             `(?i)(api[_-]?key|access[_-]?token)`,
			Result:           "***HIDDEN***",
			Color:            "red",
			EnableForRequest: true,
			EnableForHeader:  true,
			EnableForBody:    true,
			Disabled:         false,
		},
		{
			Index:            2,
			VerboseName:      "SQL注入检测",
			Rule:             `(?i)(union\s+select|information_schema)`,
			Result:           "",
			Color:            "orange",
			NoReplace:        true,
			EnableForRequest: true,
			EnableForBody:    true,
			Disabled:         false,
		},
		{
			Index:            3,
			VerboseName:      "用户代理替换",
			Rule:             `User-Agent: .*`,
			Result:           "User-Agent: TestBot/1.0",
			EnableForRequest: true,
			EnableForHeader:  true,
			Disabled:         false,
		},
		{
			Index:            4,
			VerboseName:      "禁用规则",
			Rule:             `disabled-rule`,
			Result:           "replacement",
			EnableForRequest: true,
			Disabled:         true, // 这个规则被禁用
		},
		{
			Index:             5,
			VerboseName:       "XSS检测",
			Rule:              `<script[^>]*>.*?</script>`,
			Result:            "",
			Color:             "yellow",
			NoReplace:         true,
			EnableForResponse: true,
			EnableForBody:     true,
			Disabled:          false,
		},
	}

	// 设置测试规则
	_, err = client.SetCurrentRules(ctx, &ypb.MITMContentReplacers{Rules: testRules})
	require.NoError(t, err)

	testCases := []struct {
		name          string
		keyword       string
		expectedCount int
		expectedNames []string
		description   string
	}{
		{
			name:          "空关键字查询所有规则",
			keyword:       "",
			expectedCount: 5,
			expectedNames: []string{"测试规则1 - API密钥检测", "SQL注入检测", "用户代理替换", "禁用规则", "XSS检测"},
			description:   "应该返回所有规则包括禁用的",
		},
		{
			name:          "按规则名称搜索",
			keyword:       "API",
			expectedCount: 1,
			expectedNames: []string{"测试规则1 - API密钥检测"},
			description:   "应该匹配VerboseName中包含'API'的规则",
		},
		{
			name:          "按规则内容搜索",
			keyword:       "union",
			expectedCount: 1,
			expectedNames: []string{"SQL注入检测"},
			description:   "应该匹配Rule字段中包含'union'的规则",
		},
		{
			name:          "按替换结果搜索",
			keyword:       "TestBot",
			expectedCount: 1,
			expectedNames: []string{"用户代理替换"},
			description:   "应该匹配Result字段中包含'TestBot'的规则",
		},
		{
			name:          "大小写不敏感搜索",
			keyword:       "user-agent",
			expectedCount: 1,
			expectedNames: []string{"用户代理替换"},
			description:   "搜索应该忽略大小写",
		},
		{
			name:          "中文搜索",
			keyword:       "检测",
			expectedCount: 3,
			expectedNames: []string{"测试规则1 - API密钥检测", "SQL注入检测", "XSS检测"},
			description:   "应该支持中文关键字搜索",
		},
		{
			name:          "搜索禁用规则",
			keyword:       "禁用",
			expectedCount: 1,
			expectedNames: []string{"禁用规则"},
			description:   "应该能搜索到禁用的规则",
		},
		{
			name:          "无匹配结果",
			keyword:       "不存在的关键字xyz123",
			expectedCount: 0,
			expectedNames: []string{},
			description:   "不匹配任何规则时应该返回空列表",
		},
		{
			name:          "部分匹配",
			keyword:       "script",
			expectedCount: 1,
			expectedNames: []string{"XSS检测"},
			description:   "应该支持部分匹配",
		},
		{
			name:          "正则表达式字符搜索",
			keyword:       "select",
			expectedCount: 1,
			expectedNames: []string{"SQL注入检测"},
			description:   "应该能搜索包含正则表达式字符的内容",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.QueryMITMReplacerRules(ctx, &ypb.QueryMITMReplacerRulesRequest{
				KeyWord: tc.keyword,
			})
			require.NoError(t, err, "查询规则时出错")
			require.NotNil(t, resp, "响应不应该为空")
			require.NotNil(t, resp.Rules, "响应的Rules字段不应该为空")

			actualCount := len(resp.Rules.Rules)
			assert.Equal(t, tc.expectedCount, actualCount,
				"关键字'%s': %s - 期望%d个结果，实际得到%d个",
				tc.keyword, tc.description, tc.expectedCount, actualCount)

			if tc.expectedCount > 0 {
				actualNames := make([]string, 0, len(resp.Rules.Rules))
				for _, rule := range resp.Rules.Rules {
					actualNames = append(actualNames, rule.VerboseName)
				}

				for _, expectedName := range tc.expectedNames {
					assert.Contains(t, actualNames, expectedName,
						"关键字'%s': 期望找到规则'%s'", tc.keyword, expectedName)
				}

				// 验证返回的规则确实包含关键字
				if tc.keyword != "" {
					keywordLower := strings.ToLower(tc.keyword)
					for _, rule := range resp.Rules.Rules {
						nameMatch := rule.VerboseName != "" &&
							strings.Contains(strings.ToLower(rule.VerboseName), keywordLower)
						ruleMatch := rule.Rule != "" &&
							strings.Contains(strings.ToLower(rule.Rule), keywordLower)
						resultMatch := rule.Result != "" &&
							strings.Contains(strings.ToLower(rule.Result), keywordLower)

						assert.True(t, nameMatch || ruleMatch || resultMatch,
							"规则'%s'应该在名称、规则内容或结果中包含关键字'%s'",
							rule.VerboseName, tc.keyword)
					}
				}
			}
		})
	}

	_, err = client.SetCurrentRules(ctx, &ypb.MITMContentReplacers{Rules: []*ypb.MITMContentReplacer{}})
	require.NoError(t, err)
}

func TestMITMReplaceRule_matchByPacketInfo(t *testing.T) {
	// Test data from user - this is a response
	bodyRaw := `HTTP/1.1 200 OK
Date: Thu, 26 Jun 2025 07:30:37 GMT
Content-Type: text/plain; charset=utf-8
Content-Length: 15

"/path/to/file"`

	// Create MITM Rule - Try with a simpler regex that should work correctly
	rule := &yakit.MITMReplaceRule{
		MITMContentReplacer: &ypb.MITMContentReplacer{
			Rule:              `(?:"|')(((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})|((?:/|\.\./|\./)[^"'><,;|*()(%%$^/\\\[\]][^"'><,;|()]{1,})|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{3,}(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-]{1,}\.(?:\w)(?:[\?|#][^"|']{0,}|)))(?:"|')`, // Simple pattern to match quoted strings and capture the content
			EnableForResponse: true,                                                                                                                                                                                                                                                                                                                                               // Enable for response
			EnableForBody:     true,                                                                                                                                                                                                                                                                                                                                               // Enable for body matching
			EnableForRequest:  false,                                                                                                                                                                                                                                                                                                                                              // Disable for request
			EnableForHeader:   false,                                                                                                                                                                                                                                                                                                                                              // Disable for header
			EnableForURI:      false,                                                                                                                                                                                                                                                                                                                                              // Disable for URI
			RegexpGroups:      []int64{1},                                                                                                                                                                                                                                                                                                                                         // Explicitly specify to extract group 1
		},
	}

	// Add debug info
	t.Logf("Rule: %s", rule.Rule)
	t.Logf("EnableForResponse: %v", rule.EnableForResponse)
	t.Logf("EnableForBody: %v", rule.EnableForBody)
	t.Logf("RegexpGroups: %v", rule.GetRegexpGroups())

	// Test using MatchPacket method which handles the isReq parameter correctly
	packetInfo, results, err := rule.MatchPacket([]byte(bodyRaw), false) // false = this is a response
	require.NoError(t, err, "MatchPacket should not return error")

	// Add more debug info
	t.Logf("PacketInfo.IsRequest: %v", packetInfo.IsRequest)
	t.Logf("PacketInfo.BodyRaw: %q", string(packetInfo.BodyRaw))
	t.Logf("Number of results: %d", len(results))
	for i, result := range results {
		t.Logf("Result %d: %q", i, result.MatchResult)
	}

	require.NotEmpty(t, results, "Should find at least one match")

	// Verify the match result
	assert.Len(t, results, 1, "Should find exactly one match")
	assert.Equal(t, "/path/to/file", results[0].MatchResult, "Should match the file path")
	assert.False(t, results[0].IsMatchRequest, "Should be marked as response match")
	assert.False(t, packetInfo.IsRequest, "PacketInfo should be marked as response")

	t.Logf("Match result: %s", results[0].MatchResult)
}

func TestMITMReplaceRule_UseInlowhttp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Test data from user - this is a response
	bodyRaw := `HTTP/1.1 200 OK
Date: Thu, 26 Jun 2025 07:30:37 GMT
Content-Type: text/plain; charset=utf-8
Content-Length: 15

"/path/to/file"`

	// Create MITM Rule - Try with a simpler regex that should work correctly

	var ruleVerbose = uuid.NewString()
	var rules []*ypb.MITMContentReplacer
	rules = append(rules, &ypb.MITMContentReplacer{
		Rule:              `(?:"|')(((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})|((?:/|\.\./|\./)[^"'><,;|*()(%%$^/\\\[\]][^"'><,;|()]{1,})|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{3,}(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-]{1,}\.(?:\w)(?:[\?|#][^"|']{0,}|)))(?:"|')`, // Simple pattern to match quoted strings and capture the content
		EnableForResponse: true,                                                                                                                                                                                                                                                                                                                                               // Enable for response
		EnableForBody:     true,                                                                                                                                                                                                                                                                                                                                               // Enable for body matching
		EnableForRequest:  false,                                                                                                                                                                                                                                                                                                                                              // Disable for request
		EnableForHeader:   false,                                                                                                                                                                                                                                                                                                                                              // Disable for header
		EnableForURI:      false,                                                                                                                                                                                                                                                                                                                                              // Disable for URI
		RegexpGroups:      []int64{1},                                                                                                                                                                                                                                                                                                                                         // Explicitly specify to extract group 1
		VerboseName:       ruleVerbose,
		Color:             "cyan",
		NoReplace:         true,
	})

	marshal, err := json.Marshal(rules)
	require.NoError(t, err, "Marshal should not return error")
	originReplacer := yakit.GetKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords)
	t.Cleanup(func() {
		yakit.SetKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords, originReplacer)
	})
	err = yakit.SetKey(consts.GetGormProfileDatabase(), yakit.MITMReplacerKeyRecords, string(marshal))
	require.NoError(t, err, "SetKey should not return error")

	runtimeid := uuid.NewString()
	host, port := utils.DebugMockHTTP([]byte(bodyRaw))
	_, _, err = poc.DoGET(fmt.Sprintf("http://%s:%d", host, port), poc.WithMITMRule(true), poc.WithRuntimeId(runtimeid))
	require.NoError(t, err, "DoGET should not return error")

	client, err := NewLocalClient()
	require.NoError(t, err, "NewLocalClient should not return error")
	flows, err := QueryHTTPFlows(ctx, client, &ypb.QueryHTTPFlowRequest{
		RuntimeId: runtimeid,
	}, 1)
	require.NoError(t, err, "QueryHTTPFlow should not return error")
	require.Contains(t, flows.GetData()[0].Tags, schema.FLOW_COLOR_CYAN)
	_, data, err := yakit.QueryExtractedDataPagination(consts.GetGormProjectDatabase(), &ypb.QueryMITMRuleExtractedDataRequest{
		Filter: &ypb.ExtractedDataFilter{
			RuleVerbose: []string{ruleVerbose},
		},
	})
	require.NoError(t, err, "QueryExtractedData should not return error")
	require.GreaterOrEqual(t, len(data), 1, "QueryExtractedData should return extracted data")
}
