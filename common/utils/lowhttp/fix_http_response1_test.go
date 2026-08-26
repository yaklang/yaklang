package lowhttp

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestUnquoteFuzzTagSizeMatchesEncoding(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{name: "empty", body: nil, want: 15},
		{name: "ordinary printable ASCII is one byte", body: []byte("A"), want: 16},
		{name: "quote and slash are two bytes", body: []byte{'"', '\\'}, want: 19},
		{name: "delimiter printable ASCII is four bytes", body: []byte("(){}"), want: 31},
		{name: "non printable is four bytes", body: []byte{0x00, 0xff}, want: 23},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded := ToUnquoteFuzzTagForce(tc.body)
			require.Equal(t, tc.want, len(encoded))
			require.Equal(t, len(encoded), UnquoteFuzzTagSize(tc.body, true))
		})
	}

	utf8Body := []byte("你好")
	require.Equal(t, len(utf8Body), UnquoteFuzzTagSize(utf8Body, false))
	require.Equal(t, string(utf8Body), ToUnquoteFuzzTag(utf8Body))
}

func TestMeasureHTTPRequestFuzzTagBodySizeMatchesConversion(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantBinary  bool
	}{
		{name: "utf8 text remains raw", contentType: "text/plain", body: []byte("hello")},
		{name: "invalid utf8 text uses unquote", contentType: "text/plain", body: []byte{0xff}, wantBinary: true},
		{name: "binary MIME with valid UTF8 remains raw", contentType: "application/pdf", body: []byte("PDF")},
		{name: "octet stream with valid UTF8 remains raw", contentType: "application/octet-stream", body: []byte("readable bytes")},
		{name: "existing unquote representation remains stable", contentType: "application/pdf", body: []byte(`{{unquote("PDF")}}`)},
		{name: "existing file representation remains stable", contentType: "application/pdf", body: []byte(`{{file(/tmp/pdf)}}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet := []byte("POST / HTTP/1.1\r\nHost: example.test\r\nContent-Type: " + tc.contentType + "\r\n\r\n")
			packet = append(packet, tc.body...)
			measured, binary, err := MeasureHTTPRequestFuzzTagBodySize(packet)
			require.NoError(t, err)
			require.Equal(t, tc.wantBinary, binary)
			converted := ConvertHTTPRequestToFuzzTag(packet)
			_, convertedBody := SplitHTTPHeadersAndBodyFromPacket(converted)
			require.Equal(t, len(convertedBody), measured)
			if tc.wantBinary {
				require.True(t, bytes.HasPrefix(convertedBody, []byte(`{{unquote("`)))
			} else {
				require.Equal(t, tc.body, convertedBody)
			}
			require.Equal(t, converted, ConvertHTTPRequestToFuzzTag(converted), "conversion must be idempotent")
		})
	}
}

func TestConvertHTTPRequestToFuzzTag_MultipartUsesPerPartUTF8Validity(t *testing.T) {
	packet := []byte("POST /upload HTTP/1.1\r\n" +
		"Host: example.test\r\n" +
		"Content-Type: multipart/form-data; boundary=UTF8-BOUNDARY\r\n\r\n" +
		"--UTF8-BOUNDARY\r\n" +
		"Content-Disposition: form-data; name=\"valid\"; filename=\"readable.pdf\"\r\n" +
		"Content-Type: application/pdf\r\n\r\n" +
		"readable PDF bytes\r\n" +
		"--UTF8-BOUNDARY\r\n" +
		"Content-Disposition: form-data; name=\"invalid\"\r\n" +
		"Content-Type: text/plain\r\n\r\n")
	packet = append(packet, 0xff)
	packet = append(packet, []byte("\r\n--UTF8-BOUNDARY--\r\n")...)

	converted := ConvertHTTPRequestToFuzzTag(packet)
	convertedText := string(converted)
	require.Contains(t, convertedText, "readable PDF bytes")
	require.Contains(t, convertedText, `{{unquote("\xff")}}`)
	require.Equal(t, 1, strings.Count(convertedText, "{{unquote("), "only the invalid UTF-8 part is wrapped")

	measured, hasUnquote, err := MeasureHTTPRequestFuzzTagBodySize(packet)
	require.NoError(t, err)
	require.True(t, hasUnquote)
	_, convertedBody := SplitHTTPHeadersAndBodyFromPacket(converted)
	require.Equal(t, len(convertedBody), measured)
}

func TestConvertHTTPRequestToFuzzTag(t *testing.T) {
	req1 := `GET / HTTP/1.1
Host: www.baidu.com

` + "\xac\xedasdfasdfasdf\x00\u0000)))\u0000\u0000\u0000\u0000\u0000\u0000\x0100"

	req2 := `GET / HTTP/1.1
Host: www.baidu.com

` + "\xac\xedasdfasdfasdf\x44\x21\x00\u0000)))\"\"\u0000\u0000\u0000\u0000\u0000\u0000\x0100"

	multReq := `POST /post HTTP/1.1
Accept: */*
Accept-Encoding: gzip, deflate
Connection: keep-alive
Host: httpbin.org
User-Agent: HTTPie/3.2.1
Content-Type:multipart/form-data;boundary=----------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Length: 199

------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7
Content-Disposition: form-data; name="file"; filename="a.php"
Content-Type: image/png

` + "\xac\xed\x01\x00\xf1\xff" + `
------------Ef1KM7GI3Ef1ei4Ij5ae0KM7cH2KM7--`

	tests := []struct {
		name    string
		input   []byte
		wantRes func(t *testing.T, res []byte)
	}{
		{
			name:  "TestNewHTTPRequest",
			input: []byte(req1),
			wantRes: func(t *testing.T, res []byte) {
				if !strings.Contains(string(res), `{{unquote("\xac\xed`) {
					t.Errorf("TestNewHTTPRequest failed")
				}
			},
		},
		{
			name:  "TestNewHTTPRequest_2",
			input: []byte(req2),
			wantRes: func(t *testing.T, res []byte) {
				resStr := string(res)
				if !strings.Contains(resStr, `{{unquote("\xac\xed`) || !strings.Contains(resStr, "D") || !strings.Contains(resStr, "!") || !strings.Contains(resStr, `\"\"`) {
					t.Errorf("TestNewHTTPRequest_2 failed")
				}
			},
		},
		{
			name:  "TestConvertHTTPRequestToFuzzTag_Multipart",
			input: []byte(multReq),
			wantRes: func(t *testing.T, res []byte) {
				if !strings.Contains(string(res), `{{unquote("\xac\xed\x01\x00\xf1\xff")}}`) {
					t.Errorf("TestConvertHTTPRequestToFuzzTag_Multipart failed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ConvertHTTPRequestToFuzzTag(tt.input)
			tt.wantRes(t, res)
		})
	}
}

func TestCheckLowHttpAutoFixFlag(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Disposition: attachment; filename="example.pdf"

%PDF-1.4
%âãÏÓ
%%EOF
`))

	rsp, err := HTTP(WithHost(host), WithPort(port))
	require.NoError(t, err)
	require.True(t, rsp.IsFixContentType)
}
