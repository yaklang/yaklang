package lowhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
)

func TestReplaceHTTPPacketMethod(t *testing.T) {
	testcases := []struct {
		origin   string
		method   string
		expected string
	}{
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
			method: "PUT",
			expected: `PUT /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
			method: "POST",
			expected: `POST /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded

`,
		},
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
Content-Type: application/json

{"c":"3"}`,
			method: "POST",
			expected: `POST /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
Content-Type: application/json
Content-Length: 9

{"c":"3"}`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceHTTPPacketMethod([]byte(testcase.origin), testcase.method)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), true)
		if bytes.Compare(actual, expected) != 0 {
			spew.Dump(actual)
			spew.Dump(expected)
			t.Fatalf("ReplaceHTTPPacketMethod failed: %s", string(actual))
		}
	}
}

func TestReplaceHTTPPacketFirstLine(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"POST /abc HTTP/1.1",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com`,
			"POST /abc HTTP/1.1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"CCC /abc HTTP/1.1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"XXXXXXXXXXXXX 1.1",
		},
	} {
		byteResult := ReplaceHTTPPacketFirstLine([]byte(c[0]), c[1])
		spew.Dump(byteResult)
		if !bytes.HasPrefix(byteResult, []byte(c[1]+"\r\nHost: www.b")) {
			t.Fatalf("ReplaceHTTPPacketFirstLine failed: %s", string(byteResult))
		}
	}
}

func TestAppendHTTPPacketHeader(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"CCC", "ddd",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			"CCC", "ddd",
			"CCC: aaa",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			"CCC", "ddd",
			"aaa",
		},
	} {
		byteResult := AppendHTTPPacketHeader([]byte(c[0]), c[1], c[2])
		spew.Dump(byteResult)
		if !bytes.Contains(byteResult, []byte(CRLF+c[1]+": "+c[2]+CRLF)) {
			t.Fatalf("ReplaceHTTPPacketHeader failed: %s", string(byteResult))
		}

		if len(c) > 3 {
			if !bytes.Contains(byteResult, []byte(c[3])) {
				t.Fatalf("ReplaceHTTPPacketHeader failed: %s", string(byteResult))
			}
		}
	}
}

func TestReplaceHTTPPacketHeader(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"CCC", "ddd",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			"CCC", "ddd",
			"aaa",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			"CCC", "ddd",
			"aaa",
		},
	} {
		byteResult := ReplaceHTTPPacketHeader([]byte(c[0]), c[1], c[2])
		spew.Dump(byteResult)
		if !bytes.Contains(byteResult, []byte(CRLF+c[1]+": "+c[2]+CRLF)) {
			t.Fatalf("ReplaceHTTPPacketHeader failed: %s", string(byteResult))
		}

		if len(c) > 3 {
			if bytes.Contains(byteResult, []byte(c[3])) {
				t.Fatalf("ReplaceHTTPPacketHeader failed: %s", string(byteResult))
			}
		}
	}
}

func TestDeleteHTTPPacketHeader(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"CCC", "CCC",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			"CCC", "aaa",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			"CCC", "aaa", "Host: www.baidu.com",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			"CCC", "aaa", "DDDD: 11",
		},
		{
			`HTTP/1.1 200 OK
RefererenceA: www.baidu.com
CCC: aaa
DDDD: 11`,
			"CCC", "aaa", "DDDD: 11",
		},
	} {
		black := []byte(c[2])
		var white string
		if len(c) > 3 {
			white = c[3]
		}
		byteResult := DeleteHTTPPacketHeader([]byte(c[0]), c[1])
		spew.Dump(byteResult)
		if bytes.Contains(byteResult, black) {
			t.Fatalf("DeleteHTTPPacketHeader failed: %s", string(byteResult))
		}
		if white != "" {
			if !bytes.Contains(byteResult, []byte(white)) {
				t.Fatalf("DeleteHTTPPacketHeader failed: %s", string(byteResult))
			}
		}
	}
}

func TestReplaceHTTPRequestCookie(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			"a", "3",
			"a=3",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"a", "3",
			"c=132",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"a", "3",
			"c=333",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=1; d=1
`,
			"a", "3",
			"a=3; c=132",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=1; d=1
`,
			"e", "c",
			"e=c",
			"a=3",
		},
	} {
		var (
			black []byte
			white []byte
		)
		_ = black
		_ = white
		if len(c) > 3 {
			white = []byte(c[3])
		}
		if len(c) > 4 {
			black = []byte(c[4])
		}
		byteResult := ReplaceHTTPPacketCookie([]byte(c[0]), c[1], c[2])
		spew.Dump(byteResult)
		if bytes.Contains(byteResult, black) {
			t.Fatalf("ReplaceHTTPPacketCookie failed: %s", string(byteResult))
		}
		if !bytes.Contains(byteResult, []byte(white)) {
			t.Fatalf("ReplaceHTTPPacketCookie failed: %s", string(byteResult))
		}
	}
}

func TestDeleteHTTPRequestCookie(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=1`,
			"a",
			"c=1",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=1`,
			"c",
			"a=1",
			"c=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: d=1`,
			"c",
			"d=1",
			"c=",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
`,
			"cfffffffff",
			"baidu.com",
			"c=",
		},
	} {
		var (
			black []byte
			white []byte
		)
		_ = black
		_ = white
		if len(c) > 2 {
			white = []byte(c[2])
		}
		if len(c) > 3 {
			black = []byte(c[3])
		}
		byteResult := DeleteHTTPPacketCookie([]byte(c[0]), c[1])
		println(string(byteResult))
		if bytes.Contains(byteResult, black) {
			t.Fatalf("DeleteHTTPPacketCookie failed: %s", string(byteResult))
		}
		if !bytes.Contains(byteResult, []byte(white)) {
			t.Fatalf("DeleteHTTPPacketCookie failed: %s", string(byteResult))
		}
	}
}

func TestAppendHTTPRequestCookie(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			"a", "3",
			"a=1",
			"a=2",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			"a", "3",
			"a=3",
			"a=2",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"a", "3",
			"c=132",
			"a=222",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"a", "4",
			"a=4",
			"c=1111",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"a", "3",
			"c=333",
			"a=4",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=132
Cookie: c=333; d=1
`,
			"E", "F",
			"E=F\r\n",
			"a=4",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
`,
			"E", "F",
			"E=F\r\n",
			"a=4",
		},
	} {
		var (
			black []byte
			white []byte
		)
		_ = black
		_ = white
		if len(c) > 3 {
			white = []byte(c[3])
		}
		if len(c) > 4 {
			black = []byte(c[4])
		}
		byteResult := AppendHTTPPacketCookie([]byte(c[0]), c[1], c[2])
		println(string(byteResult))
		if bytes.Contains(byteResult, black) {
			t.Fatalf("AppendHTTPPacketCookie failed: %s", string(byteResult))
		}
		if !bytes.Contains(byteResult, []byte(white)) {
			t.Fatalf("AppendHTTPPacketCookie failed: %s", string(byteResult))
		}
	}
}

func TestGetHTTPPacketCookieValues(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			"a",
			"1", "2",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			"a",
			"1,2", "3",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			"c",
			"", "3",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
`,
			"c",
			"", "3",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
`,
			"c",
			"", "3",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
Set-Cookie: a=1; a=2`,
			"a",
			"1,2", "3",
		},
	} {
		var (
			black []byte
			white []byte
		)
		_ = black
		_ = white
		if len(c) > 2 {
			white = []byte(c[2])
		}
		if len(c) > 3 {
			black = []byte(c[3])
		}
		ret := GetHTTPPacketCookieValues([]byte(c[0]), c[1])
		byteResult := []byte(strings.Join(ret, ","))
		if bytes.Contains(byteResult, black) {
			t.Fatalf("GetHTTPPacketCookieValues failed: %s", string(byteResult))
		}
		if !bytes.Contains(byteResult, []byte(white)) {
			t.Fatalf("GetHTTPPacketCookieValues failed: %s", string(byteResult))
		}
	}
}

func TestGetHTTPPacketCookie(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			"a",
			"1", "2",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			"a",
			"1", "2",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			"c",
			"", "3",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
`,
			"c",
			"", "3",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
`,
			"c",
			"", "3",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
Set-Cookie: a=1; a=2`,
			"a",
			"1", "2",
		},
	} {
		var (
			black []byte
			white []byte
		)
		_ = black
		_ = white
		if len(c) > 2 {
			white = []byte(c[2])
		}
		if len(c) > 3 {
			black = []byte(c[3])
		}
		ret := GetHTTPPacketCookie([]byte(c[0]), c[1])
		byteResult := []byte(ret)
		if bytes.Contains(byteResult, black) {
			t.Fatalf("GetHTTPPacketCookieValues failed: %s", string(byteResult))
		}
		if !bytes.Contains(byteResult, []byte(white)) {
			t.Fatalf("GetHTTPPacketCookieValues failed: %s", string(byteResult))
		}
	}
}

func TestGetHTTPPacketContentType(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Content-Type: abc/abcc
`, "abc/abcc",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
content-type: abc/abcc
`, "abc/abcc",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Content-Type: abc/abcc
content-type: abc/abcd
`, "abc/abcc",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
content-type: abc/abcd
Content-Type: abc/abcc
`, "abc/abcd",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
content-type: abc/abcd
Content-Type: abc/abcc
`, "abc/abcd",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
content-type: abc/abcd
`, "abc/abcd",
		},
		{
			`HTTP/1.1 200 OK
Host: www.baidu.com
Content-Type: abc/abcd
`, "abc/abcd",
		},
	} {
		var packet = utils.InterfaceToBytes(c[0])
		if ret := GetHTTPPacketContentType(packet); ret != c[1] {
			t.Fatalf("GetHTTPPacketContentType failed: %s", string(packet))
		} else {
			println(string(ret))
		}
	}
}

func TestGetHTTPPacketCookies(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com`), [2]string{}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1;
`), [2]string{"a", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`), [2]string{"a", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`), [2]string{"c", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`), [2]string{"c", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`), [2]string{"a", "1"}},
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: c=1
Set-Cookie: b=1; a=1;
`), [2]string{"a", "1"}},
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
`), [2]string{"ddddd", ""}},
	} {
		var results = GetHTTPPacketCookies(c[0].([]byte))
		ret := c[1].([2]string)
		key, value := ret[0], ret[1]
		if key == "" {
			continue
		}

		spew.Dump(results)
		if ret, _ := results[key]; ret != value {
			println(string(c[0].([]byte)))
			panic(fmt.Sprintf("GetHTTPPacketCookies failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPPacketCookiesFull(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com`), [2]string{}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2
`), [2]string{"a", "1,2"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com`), [2]string{}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1;
`), [2]string{"a", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`), [2]string{"a", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`), [2]string{"c", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`), [2]string{"c", "1"}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`), [2]string{"a", "1"}},
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: c=1
Set-Cookie: b=1; a=1;
`), [2]string{"a", "1"}},
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
`), [2]string{"ddddd", ""}},
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
Set-Cookie: a=123
`), [2]string{"a", "1,123"}},
	} {
		var results = GetHTTPPacketCookiesFull(c[0].([]byte))
		ret := c[1].([2]string)
		key, value := ret[0], ret[1]
		if key == "" {
			continue
		}

		spew.Dump(results)
		if ret, _ := results[key]; strings.Join(ret, ",") != value {
			println(string(c[0].([]byte)))
			panic(fmt.Sprintf("GetHTTPPacketCookies failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPPacketHeadersFull(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.com
Host: www.baidu.cn`), [2]string{
			"Host", "www.baidu.com,www.baidu.cn",
		}},
		{[]byte(`GET / HTTP/1.1
Host: www.baidu.cn`), [2]string{
			"Host", "www.baidu.cn",
		}},
		{[]byte(`GET / HTTP/1.1
Content-Type: www.baidu.cn`), [2]string{
			"Content-Type", "www.baidu.cn",
		}},
	} {
		var results = GetHTTPPacketHeadersFull(c[0].([]byte))
		ret := c[1].([2]string)
		key, value := ret[0], ret[1]
		if key == "" {
			continue
		}

		spew.Dump(results)
		if ret, _ := results[key]; strings.Join(ret, ",") != value {
			println(string(c[0].([]byte)))
			panic(fmt.Sprintf("GetHTTPPacketCookies failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetStatusCodeFromResponse(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
`), 200},
		{[]byte(`HTTP/1.1 300 OK
Content-Type: text/html; charset=utf-8
`), 300},
		{[]byte(`HTTP/1.1 3 OK
Content-Type: text/html; charset=utf-8
`), 3},
		{[]byte(`GET / HTTP/1.1
Content-Type: text/html; charset=utf-8
`), 0},
	} {
		if GetStatusCodeFromResponse(c[0].([]byte)) != c[1].(int) {
			panic(fmt.Sprintf("GetStatusCodeFromResponse failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPRequestQueryParam(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`), [2]string{"a", "1"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`), [2]string{"b", "2"}},
		{[]byte(`GET /?a=1&b=2&a=3 HTTP/1.1
Host: www.baidu.com
`), [2]string{"a", "1"}},
	} {
		if GetHTTPRequestQueryParam(c[0].([]byte), c[1].([2]string)[0]) != c[1].([2]string)[1] {
			panic(fmt.Sprintf("GetHTTPRequestQueryParam failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPRequestQueryParamFull(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`), [2]string{"a", "1"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`), [2]string{"b", "2"}},
		{[]byte(`GET /?a=1&b=2&a=3 HTTP/1.1
Host: www.baidu.com
`), [2]string{"a", "1,3"}},
	} {
		if strings.Join(GetHTTPRequestQueryParamFull(c[0].([]byte), c[1].([2]string)[0]), ",") != c[1].([2]string)[1] {
			panic(fmt.Sprintf("GetHTTPRequestQueryParamFull failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPRequestPostParamFull(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1
`), [2]string{"a", ""}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1
`), [2]string{"d", "1"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1`), [2]string{"e", "555"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1&e=111`), [2]string{"e", "555,111"}},
	} {
		vals := strings.Join(GetHTTPRequestPostParamFull(c[0].([]byte), c[1].([2]string)[0]), ",")
		if vals != c[1].([2]string)[1] {
			spew.Dump(vals)
			spew.Dump(c)
			panic(fmt.Sprintf("GetHTTPRequestQueryParamFull failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetHTTPRequestPostParam(t *testing.T) {
	for _, c := range [][]any{
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1
`), [2]string{"a", ""}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1
`), [2]string{"d", "1"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1`), [2]string{"e", "555"}},
		{[]byte(`GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

d=1&e=555&f=1&e=111`), [2]string{"e", "555"}},
	} {
		vals := strings.Join([]string{GetHTTPRequestPostParam(c[0].([]byte), c[1].([2]string)[0])}, ",")
		if vals != c[1].([2]string)[1] {
			spew.Dump(vals)
			spew.Dump(c)
			panic(fmt.Sprintf("GetHTTPRequestQueryParamFull failed: %s", string(c[0].([]byte))))
		}
	}
}

func TestGetAllHTTPRequestQueryParam(t *testing.T) {
	testcases := []struct {
		origin   string
		expected map[string]string
	}{
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,

			expected: map[string]string{
				"a": "1",
				"b": "2",
			},
		},
		{
			origin: `GET /?a=1&b=2&a=3 HTTP/1.1
Host: www.baidu.com
`,

			expected: map[string]string{
				"a": "3",
				"b": "2",
			},
		},
	}
	for _, testcase := range testcases {
		actual := GetAllHTTPRequestQueryParams([]byte(testcase.origin))
		if !reflect.DeepEqual(actual, testcase.expected) {
			t.Fatalf("GetAllHTTPRequestQueryParam failed: %v", actual)
		}
	}
}

func TestGetAllHTTPRequestPostParam(t *testing.T) {
	testcases := []struct {
		origin   string
		expected map[string]string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,

			expected: map[string]string{
				"a": "1",
				"b": "2",
			},
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2&a=3`,

			expected: map[string]string{
				"a": "3",
				"b": "2",
			},
		},
	}
	for _, testcase := range testcases {
		actual := GetAllHTTPRequestPostParams([]byte(testcase.origin))
		if !reflect.DeepEqual(actual, testcase.expected) {
			t.Fatalf("GetAllHTTPRequestPostParam failed: %v", actual)
		}
	}
}

func TestGetHTTPPacketFirstLine(t *testing.T) {
	testcases := []struct {
		origin   string
		expected [3]string
	}{
		{
			origin: `POST /path HTTP/1.1
Host: www.baidu.com

a=1&b=2`,

			expected: [3]string{"POST", "/path", "HTTP/1.1"},
		},
		{
			origin: `HTTP/1.1 200 OK
Content-Length: 4

test`,
			expected: [3]string{"HTTP/1.1", "200", "OK"},
		},
	}
	for _, testcase := range testcases {
		first, second, third := GetHTTPPacketFirstLine([]byte(testcase.origin))
		if first != testcase.expected[0] {
			t.Fatalf("GetHTTPPacketFirstLine first failed: %v(got) != %v(want)", first, testcase.expected[0])
		}
		if second != testcase.expected[1] {
			t.Fatalf("GetHTTPPacketFirstLine second failed: %v(got) != %v(want)", second, testcase.expected[1])
		}
		if third != testcase.expected[2] {
			t.Fatalf("GetHTTPPacketFirstLine third failed: %v(got) != %v(want)", third, testcase.expected[2])
		}

	}
}

func TestReplaceHTTPPacketQueryParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		value    string
		expected string
	}{
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
			key:   "a",
			value: "3",
			expected: `GET /?a=3&b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceHTTPPacketQueryParam([]byte(testcase.origin), testcase.key, testcase.value)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceHTTPPacketQueryParam failed: %s", string(actual))
		}
	}
}

func TestAppendHTTPPacketQueryParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		value    string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			key:   "a",
			value: "1",
			expected: `GET /?a=1 HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
			key:   "c",
			value: "3",
			expected: `GET /?a=1&b=2&c=3 HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {
		actual := AppendHTTPPacketQueryParam([]byte(testcase.origin), testcase.key, testcase.value)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("AddHTTPPacketQueryParam failed: %s", string(actual))
		}
	}
}

func TestDeleteHTTPPacketQueryParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			key: "a",
			expected: `GET / HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /?a=1 HTTP/1.1
Host: www.baidu.com
`,
			key: "a",
			expected: `GET / HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
			key: "a",
			expected: `GET /?b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {
		actual := DeleteHTTPPacketQueryParam([]byte(testcase.origin), testcase.key)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("DeleteHTTPPacketQueryParam failed: %s", string(actual))
		}
	}
}

func TestReplaceHTTPPacketPostParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		value    string
		expected string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,
			key:   "a",
			value: "3",
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a=3&b=2`,
		},
	}
	for _, testcase := range testcases {

		actual := ReplaceHTTPPacketPostParam([]byte(testcase.origin), testcase.key, testcase.value)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceHTTPPacketPostParam failed: %s", string(actual))
		}
	}
}

func TestAppendHTTPPacketPostParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		value    string
		expected string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

`,
			key:   "a",
			value: "1",
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a=1`,
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1`,
			key:   "b",
			value: "2",
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,
		},
	}
	for _, testcase := range testcases {

		actual := AppendHTTPPacketPostParam([]byte(testcase.origin), testcase.key, testcase.value)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("AddHTTPPacketPostParam failed: %s", string(actual))
		}
	}
}

func TestDeleteHTTPPacketPostParam(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		expected string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Length: 3

a=1`,
			key: "a",
			expected: `POST / HTTP/1.1
Host: www.baidu.com

`,
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Length: 7

a=1&b=2`,
			key: "a",
			expected: `POST / HTTP/1.1
Host: www.baidu.com
Content-Length: 3

b=2`,
		},
	}
	for _, testcase := range testcases {

		actual := DeleteHTTPPacketPostParam([]byte(testcase.origin), testcase.key)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), true)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceHTTPPacketPostParam failed: \ngot:\n%s\n\nwant:\n%s", string(actual), string(expected))
		}
	}
}

func TestReplaceHTTPPacketPath(t *testing.T) {
	testcases := []struct {
		origin   string
		path     string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			path: "path",
			expected: `GET /path HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			path: "/path",
			expected: `GET /path HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET invalid HTTP/1.1
Host: www.baidu.com
`,
			path: "/path",
			expected: `GET /path HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {

		actual := ReplaceHTTPPacketPath([]byte(testcase.origin), testcase.path)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceHTTPPacketPath failed: %s", string(actual))
		}
	}
}

func TestAppendHTTPPacketPath(t *testing.T) {
	testcases := []struct {
		origin   string
		path     string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			path: "/path",
			expected: `GET /path HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /prefix HTTP/1.1
Host: www.baidu.com
`,
			path: "/path",
			expected: `GET /prefix/path HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /prefix HTTP/1.1
Host: www.baidu.com
`,
			path: "path",
			expected: `GET /prefix/path HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {

		actual := AppendHTTPPacketPath([]byte(testcase.origin), testcase.path)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("AddHTTPPacketPath failed: %s", string(actual))
		}
	}
}

func TestAppendHTTPPacketFormEncoded(t *testing.T) {
	compare := func(mutlipartReader *multipart.Reader, key, value string) {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)

			}
			return
		}
		if part.FormName() != key {
			t.Fatalf("AppendHTTPPacketFormEncoded failed: form-key failed: %s(got) != %s(want)", part.FormName(), key)
		}

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, part); err != nil {
			t.Fatal(err)
		}
		if buf.String() != value {
			t.Fatalf("AppendHTTPPacketFormEncoded failed: form-value failed: %s(got) != %s(want)", buf.String(), value)
		}
	}

	testcases := []struct {
		origin           string
		oldKey, oldValue string
		key, value       string
		// expected   string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			key:   "a",
			value: "1",
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 7

a=1&b=2`,
			key:   "a",
			value: "1",
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="a"

1
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			oldKey:   "a",
			oldValue: "1",
			key:      "b",
			value:    "2",
		},
	}
	for _, testcase := range testcases {
		actual := AppendHTTPPacketFormEncoded([]byte(testcase.origin), testcase.key, testcase.value)

		blocks := strings.SplitN(string(actual), "\r\n\r\n", 2)
		body := blocks[1]
		_ = body
		re := regexp.MustCompile(`(?m)(--\w+)`)
		result := re.ReplaceAllString(body, "--test")

		// multipart reader
		mutlipartReader := multipart.NewReader(strings.NewReader(result), "test")

		// compare old key and value
		if testcase.oldKey != "" {
			compare(mutlipartReader, testcase.oldKey, testcase.oldValue)
		}

		// compare new key and value
		compare(mutlipartReader, testcase.key, testcase.value)

	}
}

func TestAppendHTTPPacketUploadFile(t *testing.T) {
	compare := func(mutlipartReader *multipart.Reader, fieldName, fileName string, fileContent interface{}) {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)

			}
			return
		}
		if part.FormName() != fieldName {
			t.Fatalf("AppendHTTPPacketFormEncoded failed: form-key failed: %s(got) != %s(want)", part.FormName(), fieldName)
		}
		if part.FileName() != fileName {
			t.Fatalf("AppendHTTPPacketFormEncoded failed: form-key failed: %s(got) != %s(want)", part.FileName(), fileName)
		}

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, part); err != nil {
			t.Fatal(err)
		}

		switch r := fileContent.(type) {
		case string:
			if buf.String() != r {
				t.Fatalf("AppendHTTPPacketFormEncoded failed: form-value failed: %s(got) != %s(want)", buf.String(), r)
			}
		case []byte:
			if bytes.Compare(buf.Bytes(), r) != 0 {
				t.Fatalf("AppendHTTPPacketFormEncoded failed: form-value failed: %s(got) != %s(want)", buf.String(), r)
			}
		case io.Reader:
			buf2 := new(bytes.Buffer)
			if _, err = io.Copy(buf2, r); err != nil {
				t.Fatal(err)
			}
			if buf.String() != buf2.String() {
				t.Fatalf("AppendHTTPPacketFormEncoded failed: form-value failed: %s(got) != %s(want)", buf.String(), buf2.String())
			}
		}
	}

	testcases := []struct {
		origin                    string
		oldfieldName, oldfileName string
		oldFileContent            string
		fieldName, fileName       string
		fileContent               interface{}
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			fieldName:   "test",
			fileName:    "test.txt",
			fileContent: "test",
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="a"

1
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			oldfieldName:   "a",
			oldfileName:    "",
			oldFileContent: "1",
			fieldName:      "test",
			fileName:       "test.txt",
			fileContent:    "test",
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="aaa"; filename="aaa.txt"
Content-Type: application/octet-stream

bbb
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			oldfieldName:   "aaa",
			oldfileName:    "aaa.txt",
			oldFileContent: "bbb",
			fieldName:      "test",
			fileName:       "test.txt",
			fileContent:    "test",
		},
	}
	for _, testcase := range testcases {
		actual := AppendHTTPPacketUploadFile([]byte(testcase.origin), testcase.fieldName, testcase.fileName, testcase.fileContent)

		blocks := strings.SplitN(string(actual), "\r\n\r\n", 2)
		body := blocks[1]
		_ = body
		re := regexp.MustCompile(`(?m)(--\w+)`)
		result := re.ReplaceAllString(body, "--test")

		// multipart reader
		mutlipartReader := multipart.NewReader(strings.NewReader(result), "test")

		// compare old
		if testcase.oldfieldName != "" {
			compare(mutlipartReader, testcase.oldfieldName, testcase.oldfileName, testcase.oldFileContent)
		}

		// compare new
		compare(mutlipartReader, testcase.fieldName, testcase.fileName, testcase.fileContent)

	}
}

func TestDeleteHTTPPacketFormEncoded(t *testing.T) {
	testcases := []struct {
		origin   string
		key      string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			key: "a",
			expected: `GET / HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 7

a=1&b=2`,
			key: "a",
			expected: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 7

a=1&b=2`,
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="a"

1
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="b"

2
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			key: "a",
			expected: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Length: 131

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="b"

2
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
		},
	}
	for _, testcase := range testcases {
		actual := DeleteHTTPPacketForm([]byte(testcase.origin), testcase.key)

		expected := FixHTTPPacketCRLF([]byte(testcase.expected), true)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("DeleteHTTPPacketFormEncoded failed: \n%s", string(actual))
		}
	}
}
