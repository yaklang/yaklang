package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

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
