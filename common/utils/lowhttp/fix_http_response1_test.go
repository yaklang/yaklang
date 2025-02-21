package lowhttp

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

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
