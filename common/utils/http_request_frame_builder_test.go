package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestHttpRequestFrameBuilder_3(t *testing.T) {
	packet := `
user-agent: A
:method

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "A", h[0][1])
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
}

func TestHttpRequestFrameBuilder_4(t *testing.T) {
	packet := `
user-ag
:method

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-ag", h[0][0])
	require.Equal(t, "", h[0][1])
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
}

func TestHttpRequestFrameBuilder_4_1(t *testing.T) {
	packet := `
user-ag:
:method:

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-ag", h[0][0])
	require.Equal(t, "", h[0][1])
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
}

func TestHttpRequestFrameBuilder_4_2(t *testing.T) {
	packet := `
user-ag:
method:

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-ag", h[0][0])
	require.Equal(t, "", h[0][1])
	require.Equal(t, "method", h[1][0])
	require.Equal(t, "", h[1][1])
}

func TestHttpRequestFrameBuilder_4_34(t *testing.T) {
	packet := `
:method: 
:authority: www.baidu.com

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
	require.Equal(t, ":authority", fh[1][0])
	require.Equal(t, "www.baidu.com", fh[1][1])
}

func TestHttpRequestFrameBuilder_4_341(t *testing.T) {
	packet := `
:method: 
:authority:  www.baidu.com

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
	require.Equal(t, ":authority", fh[1][0])
	require.Equal(t, " www.baidu.com", fh[1][1])
}

func TestHttpRequestFrameBuilder_4_2_1(t *testing.T) {
	packet := `
user-ag:
method: G

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-ag", h[0][0])
	require.Equal(t, "", h[0][1])
	require.Equal(t, "method", h[1][0])
	require.Equal(t, "G", h[1][1])
}

func TestHttpRequestFrameBuilder_2(t *testing.T) {
	packet := `
user-agent: A

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "A", h[0][1])
	spew.Dump(h)
}

func TestHttpRequestFrameBuilder(t *testing.T) {
	packet := `
:method: <<<METHOD` + "\nGET\n123\n123\n123\n123\n" + `METHOD
:authority: www.baidu.com
:scheme: https
user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	require.Equal(t, 3, len(fh))
	require.Equal(t, 1, len(h))

	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "GET\n123\n123\n123\n123", fh[0][1])
	require.Equal(t, ":authority", fh[1][0])
	require.Equal(t, "www.baidu.com", fh[1][1])
	require.Equal(t, ":scheme", fh[2][0])
	require.Equal(t, "https", fh[2][1])
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3", h[0][1])
}

func TestHTTPFrameParser_Body(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantBody    string
		wantErr     bool
		description string
	}{
		{
			name: "正常HTTP请求体",
			input: `
:method: GET

abcdefghijklmnopqrstuvwxyz`,
			wantBody:    "abcdefghijklmnopqrstuvwxyz",
			wantErr:     false,
			description: "包含标准HTTP请求体的请求",
		},
		{
			name: "空请求体",
			input: `
:method: GET

`,
			wantBody:    "",
			wantErr:     false,
			description: "请求体为空的情况",
		},
		{
			name: "无分隔行的请求体",
			input: `:method: GET
abcdefg`,
			wantBody:    "",
			wantErr:     false,
			description: "头部和请求体之间没有空行",
		},
		{
			name: "多个空行分隔(LF)",
			input: `
:method: GET` + "\n\n\n" + `body content`,
			wantBody:    "\nbody content",
			wantErr:     false,
			description: "头部和请求体之间有多个空行",
		},
		{
			name: "多个空行分隔(CRLF)",
			input: `
:method: GET` + "\r\n\r\n\r\n" + `body content`,
			wantBody:    "\r\nbody content",
			wantErr:     false,
			description: "头部和请求体之间有多个空行",
		},
		{
			name: "特殊字符请求体",
			input: `
:method: GET

!@#$%^&*()_+` + "\n\r\t",
			wantBody:    "!@#$%^&*()_+\n\r\t",
			wantErr:     false,
			description: "请求体包含特殊字符",
		},
		{
			name: "超长请求体",
			input: `
:method: GET

` + strings.Repeat("a", 1024),
			wantBody:    strings.Repeat("a", 1024),
			wantErr:     false,
			description: "大量数据的请求体",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, resultReader, err := HTTPFrameParser(bytes.NewReader([]byte(tt.input)))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			body, err := io.ReadAll(resultReader)
			require.NoError(t, err)
			require.Equal(t, tt.wantBody, string(body))
		})
	}
}

func TestHTTPFrameParser_Cases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantFakeLen   int
		wantHeaderLen int
		wantErr       bool
	}{
		{
			name:          "基本HTTP头",
			input:         ":method: GET\n:authority: www.example.com",
			wantFakeLen:   2,
			wantHeaderLen: 0,
			wantErr:       false,
		},
		{
			name:          "空输入",
			input:         "",
			wantFakeLen:   0,
			wantHeaderLen: 0,
			wantErr:       true,
		},
		{
			name:          "无效格式",
			input:         "invalid format",
			wantFakeLen:   0,
			wantHeaderLen: 1,
			wantErr:       false,
		},
		{
			name:          "混合头部",
			input:         ":method: POST\nuser-agent: test\n:scheme: https",
			wantFakeLen:   2,
			wantHeaderLen: 1,
			wantErr:       false,
		},
		{
			name:          "重复头部",
			input:         ":method: GET\n:method: POST",
			wantFakeLen:   2,
			wantHeaderLen: 0,
			wantErr:       false,
		},
		{
			name:          "特殊字符",
			input:         ":method: GET\nX-Test: test:with:colons",
			wantFakeLen:   1,
			wantHeaderLen: 1,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, header, _, err := HTTPFrameParser(bytes.NewReader([]byte(tt.input)))

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantFakeLen, len(fake))
			require.Equal(t, tt.wantHeaderLen, len(header))
		})
	}
}
