package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
)

func TestUtf8Converter(t *testing.T) {

	chChar := []byte{229, 147, 136}
	overLongChChar := Utf8EncodeBySpecificLength(chChar, 4)
	assert.Len(t, overLongChChar, 4)
	newRes, err := SimplifyUtf8(overLongChChar)
	assert.NoError(t, err)
	assert.Len(t, newRes, 3)
	assert.Equal(t, string(newRes), "哈")

	overLongChChar = Utf8EncodeBySpecificLength(chChar, 2)
	assert.Len(t, overLongChChar, 3)
	newRes, err = SimplifyUtf8(overLongChChar)
	assert.NoError(t, err)
	assert.Len(t, newRes, 3)
	assert.Equal(t, string(newRes), "哈")

	singleChars := []byte("abc")
	overLongChChar = Utf8EncodeBySpecificLength(singleChars, 4)
	assert.Len(t, overLongChChar, 12)
	newRes, err = SimplifyUtf8(overLongChChar)
	assert.NoError(t, err)
	assert.Len(t, newRes, 3)
	assert.Equal(t, string(newRes), "abc")
}
func TestRemoveUnprintableChars(t *testing.T) {
	cases := map[string]string{
		"\x00W\xffO\x00R\x00K": `\x00W\xffO\x00R\x00K`,
	}
	for input, output := range cases {
		if result := RemoveUnprintableChars(input); result == output {
			continue
		} else {
			t.Logf("expect %#v got %#v", output, result)
			t.FailNow()
		}
	}
}

func TestParseStringToLines(t *testing.T) {
	a := ParseStringToLines(`abc
ccc
ddd`)
	spew.Dump(a)
	assert.Equal(t, a[0], "abc")
	assert.Equal(t, a[1], "ccc")
	assert.Equal(t, a[2], "ddd")
}

func TestMUSTPASS_UrlJoin2(t *testing.T) {
	u, err := UrlJoin("https://baidu.com/a/b.html", "c.html")
	if err != nil {
		panic(err)
	}
	assert.Equal(t, "https://baidu.com/a/c.html", u)
}

func TestMUSTPASS_UrlJoin(t *testing.T) {
	cases := map[string][2]string{
		"/abc":                          {"https://baidu.com/root", "https://baidu.com/abc"},
		"/abc/":                         {"https://baidu.com/root", "https://baidu.com/abc/"},
		"abc":                           {"https://baidu.com/root", "https://baidu.com/root/abc"},
		"abc/":                          {"https://baidu.com/root", "https://baidu.com/root/abc/"},
		"/index.php":                    {"https://baidu.com/root", "https://baidu.com/index.php"},
		"/index.php?a=b":                {"https://baidu.com/root", "https://baidu.com/index.php?a=b"},
		"login.php":                     {"https://baidu.com/root", "https://baidu.com/root/login.php"},
		"login.php?ab=1":                {"https://baidu.com/root", "https://baidu.com/root/login.php?ab=1"},
		"./index.php":                   {"https://baidu.com/root", "https://baidu.com/root/index.php"},
		"../index.php":                  {"https://baidu.com/root", "https://baidu.com/index.php"},
		"./../.././../index.php":        {"https://baidu.com/root/a/b/c/d/e/f", "https://baidu.com/root/a/b/c/index.php"},
		"./././././.././index.php":      {"https://baidu.com/root", "https://baidu.com/index.php"},
		"./index.php?c=123":             {"https://baidu.com/root", "https://baidu.com/root/index.php?c=123"},
		"https://example.com/index.php": {"https://baidu.com/root", "https://example.com/index.php"},
		"http://example.com/index.php":  {"https://baidu.com/root", "http://example.com/index.php"},

		// 这两个不知道应不应该在这么做，但是先这样吧
		"./././././.././a/b/./index.php":  {"https://baidu.com/root", "https://baidu.com/a/b/./index.php"},
		"./././././.././a/b/../index.php": {"https://baidu.com/root", "https://baidu.com/a/b/../index.php"},
	}
	for input, output := range cases {
		input := input
		origin := output[0]
		expected := output[1]
		if result, err := UrlJoin(origin, input); err != nil {
			panic(err)
		} else {
			if result != expected {
				t.Logf("origin: %v input %v", origin, input)
				t.Logf("expect %#v got %#v", expected, result)
				t.FailNow()
			}
		}
	}
}

func TestMUSTPASS_ParseStringToHostPort(t *testing.T) {
	type Result struct {
		Host   string
		Port   int
		hasErr bool
	}
	cases := map[string]Result{
		"http://baidu.com":     {Host: "baidu.com", Port: 80},
		"https://baidu.com":    {Host: "baidu.com", Port: 443},
		"https://baidu.com:88": {Host: "baidu.com", Port: 88},
		"http://baidu.com:88":  {Host: "baidu.com", Port: 88},
		"ws://baidu.com":       {Host: "baidu.com", Port: 80},
		"wss://baidu.com":      {Host: "baidu.com", Port: 443},
		"1.2.3.4:1":            {Host: "1.2.3.4", Port: 1},
		"baidu.com:1":          {Host: "baidu.com", Port: 1},
		"http://[::1]:1":       {Host: "::1", Port: 1},
		"baidu.com":            {Host: "baidu.com", Port: 0, hasErr: true},
		"1.2.3.5":              {Host: "1.2.3.5", Port: 0, hasErr: true},
		"[1:123:123:123]":      {Host: "1:123:123:123", Port: 0, hasErr: true},
		"::1":                  {Host: "::1", Port: 0, hasErr: true},
	}

	for raw, result := range cases {
		host, port, err := ParseStringToHostPort(raw)
		require.Equal(t, result.Host, host)
		require.Equal(t, result.Port, port)
		if result.hasErr {
			require.Error(t, err, "should have error")
		} else {
			require.NoError(t, err, "should not have error")
		}
	}
}

func TestMUSTPASS_SliceGroup(t *testing.T) {
	s := SliceGroup([]string{
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
	}, 3)
	log.Info(spew.Sdump(s))
	assert.True(t, len(s) == 7, "%v", spew.Sdump(s))
}

func TestMUSTPASS_HostPort_AppendDefaultPort(t *testing.T) {
	type Case struct {
		Raw  string
		Port int
		Res  string
	}
	cases := []Case{
		{"::1", 113, "[::1]:113"},
		{"baidu.com", 88, "baidu.com:88"},
		{"baidu.com:80", 80, "baidu.com:80"},
		{"http://127.0.0.1", 111, "127.0.0.1:80"},
		{"http://127.0.0.1:8888", 111, "127.0.0.1:8888"},
		{"127.0.0.1", 113, "127.0.0.1:113"},
		{"[::1]:111", 113, "[::1]:111"},
		{"https://[::1]:111", 113, "[::1]:111"},
	}
	for _, c := range cases {
		if res := AppendDefaultPort(c.Raw, c.Port); res != c.Res {
			t.Errorf("expect %s got %s", c.Res, res)
		}
	}
}

func TestMUSTPASS_HostPort(t *testing.T) {
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("127.0.0.1:80", 8787))
	assert.Equal(t, "127.0.0.1:8787", AppendDefaultPort("127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("http://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:443", AppendDefaultPort("https://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:7777", AppendDefaultPort("https://127.0.0.1:7777", 8787))
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("ws://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:443", AppendDefaultPort("wss://127.0.0.1", 8787))
	assert.Equal(t, ":7777", AppendDefaultPort(":7777", 8787))
	assert.Equal(t, ":8787", AppendDefaultPort(":8787", 8787))
	assert.Equal(t, "127.0.0.1:8787", AppendDefaultPort("127.0.0.1", 8787))
	assert.Equal(t, "yaklang.io:8787", AppendDefaultPort("yaklang.io", 8787))
}

func TestMUSSPASS_StringGlobArrayContains(t *testing.T) {
	assert.Equal(t, true, StringGlobArrayContains([]string{"/api/push?pass=*"}, "localhost/api/push?pass=123"))
	assert.Equal(t, true, StringGlobArrayContains([]string{"/api/push?pass=*&abc=123"}, "localhost/api/push?pass=123&abc=123"))
}

func TestUnquoteANSIC(t *testing.T) {
	// 定义测试表
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
		errMsg   string
	}{
		// 基本字符串测试
		{
			name:     "empty string",
			input:    "''",
			expected: "",
		},
		{
			name:     "simple string without escapes",
			input:    "'hello'",
			expected: "hello",
		},

		// 简单转义序列测试
		{
			name:     "basic escape sequences",
			input:    "'\\a\\b\\f\\n\\r\\t\\v'",
			expected: "\a\b\f\n\r\t\v",
		},
		{
			name:     "quote escapes",
			input:    `'\\\'\\\"'`,
			expected: `\'\"`,
		},
		{
			name:     "backslash escape",
			input:    "'\\\\'",
			expected: "\\",
		},

		// 十六进制转义测试
		{
			name:     "hex escape - lowercase",
			input:    "'\\x41\\x42\\x43'",
			expected: "ABC",
		},
		{
			name:     "hex escape - uppercase",
			input:    "'\\x61\\x62\\x63'",
			expected: "abc",
		},
		{
			name:     "hex escape - mix with normal chars",
			input:    "'Hello\\x20World'",
			expected: "Hello World",
		},

		// 八进制转义测试
		{
			name:     "octal escape - single digit",
			input:    "'\\7'",
			expected: "\x07",
		},
		{
			name:     "octal escape - two digits",
			input:    "'\\12'",
			expected: "\x0A",
		},
		{
			name:     "octal escape - three digits",
			input:    "'\\101\\102\\103'",
			expected: "ABC",
		},
		{
			name:     "octal escape - mix with normal chars",
			input:    "'Hello\\040World'",
			expected: "Hello World",
		},
		{
			name:     "ansi-c escape",
			input:    "'\\a\\b\\f\\n\\r\\t\\v\\'\\\"\\\\\\x41\\x42\\x43\\101\\102\\103'",
			expected: "\a\b\f\n\r\t\v'\"\\ABCABC",
		},
		// 组合测试
		{
			name:     "mixed escapes",
			input:    "'\\x41\\n\\102\\t\\103'",
			expected: "A\nB\tC",
		},
		{
			name:     "complex string",
			input:    "'Hello\\040\\x57\\157rld\\041'",
			expected: "Hello World!",
		},

		// 错误情况测试
		{
			name:    "error - no starting quote",
			input:   "hello'",
			wantErr: true,
			errMsg:  "string must begin and end with '",
		},
		{
			name:    "error - no ending quote",
			input:   "'hello",
			wantErr: true,
			errMsg:  "string must begin and end with '",
		},
		{
			name:    "error - invalid escape sequence",
			input:   "'\\z'",
			wantErr: true,
			errMsg:  "invalid escape sequence: \\z",
		},
		{
			name:    "error - incomplete hex escape",
			input:   "'\\x4'",
			wantErr: true,
			errMsg:  "invalid hex escape sequence",
		},
		{
			name:    "error - invalid hex escape",
			input:   "'\\xZZ'",
			wantErr: true,
			errMsg:  "invalid hex escape sequence: ZZ",
		},
		{
			name:    "error - escape at end of string",
			input:   "'\\",
			wantErr: true,
			errMsg:  "string must begin and end with '",
		},
		{
			name:    "error - invalid octal value",
			input:   "'\\400'",
			wantErr: true,
			errMsg:  "invalid octal escape sequence: 400",
		},

		// 边界测试
		{
			name:     "boundary - all ASCII printable characters",
			input:    "'\\x20\\x21\\x22\\x23\\x24\\x25\\x26\\x27\\x28\\x29\\x2A\\x2B\\x2C\\x2D\\x2E\\x2F'",
			expected: " !\"#$%&'()*+,-./",
		},
		{
			name:     "boundary - max octal value",
			input:    "'\\377'",
			expected: "\xFF",
		},
		{
			name:     "boundary - consecutive escapes",
			input:    "'\\x00\\x01\\x02'",
			expected: "\x00\x01\x02",
		},
	}

	// 运行测试用例
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnquoteANSIC(tt.input)

			// 检查错误情况
			if tt.wantErr {
				if err == nil {
					t.Errorf("UnquoteANSIC() expected error, got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("UnquoteANSIC() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			// 检查正常情况
			if err != nil {
				t.Errorf("UnquoteANSIC() unexpected error = %v", err)
				return
			}

			if got != tt.expected {
				t.Errorf("UnquoteANSIC() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// 基准测试
func BenchmarkUnquoteANSIC(b *testing.B) {
	testCases := []string{
		"'simple string'",
		"'string\\x20with\\x20hex'",
		"'string\\040with\\040octal'",
		"'mixed\\x20\\n\\t\\r\\040string'",
	}

	for _, tc := range testCases {
		b.Run(tc, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = UnquoteANSIC(tc)
			}
		})
	}
}

func TestPrefixLines(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		prefix   string
		expected string
	}{
		{
			name:     "single line string",
			input:    "hello",
			prefix:   "> ",
			expected: "> hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			prefix:   "> ",
			expected: "> line1\n> line2",
		},
		{
			name:     "three lines with different prefix",
			input:    "a\nb\nc",
			prefix:   "# ",
			expected: "# a\n# b\n# c",
		},
		{
			name:     "empty prefix",
			input:    "line1\nline2",
			prefix:   "",
			expected: "line1\nline2",
		},
		{
			name:     "empty string",
			input:    "",
			prefix:   "> ",
			expected: "> ",
		},
		{
			name:     "string with empty lines",
			input:    "line1\n\nline3",
			prefix:   "> ",
			expected: "> line1\n> \n> line3",
		},
		{
			name:     "io.Reader input",
			input:    bytes.NewReader([]byte("reader1\nreader2")),
			prefix:   ">>> ",
			expected: ">>> reader1\n>>> reader2",
		},
		{
			name:     "io.Reader single line",
			input:    bytes.NewReader([]byte("single")),
			prefix:   ">>> ",
			expected: ">>> single",
		},
		{
			name:     "integer input (interface{})",
			input:    123,
			prefix:   "num: ",
			expected: "num: 123",
		},
		{
			name:     "multiline with tabs and spaces",
			input:    "	line1\n  line2  \n\tline3\t",
			prefix:   "* ",
			expected: "* \tline1\n*   line2  \n* \tline3\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrefixLines(tt.input, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixLinesWithLineNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "single line string",
			input:    "hello",
			expected: "1 | hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			expected: "1 | line1\n2 | line2",
		},
		{
			name:     "three lines",
			input:    "first\nsecond\nthird",
			expected: "1 | first\n2 | second\n3 | third",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "1 | ",
		},
		{
			name:     "string with empty lines",
			input:    "line1\n\nline3",
			expected: "1 | line1\n2 | \n3 | line3",
		},
		{
			name:     "io.Reader input",
			input:    bytes.NewReader([]byte("reader1\nreader2\nreader3")),
			expected: "1 | reader1\n2 | reader2\n3 | reader3",
		},
		{
			name:     "io.Reader single line",
			input:    bytes.NewReader([]byte("single")),
			expected: "1 | single",
		},
		{
			name:     "integer input (interface{})",
			input:    123,
			expected: "1 | 123",
		},
		{
			name:     "ten lines (test double digit numbers)",
			input:    "1\n2\n3\n4\n5\n6\n7\n8\n9\n10",
			expected: "1 | 1\n2 | 2\n3 | 3\n4 | 4\n5 | 5\n6 | 6\n7 | 7\n8 | 8\n9 | 9\n10 | 10",
		},
		{
			name:     "multiline with whitespace",
			input:    "  line1  \n\tline2\t\n   ",
			expected: "1 |   line1  \n2 | \tline2\t\n3 |    ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrefixLinesWithLineNumbers(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests for new functions
func BenchmarkPrefixLines(b *testing.B) {
	testInput := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	prefix := "> "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrefixLines(testInput, prefix)
	}
}

func BenchmarkPrefixLinesWithLineNumbers(b *testing.B) {
	testInput := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrefixLinesWithLineNumbers(testInput)
	}
}

func TestPrefixLinesReader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "single line string",
			input:    "hello",
			prefix:   "> ",
			expected: "> hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			prefix:   "> ",
			expected: "> line1\n> line2",
		},
		{
			name:     "three lines with different prefix",
			input:    "a\nb\nc",
			prefix:   "# ",
			expected: "# a\n# b\n# c",
		},
		{
			name:     "empty prefix",
			input:    "line1\nline2",
			prefix:   "",
			expected: "line1\nline2",
		},
		{
			name:     "empty string",
			input:    "",
			prefix:   "> ",
			expected: "> ",
		},
		{
			name:     "string with empty lines",
			input:    "line1\n\nline3",
			prefix:   "> ",
			expected: "> line1\n> \n> line3",
		},
		{
			name:     "multiline with tabs and spaces",
			input:    "\tline1\n  line2  \n\tline3\t",
			prefix:   "* ",
			expected: "* \tline1\n*   line2  \n* \tline3\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			resultReader := PrefixLinesReader(input, tt.prefix)

			resultData, err := io.ReadAll(resultReader)
			require.NoError(t, err)

			result := string(resultData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixLinesReaderSimple(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "single line string (always adds prefix)",
			input:    "hello",
			prefix:   "> ",
			expected: "> hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			prefix:   "> ",
			expected: "> line1\n> line2",
		},
		{
			name:     "three lines",
			input:    "a\nb\nc",
			prefix:   "# ",
			expected: "# a\n# b\n# c",
		},
		{
			name:     "empty string",
			input:    "",
			prefix:   "> ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			resultReader := PrefixLinesReaderSimple(input, tt.prefix)

			resultData, err := io.ReadAll(resultReader)
			require.NoError(t, err)

			result := string(resultData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixLinesWithLineNumbersReader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line string",
			input:    "hello",
			expected: "1 | hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			expected: "1 | line1\n2 | line2",
		},
		{
			name:     "three lines",
			input:    "first\nsecond\nthird",
			expected: "1 | first\n2 | second\n3 | third",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "1 | ",
		},
		{
			name:     "string with empty lines",
			input:    "line1\n\nline3",
			expected: "1 | line1\n2 | \n3 | line3",
		},
		{
			name:     "ten lines (test double digit numbers)",
			input:    "1\n2\n3\n4\n5\n6\n7\n8\n9\n10",
			expected: "1 | 1\n2 | 2\n3 | 3\n4 | 4\n5 | 5\n6 | 6\n7 | 7\n8 | 8\n9 | 9\n10 | 10",
		},
		{
			name:     "multiline with whitespace",
			input:    "  line1  \n\tline2\t\n   ",
			expected: "1 |   line1  \n2 | \tline2\t\n3 |    ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			resultReader := PrefixLinesWithLineNumbersReader(input)

			resultData, err := io.ReadAll(resultReader)
			require.NoError(t, err)

			result := string(resultData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixLinesWithLineNumbersReaderStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line string",
			input:    "hello",
			expected: "1 | hello",
		},
		{
			name:     "multi-line string",
			input:    "line1\nline2",
			expected: "1 | line1\n2 | line2",
		},
		{
			name:     "three lines",
			input:    "first\nsecond\nthird",
			expected: "1 | first\n2 | second\n3 | third",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "1 | ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			resultReader := PrefixLinesWithLineNumbersReader(input)

			resultData, err := io.ReadAll(resultReader)
			require.NoError(t, err)

			result := string(resultData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests for Reader versions
func BenchmarkPrefixLinesReader(b *testing.B) {
	testInput := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	prefix := "> "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := strings.NewReader(testInput)
		resultReader := PrefixLinesReader(input, prefix)
		io.ReadAll(resultReader)
	}
}

func BenchmarkPrefixLinesWithLineNumbersReader(b *testing.B) {
	testInput := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := strings.NewReader(testInput)
		resultReader := PrefixLinesWithLineNumbersReader(input)
		io.ReadAll(resultReader)
	}
}
