package lowhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/multipart"
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

func TestAppendHTTPPacketHeaderIfNotExist(t *testing.T) {
	for _, c := range [][4]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"CCC", "ddd",
			"CCC: ddd",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			"CCC", "ddd",
			"CCC: aaa",
		},
	} {
		byteResult := AppendHTTPPacketHeaderIfNotExist([]byte(c[0]), c[1], c[2])
		spew.Dump(byteResult)
		if !bytes.Contains(byteResult, []byte(c[3])) {
			t.Fatalf("ReplaceHTTPPacketHeader failed: %s", string(byteResult))
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

func TestReplaceHTTPRequestCookies(t *testing.T) {
	for _, testcase := range []struct {
		packet  string
		m       map[string]string
		excepts []string
	}{
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1
`,
			m: map[string]string{
				"a": "3",
				"b": "4",
			},
			excepts: []string{"a=3", "b=4"},
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
`,
			m: map[string]string{
				"a": "3",
				"b": "4",
			},
			excepts: []string{"a=3", "b=4"},
		},
	} {
		result := string(ReplaceHTTPPacketCookies([]byte(testcase.packet), testcase.m))
		for _, except := range testcase.excepts {
			require.Contains(t, result, except)
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
	testCases := []struct {
		name      string
		packet    string
		cookie    string
		expected  []byte
		blacklist []byte
	}{
		{
			name: "SingleCookie",
			packet: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1`,
			cookie:   "a",
			expected: []byte("1"),
		},
		{
			name: "MultipleCookiesSameName",
			packet: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			cookie:   "a",
			expected: []byte("1,2"),
		},
		{
			name: "NonExistentCookie",
			packet: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2`,
			cookie:   "c",
			expected: []byte(""),
		},
		{
			name: "EmptyCookies",
			packet: `GET / HTTP/1.1
Host: www.baidu.com
`,
			cookie:   "c",
			expected: []byte(""),
		},
		{
			name: "NoCookiesHTTP200",
			packet: `HTTP/1.1 200 OK
Host: www.baidu.com
`,
			cookie:   "c",
			expected: []byte(""),
		},
		{
			name: "SetCookieInsteadOfCookie",
			packet: `HTTP/1.1 200 OK
Host: www.baidu.com
Set-Cookie: a=1; HttpOnly`,
			cookie:   "a",
			expected: []byte("1"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetHTTPPacketCookieValues([]byte(tc.packet), tc.cookie)
			byteResult := []byte(strings.Join(result, ","))

			// Validate expected result should be present
			if !bytes.Contains(byteResult, tc.expected) {
				t.Errorf("Expected %q to include %q", byteResult, tc.expected)
			}

			// Validate blacklist result should not be present
			if len(tc.blacklist) > 0 && bytes.Contains(byteResult, tc.blacklist) {
				t.Errorf("Result %q should not contain %q", byteResult, tc.blacklist)
			}
		})
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
		packet := utils.InterfaceToBytes(c[0])
		if ret := GetHTTPPacketContentType(packet); ret != c[1] {
			t.Fatalf("GetHTTPPacketContentType failed: %s", string(packet))
		} else {
			println(string(ret))
		}
	}
}

//func TestGetHTTPPacketCookies(t *testing.T) {
//	for _, c := range [][]any{
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com`), [2]string{}},
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com
//Cookie: a=1;
//`), [2]string{"a", "1"}},
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com
//Cookie: c=1
//Cookie: a=1;
//`), [2]string{"a", "1"}},
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com
//Cookie: c=1
//Cookie: a=1;
//`), [2]string{"c", "1"}},
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com
//Cookie: c=1
//Cookie: b=1; a=1;
//`), [2]string{"c", "1"}},
//		{[]byte(`GET / HTTP/1.1
//Host: www.baidu.com
//Cookie: c=1
//Cookie: b=1; a=1;
//`), [2]string{"a", "1"}},
//		{[]byte(`HTTP/1.1 200 OK
//Content-Type: text/html; charset=utf-8
//Set-Cookie: c=1
//Set-Cookie: b=1; a=1;
//`), [2]string{"a", "1"}},
//		{[]byte(`HTTP/1.1 200 OK
//Content-Type: text/html; charset=utf-8
//Set-Cookie: b=1; a=1;
//`), [2]string{"ddddd", ""}},
//	} {
//		results := GetHTTPPacketCookies(c[0].([]byte))
//		ret := c[1].([2]string)
//		key, value := ret[0], ret[1]
//		if key == "" {
//			continue
//		}
//
//		spew.Dump(results)
//		if ret, _ := results[key]; ret != value {
//			println(string(c[0].([]byte)))
//			panic(fmt.Sprintf("GetHTTPPacketCookies failed: %s", string(c[0].([]byte))))
//		}
//	}
//}

func TestGetHTTPPacketCookies(t *testing.T) {
	testCases := []struct {
		name     string
		packet   []byte
		key      string
		expected string
	}{
		{
			name: "NoCookies",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com`),
			key:      "",
			expected: "",
		},
		{
			name: "SingleCookie",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1;
`),
			key: "a", expected: "1",
		},
		{
			name: "MultipleCookiesSamePacket",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`),
			key:      "a",
			expected: "1",
		},
		{
			name: "FirstCookie",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`),
			key:      "c",
			expected: "1",
		},
		{
			name: "SelectFirstCookie",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`),
			key:      "c",
			expected: "1",
		},
		{
			name: "SelectLastCookie",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`),
			key:      "a",
			expected: "1",
		},
		{
			name: "HTTP200Cookies",
			packet: []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: c=1
Set-Cookie: b=1
`),
			key:      "b",
			expected: "1",
		},
		{
			name: "NonexistentCookie",
			packet: []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
`),
			key:      "ddddd",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := GetHTTPPacketCookies(tc.packet)
			if tc.key == "" {
				return // If key is empty, skip the check
			}
			value, exists := results[tc.key]
			if !exists && tc.expected != "" {
				t.Fatalf("Expected key %q was not found in the results", tc.key)
			}
			if exists && value != tc.expected {
				t.Errorf("Mismatch for key %q: expected %q, got %q", tc.key, tc.expected, value)
			}
		})
	}
}

func TestGetHTTPPacketCookiesFull(t *testing.T) {
	testCases := []struct {
		name     string
		packet   []byte
		key      string
		expected string
	}{
		{
			name: "NoCookiesEmptyPacket",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com`),
			key:      "",
			expected: "",
		},
		{
			name: "CookiesSetOnce",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; a=2
`), key: "a",
			expected: "1,2",
		},
		{
			name: "SingleCookieSingleValue",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1;
`),
			key:      "a",
			expected: "1",
		},
		{
			name: "MultipleCookiesSamePacket",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`),
			key:      "a",
			expected: "1",
		},
		{
			name: "MultipleCookiesDifferentPacket",
			packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: a=1;
`),
			key:      "c",
			expected: "1",
		},
		{
			name: "MixedCookies", packet: []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=1
Cookie: b=1; a=1;
`),
			key: "c", expected: "1",
		},
		{
			name: "MultipleCookiesMixedPacket",
			packet: []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: c=1
Set-Cookie: b=1
Set-Cookie: a=1;
`),
			key:      "a",
			expected: "1",
		},
		{
			name: "CookiesNotFound",
			packet: []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
`),
			key:      "ddddd",
			expected: "",
		},
		{
			name: "CookiesOverwritten",
			packet: []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: b=1; a=1;
Set-Cookie: a=123
`),
			key:      "a",
			expected: "123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := GetHTTPPacketCookiesFull(tc.packet)
			if tc.key == "" { // If key is empty, skip check
				return
			}
			value, exists := results[tc.key]
			if !exists && tc.expected != "" {
				t.Fatalf("Cookie key %q not found in results", tc.key)
			}
			joinedValue := strings.Join(value, ",")
			if joinedValue != tc.expected {
				t.Errorf("Mismatch for cookie %q: expected %q, got %q", tc.key, tc.expected, joinedValue)
			}
		})
	}
}

func TestGetHTTPPacketHeader2(t *testing.T) {
	if GetHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.baidu.com`), "host") != "www.baidu.com" {
		t.Fatal("GetHTTPPacketHeader failed(insensitive case test)")
	}
}

func TestGetHTTPPacketURLFetcher(t *testing.T) {
	for _, c := range [][]string{
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"http",
			"http://www.baidu.com/",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"https",
			"https://www.baidu.com/",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com`,
			"mailto",
			"mailto://www.baidu.com/",
		},
		{
			`GET /?a=1 HTTP/1.1
Host: www.baidu.com`,
			"mailto",
			"mailto://www.baidu.com/?a=1",
		},
		{
			`GET ?a=1 HTTP/1.1
Host: www.baidu.com`,
			"mailto",
			"mailto://www.baidu.com/?a=1",
		},
		{
			`GET /aaaa?a=1 HTTP/1.1
Host: www.baidu.com`,
			"mailto",
			"mailto://www.baidu.com/aaaa?a=1",
		},
		{
			`GET aaaa?a=1 HTTP/1.1
Host: www.baidu.com`,
			"mailto",
			"mailto://www.baidu.com/aaaa?a=1",
		},
		{
			`GET aaaa?a=1 HTTP/1.1
Host: www.baidu.com:80`,
			"http",
			"http://www.baidu.com:80/aaaa?a=1",
		},
		{
			`GET aaaa?a=1 HTTP/1.1
Host: www.baidu.com`,
			"https",
			"https://www.baidu.com/aaaa?a=1",
		},
	} {
		if ret := GetUrlFromHTTPRequest(c[1], []byte(c[0])); ret != c[2] {
			spew.Dump(ret)
			t.Fatalf("GetHTTPPacketURLFetcher failed: %s", string(c[0]))
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
		results := GetHTTPPacketHeadersFull(c[0].([]byte))
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

func TestReplaceAllHttpPacketQueryParams(t *testing.T) {
	for i := 0; i < 100; i++ {
		_testReplaceAllHttpPacketQueryParams(t)
	}
}

func _testReplaceAllHttpPacketQueryParams(t *testing.T) {
	testcases := []struct {
		origin   string
		values   map[string]string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			values: map[string]string{"a": "1", "b": "2"},
			expected: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
		{
			origin: `GET /?c=3 HTTP/1.1
Host: www.baidu.com
`,
			values: map[string]string{"a": "1", "b": "2"},
			expected: `GET /?a=1&b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceAllHTTPPacketQueryParams([]byte(testcase.origin), testcase.values)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			spew.Dump(actual, expected)
			t.Fatalf("ReplaceAllHTTPPacketQueryParams failed: %s", string(actual))
		}
	}
}

func TestReplaceAllHttpPacketQueryParamsWithoutEscape(t *testing.T) {
	testcases := []struct {
		origin   string
		values   map[string]string
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			values: map[string]string{"a": "{{int(1-100)}}", "b": "2"},
			expected: `GET /?a={{int(1-100)}}&b=2 HTTP/1.1
Host: www.baidu.com
`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceAllHTTPPacketQueryParamsWithoutEscape([]byte(testcase.origin), testcase.values)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			spew.Dump(actual, expected)
			t.Fatalf("ReplaceAllHTTPPacketQueryParamsWithoutEscape failed: %s", string(actual))
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
		spew.Dump(actual, expected)
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

func TestReplaceAllHttpPacketPostParams(t *testing.T) {
	testcases := []struct {
		origin   string
		values   map[string]string
		expected string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

`,
			values: map[string]string{"a": "1", "b": "2"},
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

c=3`,
			values: map[string]string{"a": "1", "b": "2"},
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceAllHTTPPacketPostParams([]byte(testcase.origin), testcase.values)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceAllHTTPPacketQueryParams failed: %s", string(actual))
		}
	}
}

func TestReplaceAllHttpPacketPostParamsWithoutEscape(t *testing.T) {
	testcases := []struct {
		origin   string
		values   map[string]string
		expected string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

c=1&d=2`,
			values: map[string]string{"a": "{{int(1-100)}}", "b": "2"},
			expected: `POST / HTTP/1.1
Host: www.baidu.com

a={{int(1-100)}}&b=2`,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceAllHTTPPacketPostParamsWithoutEscape([]byte(testcase.origin), testcase.values)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceAllHTTPPacketQueryParams failed: %s", string(actual))
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

func TestReplaceHTTPPacketFormEncoded(t *testing.T) {
	compare := func(mutlipartReader *multipart.Reader, key, value string) {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			return
		}
		require.Equal(t, key, part.FormName(), "form-key")
		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, part); err != nil {
			t.Fatal(err)
		}
		require.Equal(t, value, buf.String(), "form-value")
	}

	testcases := []struct {
		origin           string
		oldKey, oldValue string
		key, value       string
		exceptFormCount  int // set if oldKey == key
	}{
		{
			// append
			origin: `GET / HTTP/1.1
		Host: www.baidu.com
		`,
			key:   "a",
			value: "1",
		},
		{
			// append with no-form data
			origin: `POST / HTTP/1.1
		Host: www.baidu.com
		Content-Type: application/x-www-form-urlencoded
		Content-Length: 7

		a=1&b=2`,
			key:   "a",
			value: "1",
		},
		{
			// replace
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="a"

1
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			oldKey:          "a",
			oldValue:        "1",
			key:             "a",
			value:           "2",
			exceptFormCount: 1,
		},
	}
	for _, testcase := range testcases {
		actual := ReplaceHTTPPacketFormEncoded([]byte(testcase.origin), testcase.key, testcase.value)
		blocks := strings.SplitN(string(actual), "\r\n\r\n", 2)
		body := blocks[1]
		mutlipartReader := multipart.NewReader(bytes.NewBufferString(body))

		if testcase.oldKey != testcase.key {
			// compare old key and value
			if testcase.oldKey != "" {
				compare(mutlipartReader, testcase.oldKey, testcase.oldValue)
			}

			// compare new key and value
			compare(mutlipartReader, testcase.key, testcase.value)
		} else {
			// compare new key and value
			compare(mutlipartReader, testcase.key, testcase.value)
			count := 1
			for {
				_, err := mutlipartReader.NextPart()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatal(err)
				}
				count++
			}
			require.Equal(t, testcase.exceptFormCount, count)
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
		mutlipartReader := multipart.NewReader(bytes.NewBufferString(body))

		// compare old key and value
		if testcase.oldKey != "" {
			compare(mutlipartReader, testcase.oldKey, testcase.oldValue)
		}

		// compare new key and value
		compare(mutlipartReader, testcase.key, testcase.value)

	}
}

func TestReplaceHTTPPacketUploadFile(t *testing.T) {
	compare := func(mutlipartReader *multipart.Reader, fieldName, fileName string, fileContent interface{}, contentType string) {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			return
		}
		if part.FormName() != fieldName {
			t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-key failed: %s(got) != %s(want)", part.FormName(), fieldName)
		}
		if part.FileName() != fileName {
			t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-key failed: %s(got) != %s(want)", part.FileName(), fileName)
		}

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, part); err != nil {
			t.Fatal(err)
		}

		switch r := fileContent.(type) {
		case string:
			if buf.String() != r {
				t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", buf.String(), r)
			}
		case []byte:
			if bytes.Compare(buf.Bytes(), r) != 0 {
				t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", buf.String(), r)
			}
		case io.Reader:
			buf2 := new(bytes.Buffer)
			if _, err = io.Copy(buf2, r); err != nil {
				t.Fatal(err)
			}
			if buf.String() != buf2.String() {
				t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", buf.String(), buf2.String())
			}
		}

		if contentType != "" {
			if part.Header.Get("Content-Type") != contentType {
				t.Fatalf("ReplaceHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", part.Header.Get("Content-Type"), contentType)
			}
		}
	}

	testcases := []struct {
		origin                    string
		oldfieldName, oldfileName string
		oldFileContent            string
		fieldName, fileName       string
		fileContent               interface{}
		contentType               string
		exceptFormCount           int // set if oldfieldName == fieldName
	}{
		{
			// append
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			fieldName:   "test",
			fileName:    "test.txt",
			fileContent: "test",
		},
		{
			// append with already existed form-data
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
			// replace
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="aaa"; filename="aaa.txt"
Content-Type: application/octet-stream

bbb
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			oldfieldName:    "aaa",
			oldfileName:     "aaa.txt",
			oldFileContent:  "aaa",
			fieldName:       "aaa",
			fileName:        "test.txt",
			fileContent:     "test",
			exceptFormCount: 1,
		},
	}
	for _, testcase := range testcases {
		var actual []byte
		if testcase.contentType != "" {
			actual = ReplaceHTTPPacketUploadFile([]byte(testcase.origin), testcase.fieldName, testcase.fileName, testcase.fileContent, testcase.contentType)
		} else {
			actual = ReplaceHTTPPacketUploadFile([]byte(testcase.origin), testcase.fieldName, testcase.fileName, testcase.fileContent)
		}
		blocks := strings.SplitN(string(actual), "\r\n\r\n", 2)
		body := blocks[1]
		mutlipartReader := multipart.NewReader(bytes.NewBufferString(body))

		if testcase.oldfieldName != testcase.fieldName {
			// compare old
			if testcase.oldfieldName != "" {
				compare(mutlipartReader, testcase.oldfieldName, testcase.oldfileName, testcase.oldFileContent, testcase.contentType)
			}

			// compare new
			compare(mutlipartReader, testcase.fieldName, testcase.fileName, testcase.fileContent, testcase.contentType)
		} else {
			// compare new
			compare(mutlipartReader, testcase.fieldName, testcase.fileName, testcase.fileContent, testcase.contentType)
			// check count
			count := 1
			for {
				_, err := mutlipartReader.NextPart()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatal(err)
				}
				count++
			}
			require.Equal(t, testcase.exceptFormCount, count)
		}

	}
}

func TestAppendHTTPPacketUploadFile(t *testing.T) {
	compare := func(mutlipartReader *multipart.Reader, fieldName, fileName string, fileContent string, contentType string) {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			return
		}
		if part.FormName() != fieldName {
			t.Fatalf("AppendHTTPPacketUploadFile failed: form-key failed: %s(got) != %s(want)", part.FormName(), fieldName)
		}
		if part.FileName() != fileName {
			t.Fatalf("AppendHTTPPacketUploadFile failed: form-key failed: %s(got) != %s(want)", part.FileName(), fileName)
		}

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, part); err != nil {
			t.Fatal(err)
		}

		if buf.String() != fileContent {
			t.Fatalf("AppendHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", buf.String(), fileContent)
		}

		if contentType != "" {
			if part.Header.Get("Content-Type") != contentType {
				t.Fatalf("AppendHTTPPacketUploadFile failed: form-value failed: %s(got) != %s(want)", part.Header.Get("Content-Type"), contentType)
			}
		}
	}

	testcases := []struct {
		origin                    string
		oldfieldName, oldfileName string
		oldFileContent            string
		oldContentType            string
		fieldName, fileName       string
		fileContent               string
		contentType               string
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
			oldContentType: "application/octet-stream",
			fieldName:      "test",
			fileName:       "test.php",
			fileContent:    "<?php phpinfo();?>",
			contentType:    "image/png",
		},
	}
	for _, testcase := range testcases {
		var actual []byte
		if testcase.contentType != "" {
			actual = AppendHTTPPacketUploadFile([]byte(testcase.origin), testcase.fieldName, testcase.fileName, testcase.fileContent, testcase.contentType)
		} else {
			actual = AppendHTTPPacketUploadFile([]byte(testcase.origin), testcase.fieldName, testcase.fileName, testcase.fileContent)
		}
		blocks := strings.SplitN(string(actual), "\r\n\r\n", 2)
		body := blocks[1]
		mutlipartReader := multipart.NewReader(bytes.NewBufferString(body))

		// compare old
		if testcase.oldfieldName != "" {
			compare(mutlipartReader, testcase.oldfieldName, testcase.oldfileName, testcase.oldFileContent, testcase.oldContentType)
		}

		// compare new
		compare(mutlipartReader, testcase.fieldName, testcase.fileName, testcase.fileContent, testcase.contentType)

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

func TestGetParamsFromBody(t *testing.T) {
	type Excepted struct {
		params map[string][]string
		useRaw bool
		err    error
	}

	testcases := []struct {
		name        string
		contentType string
		body        string
		expected    *Excepted
	}{
		{
			name:        "form-urlencoded",
			contentType: "application/x-www-form-urlencoded",
			body:        "a=1&b=2",
			expected: &Excepted{
				params: map[string][]string{"a": {"1"}, "b": {"2"}},
				useRaw: false,
				err:    nil,
			},
		},
		{
			name:        "form-data",
			contentType: "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			body: `------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="a"

1
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="b"

2
------WebKitFormBoundary7MA4YWxkTrZu0gW--`,
			expected: &Excepted{
				params: map[string][]string{"a": {"1"}, "b": {"2"}},
				useRaw: false,
				err:    nil,
			},
		},
		{
			name:        "json",
			contentType: "application/json",
			body:        `{"a":1,"b":2}`,
			expected: &Excepted{
				params: map[string][]string{"a": {"1"}, "b": {"2"}},
				useRaw: false,
				err:    nil,
			},
		},
		{
			name:        "complex-json",
			contentType: "application/json",
			body:        `{"a":[1, 2],"b":{"c": "d","q":"w"}}`,
			expected: &Excepted{
				params: map[string][]string{"a": {"2"}, "b[c]": {"d"}, "b[q]": {"w"}},
				useRaw: false,
				err:    nil,
			},
		},
		{
			name:        "xml",
			contentType: "application/xml",
			body:        `<COM><a>1</a><b>2</b></COM>`,
			expected: &Excepted{
				params: map[string][]string{"COM[a]": {"1"}, "COM[b]": {"2"}},
				useRaw: false,
				err:    nil,
			},
		},
		{
			name:        "form-urlencoded-with-complex-symbol",
			contentType: "application/x-www-form-urlencoded",
			body:        `x=1';%0d%0aWAITFOR%0d%0aDELAY%0d%0a'0:0:5'--+-`,
			expected: &Excepted{
				params: map[string][]string{"x": {`1';%0d%0aWAITFOR%0d%0aDELAY%0d%0a'0:0:5'--+-`}},
				useRaw: false,
				err:    nil,
			},
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			actualParams, actualUseRaw, actualError := GetParamsFromBody(testcase.contentType, []byte(testcase.body))

			require.Equalf(t, testcase.expected.params, actualParams, "[%s] GetParamsFromBody failed:", testcase.name)

			// if !mapEqual(testcase.expected.params, actualParams) {
			// 	t.Fatalf("[%s] GetParamsFromBody failed: %v != %v", testcase.name, actualParams, testcase.expected.params)
			// }
			if actualUseRaw != testcase.expected.useRaw {
				t.Fatalf("[%s] GetParamsFromBody failed: %v != %v", testcase.name, actualUseRaw, testcase.expected.useRaw)
			}
			if !errors.Is(actualError, testcase.expected.err) {
				t.Fatalf("[%s] GetParamsFromBody failed: %v != %v", testcase.name, actualError, testcase.expected.err)
			}
		})
	}
}
