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
	type testcase struct {
		packet   string
		key      string
		value    string
		expected string
	}

	testcases := []testcase{
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:      "CCC",
			value:    "ddd",
			expected: "CCC: ddd",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			key:      "CCC",
			value:    "ddd",
			expected: `CCC: aaa`,
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			key:      "CCC",
			value:    "ddd",
			expected: `CCC: aaa`,
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:      "Transfer-Encoding",
			value:    "chunked",
			expected: "Transfer-Encoding: chunked",
		},
	}

	for _, c := range testcases {
		byteResult := AppendHTTPPacketHeader([]byte(c.packet), c.key, c.value)
		spew.Dump(byteResult)
		require.Contains(t, string(byteResult), fmt.Sprintf("\r\n%s: %s", c.key, c.value), "AppendHTTPPacketHeader failed")

		if len(c.expected) > 0 {
			require.Contains(t, string(byteResult), c.expected, "AppendHTTPPacketHeader failed")
		}
	}
}

func TestAppendHTTPPacketHeaderIfNotExist(t *testing.T) {
	type testcase struct {
		packet   string
		key      string
		value    string
		expected string
		black    string
	}

	testcases := []testcase{
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:      "CCC",
			value:    "ddd",
			expected: "CCC: ddd",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			key:      "CCC",
			value:    "ddd",
			expected: "CCC: aaa",
			black:    "CCC: ddd",
		},
	}

	for _, c := range testcases {
		byteResult := AppendHTTPPacketHeaderIfNotExist([]byte(c.packet), c.key, c.value)
		spew.Dump(byteResult)
		require.Contains(t, string(byteResult), c.expected, "AppendHTTPPacketHeaderIfNotExist failed")
		if len(c.black) > 0 {
			require.NotContains(t, string(byteResult), c.black, "AppendHTTPPacketHeaderIfNotExist failed")
		}
	}
}

func TestReplaceHTTPPacketHeader(t *testing.T) {
	type testcase struct {
		packet string
		key    string
		value  string
		black  string
		whites []string
	}
	testcases := []testcase{
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:    "CCC",
			value:  "ddd",
			whites: []string{"CCC: ddd"},
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			key:    "CCC",
			value:  "ddd",
			black:  "aaa",
			whites: []string{"CCC: ddd"},
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			key:    "CCC",
			value:  "ddd",
			black:  "aaa",
			whites: []string{"DDDD: 11", "CCC: ddd"},
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:    "Transfer-Encoding",
			value:  "chunked",
			whites: []string{"Transfer-Encoding: chunked"},
		},
		{
			packet: `POST / HTTP/1.1
Host: www.baidu.com
Content-Length: 123

123`,
			key:    "c",
			value:  "123",
			whites: []string{"Content-Length: 3", "c: 123"},
		},
	}

	for _, c := range testcases {
		byteResult := ReplaceHTTPPacketHeader([]byte(c.packet), c.key, c.value)
		spew.Dump(byteResult)
		require.Contains(t, string(byteResult), c.key, "ReplaceHTTPPacketHeader failed")

		if c.black != "" {
			require.NotContains(t, string(byteResult), c.black, "ReplaceHTTPPacketHeader failed")
		}
		if len(c.whites) > 0 {
			for _, white := range c.whites {
				require.Contains(t, string(byteResult), white, "ReplaceHTTPPacketHeader failed")
			}
		}
	}
}

func TestDeleteHTTPPacketHeader(t *testing.T) {
	type testcase struct {
		packet string
		key    string
		black  string
		white  string
	}

	for _, c := range []testcase{
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com`,
			key:   "CCC",
			black: "CCC",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa`,
			key:   "CCC",
			black: "aaa",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			key:   "CCC",
			black: "aaa",
			white: "Host: www.baidu.com",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
CCC: aaa
DDDD: 11`,
			key:   "CCC",
			black: "aaa",
			white: "DDDD: 11",
		},
		{
			packet: `HTTP/1.1 200 OK
RefererenceA: www.baidu.com
CCC: aaa
DDDD: 11`,
			key:   "CCC",
			black: "aaa",
			white: "DDDD: 11",
		},
		{
			packet: `GET / HTTP/1.1
Host: www.baidu.com
Transfer-Encoding: chunked`,
			key:   "Transfer-Encoding",
			black: "chunked",
		},
	} {
		black := []byte(c.black)
		white := c.white
		byteResult := DeleteHTTPPacketHeader([]byte(c.packet), c.key)
		spew.Dump(byteResult)
		require.NotContains(t, string(byteResult), black, "DeleteHTTPPacketHeader failed")
		if white != "" {
			require.Contains(t, string(byteResult), white, "DeleteHTTPPacketHeader failed")
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
			"Cookie: c=1",
			"a=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; c=1`,
			"c",
			"Cookie: a=1",
			"c=1",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
Cookie: d=1`,
			"c",
			"Cookie: d=1",
			"c=",
		},
		{
			`GET / HTTP/1.1
Host: www.baidu.com
`,
			"cfffffffff",
			"baidu.com",
			"Cookie",
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

func TestGetFullHTTPRequestQueryParam(t *testing.T) {
	testcases := []struct {
		origin   string
		expected map[string][]string
	}{
		{
			origin: `GET /?a=1&b=2 HTTP/1.1
		Host: www.baidu.com
		`,

			expected: map[string][]string{
				"a": {"1"},
				"b": {"2"},
			},
		},
		{
			origin: `GET /?a=1&a=2 HTTP/1.1
		Host: www.baidu.com
		`,

			expected: map[string][]string{
				"a": {"1", "2"},
			},
		},
		{
			origin: `GET /?a&b=2 HTTP/1.1
Host: www.baidu.com
`,

			expected: map[string][]string{
				"a": {""},
				"b": {"2"},
			},
		},
	}
	for _, testcase := range testcases {
		actual := GetFullHTTPRequestQueryParams([]byte(testcase.origin))
		if !reflect.DeepEqual(actual, testcase.expected) {
			t.Fatalf("GetAllHTTPRequestPostParam failed: %v", actual)
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

func TestGetFullHTTPRequestPostParam(t *testing.T) {
	testcases := []struct {
		origin   string
		expected map[string][]string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,

			expected: map[string][]string{
				"a": {"1"},
				"b": {"2"},
			},
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&a=2`,

			expected: map[string][]string{
				"a": {"1", "2"},
			},
		},
	}
	for _, testcase := range testcases {
		actual := GetFullHTTPRequestPostParams([]byte(testcase.origin))
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
		origin     string
		key        string
		value      string
		whitelists []string
		blacklists []string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1&b=2`,
			key:        "a",
			value:      "3",
			whitelists: []string{"\r\n\r\na=3&b=2", "Content-Type: application/x-www-form-urlencoded"},
			blacklists: []string{"a=1"},
		},
	}
	for _, testcase := range testcases {

		actual := string(ReplaceHTTPPacketPostParam([]byte(testcase.origin), testcase.key, testcase.value))
		for _, whitelist := range testcase.whitelists {
			if !strings.Contains(actual, whitelist) {
				t.Fatalf("ReplaceHTTPPacketPostParam failed: %s", string(actual))
			}
		}
		for _, blacklist := range testcase.blacklists {
			if strings.Contains(actual, blacklist) {
				t.Fatalf("ReplaceHTTPPacketPostParam failed: %s", string(actual))
			}
		}
	}
}

func TestReplaceAllHttpPacketPostParams(t *testing.T) {
	testcases := []struct {
		origin     string
		values     map[string]string
		whitelists []string
		blacklists []string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

`,
			values: map[string]string{"a": "1", "b": "2"},
			whitelists: []string{
				"Content-Type: application/x-www-form-urlencoded",
				"a=1&b=2",
			},
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded

c=3`,
			values: map[string]string{"a": "1", "b": "2"},
			whitelists: []string{
				"Content-Type: application/x-www-form-urlencoded",
				"\r\n\r\na=1&b=2",
			},
			blacklists: []string{
				"c=3",
			},
		},
	}
	for _, testcase := range testcases {
		actual := string(ReplaceAllHTTPPacketPostParams([]byte(testcase.origin), testcase.values))
		for _, whitelist := range testcase.whitelists {
			if !strings.Contains(actual, whitelist) {
				t.Fatalf("ReplaceAllHTTPPacketPostParams failed: %s", string(actual))
			}
		}
		for _, blacklist := range testcase.blacklists {
			if strings.Contains(actual, blacklist) {
				t.Fatalf("ReplaceAllHTTPPacketPostParams failed: %s", string(actual))
			}
		}
	}
}

func TestReplaceAllHttpPacketPostParamsWithoutEscape(t *testing.T) {
	testcases := []struct {
		origin     string
		values     map[string]string
		whitelists []string
		blacklists []string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded

c=1&d=2`,
			values: map[string]string{"a": "{{int(1-100)}}", "b": "2"},
			whitelists: []string{
				"Content-Type: application/x-www-form-urlencoded",
				"\r\n\r\na={{int(1-100)}}&b=2",
			},
			blacklists: []string{
				"c=1&d=2",
			},
		},
	}
	for _, testcase := range testcases {
		actual := string(ReplaceAllHTTPPacketPostParamsWithoutEscape([]byte(testcase.origin), testcase.values))
		for _, whitelist := range testcase.whitelists {
			if !strings.Contains(actual, whitelist) {
				t.Fatalf("ReplaceAllHTTPPacketPostParamsWithoutEscape failed: %s", string(actual))
			}
		}
		for _, blacklist := range testcase.blacklists {
			if strings.Contains(actual, blacklist) {
				t.Fatalf("ReplaceAllHTTPPacketPostParamsWithoutEscape failed: %s", string(actual))
			}
		}
	}
}

func TestAppendHTTPPacketPostParam(t *testing.T) {
	testcases := []struct {
		origin     string
		key        string
		value      string
		whitelists []string
		blacklists []string
	}{
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

`,
			key:   "a",
			value: "1",
			whitelists: []string{
				"Content-Type: application/x-www-form-urlencoded",
				"\r\n\r\na=1",
			},
		},
		{
			origin: `POST / HTTP/1.1
Host: www.baidu.com

a=1`,
			key:   "b",
			value: "2",
			whitelists: []string{
				"Content-Type: application/x-www-form-urlencoded",
				"\r\n\r\na=1&b=2",
			},
		},
	}
	for _, testcase := range testcases {

		actual := string(AppendHTTPPacketPostParam([]byte(testcase.origin), testcase.key, testcase.value))
		for _, whitelist := range testcase.whitelists {
			if !strings.Contains(actual, whitelist) {
				t.Fatalf("AppendHTTPPacketPostParam failed: %s", string(actual))
			}
		}
		for _, blacklist := range testcase.blacklists {
			if strings.Contains(actual, blacklist) {
				t.Fatalf("AppendHTTPPacketPostParam failed: %s", string(actual))
			}
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
			key:             "b",
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

func TestReplaceHTTPPacketFormEncodedMultipleCalls(t *testing.T) {
	// 测试多次调用同一个key不会产生重复字段
	raw := `POST / HTTP/1.1
Host: example.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Length: 46

------WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Disposition: form-data; name="user"

11111111
------WebKitFormBoundaryJ5VMHqyPTW0aM97D--`

	// 第一次调用：应该添加新字段
	result1 := ReplaceHTTPPacketFormEncoded([]byte(raw), "a", "123")

	// 第二次调用：应该替换现有字段，不应该添加新字段
	result2 := ReplaceHTTPPacketFormEncoded(result1, "a", "456")

	// 第三次调用：应该替换现有字段，不应该添加新字段
	result3 := ReplaceHTTPPacketFormEncoded(result2, "a", "789")

	// 解析最终结果，检查字段数量
	blocks := strings.SplitN(string(result3), "\r\n\r\n", 2)
	body := blocks[1]
	multipartReader := multipart.NewReader(bytes.NewBufferString(body))

	// 统计字段数量
	fieldCount := 0
	fields := make(map[string]string)

	for {
		part, err := multipartReader.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}

		fieldCount++
		fieldName := part.FormName()
		buf := new(bytes.Buffer)
		io.Copy(buf, part)
		fields[fieldName] = buf.String()
	}

	// 应该只有2个字段：user 和 a
	require.Equal(t, 2, fieldCount, "应该只有2个字段")
	require.Equal(t, "11111111", fields["user"], "user字段值应该保持不变")
	require.Equal(t, "789", fields["a"], "a字段值应该是最后一次设置的值")
}

func TestReplaceHTTPPacketFormEncodedReplaceExisting(t *testing.T) {
	// 测试替换现有字段的情况
	raw := `POST / HTTP/1.1
Host: example.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryJ5VMHqyPTW0aM97D

------WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Disposition: form-data; name="existing"

old_value
------WebKitFormBoundaryJ5VMHqyPTW0aM97D--`

	result := ReplaceHTTPPacketFormEncoded([]byte(raw), "existing", "new_value")

	// 解析结果
	blocks := strings.SplitN(string(result), "\r\n\r\n", 2)
	body := blocks[1]
	multipartReader := multipart.NewReader(bytes.NewBufferString(body))

	// 检查字段
	part, err := multipartReader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "existing", part.FormName())

	buf := new(bytes.Buffer)
	io.Copy(buf, part)
	require.Equal(t, "new_value", buf.String())

	// 应该没有更多字段
	_, err = multipartReader.NextPart()
	require.Error(t, err)
	require.True(t, errors.Is(err, io.EOF))
}

func TestReplaceHTTPPacketFormEncodedAddNew(t *testing.T) {
	// 测试添加新字段的情况
	raw := `POST / HTTP/1.1
Host: example.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryJ5VMHqyPTW0aM97D

------WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Disposition: form-data; name="existing"

old_value
------WebKitFormBoundaryJ5VMHqyPTW0aM97D--`

	result := ReplaceHTTPPacketFormEncoded([]byte(raw), "new_field", "new_value")

	// 解析结果
	blocks := strings.SplitN(string(result), "\r\n\r\n", 2)
	body := blocks[1]
	multipartReader := multipart.NewReader(bytes.NewBufferString(body))

	// 检查字段
	fields := make(map[string]string)
	for {
		part, err := multipartReader.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		io.Copy(buf, part)
		fields[part.FormName()] = buf.String()
	}

	// 应该有两个字段
	require.Equal(t, 2, len(fields))
	require.Equal(t, "old_value", fields["existing"])
	require.Equal(t, "new_value", fields["new_field"])
}

func TestReplaceHTTPPacketFormEncodedWithDotInFieldName(t *testing.T) {
	// 测试包含点号的字段名，验证多次替换不会产生重复字段
	raw := `POST / HTTP/1.1
Host: example.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Length: 46

------WebKitFormBoundaryJ5VMHqyPTW0aM97D
Content-Disposition: form-data; name="user.mark"

11111111
------WebKitFormBoundaryJ5VMHqyPTW0aM97D--`

	// 解析multipart数据的辅助函数
	parseMultipartFields := func(data []byte) map[string]string {
		blocks := strings.SplitN(string(data), "\r\n\r\n", 2)
		if len(blocks) < 2 {
			return nil
		}
		body := blocks[1]
		multipartReader := multipart.NewReader(bytes.NewBufferString(body))

		fields := make(map[string]string)
		for {
			part, err := multipartReader.NextPart()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}

			buf := new(bytes.Buffer)
			io.Copy(buf, part)
			fields[part.FormName()] = buf.String()
		}
		return fields
	}

	// 第一次替换
	result1 := ReplaceHTTPPacketFormEncoded([]byte(raw), "user.mark", "123")
	fields1 := parseMultipartFields(result1)

	// 验证第一次替换结果
	require.Equal(t, 1, len(fields1), "第一次替换后应该只有1个字段")
	require.Equal(t, "123", fields1["user.mark"], "user.mark字段值应该是123")

	// 第二次替换
	result2 := ReplaceHTTPPacketFormEncoded(result1, "user.mark", "1234")
	fields2 := parseMultipartFields(result2)

	// 验证第二次替换结果
	require.Equal(t, 1, len(fields2), "第二次替换后应该只有1个字段")
	require.Equal(t, "1234", fields2["user.mark"], "user.mark字段值应该是1234")

	// 第三次替换
	result3 := ReplaceHTTPPacketFormEncoded(result2, "user.mark", "12345")
	fields3 := parseMultipartFields(result3)

	// 验证第三次替换结果
	require.Equal(t, 1, len(fields3), "第三次替换后应该只有1个字段")
	require.Equal(t, "12345", fields3["user.mark"], "user.mark字段值应该是12345")

	// 额外验证：确保没有产生重复字段
	// 通过检查multipart body中"user.mark"的出现次数
	body3 := strings.SplitN(string(result3), "\r\n\r\n", 2)[1]
	userMarkCount := strings.Count(body3, `name="user.mark"`)
	require.Equal(t, 1, userMarkCount, "multipart body中应该只出现一次user.mark字段")
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

func TestReplaceHTTPPacketBodyJson(t *testing.T) {
	testcases := []struct {
		origin   string
		jsonMap  map[string]interface{}
		expected string
	}{
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			jsonMap: map[string]interface{}{"a": 1, "b": 2},
			expected: `GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 13

{"a":1,"b":2}`,
		},
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			jsonMap: map[string]interface{}{"a": 1, "b": "2"},
			expected: `GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 15

{"a":1,"b":"2"}`,
		},
		{
			origin: `GET / HTTP/1.1
Host: www.baidu.com
`,
			jsonMap: map[string]interface{}{"a": 1, "b": map[string]interface{}{"c": "d", "e": 2}},
			expected: `GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 27 

{"a":1,"b":{"c":"d","e":2}}`,
		},
	}
	for _, testcase := range testcases {

		actual := ReplaceHTTPPacketJsonBody([]byte(testcase.origin), testcase.jsonMap)
		expected := FixHTTPPacketCRLF([]byte(testcase.expected), false)
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("ReplaceHTTPPacketJsonBody failed: want %s got %s", string(expected), string(actual))
		}
	}
}
