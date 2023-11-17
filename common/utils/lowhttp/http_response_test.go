package lowhttp

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestFixHTTPResponse(t *testing.T) {
	type args struct {
		raw []byte
	}

	gzipData, err := utils.GzipCompress("你好")
	if err != nil {
		panic(err)
	}
	h2packet := `HTTP/2 200 Ok
Test: 111
Content-Encoding: gzip

` + string(gzipData)
	rawResp, _ := codec.DecodeBase64(`SFRUUC8xLjEgMjAwIE9LDQpEYXRlOiBTdW4sIDI2IEZlYiAyMDIzIDAzOjQ0OjUzIEdNVA0KU2VydmVyOiBBcGFjaGUNClgtUG93ZXJlZC1CeTogUEhQLzUuMi4xNw0KRXhwaXJlczogVGh1LCAxOSBOb3YgMTk4MSAwODo1MjowMCBHTVQNCkNhY2hlLUNvbnRyb2w6IG5vLXN0b3JlLCBuby1jYWNoZSwgbXVzdC1yZXZhbGlkYXRlLCBwb3N0LWNoZWNrPTAsIHByZS1jaGVjaz0wDQpQcmFnbWE6IG5vLWNhY2hlDQpTZXQtQ29va2llOiBQSFBTRVNTSUQ9dmFsdWU7IGh0dHBPbmx5DQpWYXJ5OiBVc2VyLUFnZW50LEFjY2VwdC1FbmNvZGluZw0KVHJhbnNmZXItRW5jb2Rpbmc6IGNodW5rZWQNCkNvbnRlbnQtVHlwZTogdGV4dC9odG1sDQoNCjFmDQrmj5DkuqTluKbmnInkuI3lkIjms5Xlj4LmlbANCjANCg0K`)

	jsonChineseOrigin := `HTTP/1.1 200 OK
Connection: keep-alive
Content-Encoding: identity
Content-Type: application/json; charset=utf-8
Date: Mon, 21 Nov 2022 07:54:18 GMT
Edocecart: 32581202071201156071176630638112115
Server: BWS
Traceid: 1669017258039822849010147292226863610359
Vary: Accept-Encoding
Content-Length: 1033

{
    "errno": 0,
    "error": "\u6210\u529f",
    "data": {
        "requestParam": [],
        "response": {
            "cnt": {
                "fansCnt": 182881,
                "fansCntText": "18\u4e07\u7c89\u4e1d",
                "videoCount": 11961,
                "videoCntText": "1.2\u4e07\u4e2a\u89c6\u9891",
                "totalPlaycnt": 0,
                "totalPlaycntText": ""
            },
            "author": {
                "vip": 1,
                "author": "\u6293\u9a6c\u77ed\u661f\u95fb",
                "author_icon": "https:\/\/gimg0.baidu.com\/gimg\/src=https%3A%2F%2Fgips0.baidu.com%2Fit%2Fu%3D2476551077%2C1418779011%26fm%3D3012%26app%3D3012%26autime%3D1667425861%26size%3Db200%2C200&refer=http%3A%2F%2Fwww.baidu.com&app=0&size=f68,68&n=0&g=0n&q=60?sec=0&t=d9946543ab888fe6d5d9c1fb9991428c&fmt=auto",
                "mthid": "1680416484715223",
                "authentication_content": "\u5a31\u4e50\u9886\u57df\u521b\u4f5c\u8005"
            },
            "is_subscribe": 0
        }
    }
}`

	rapInput := `HTTP/1.1 200 OK
Connection: close
Bdpagetype: 3
Bdqid: 0x9efbfb790011d570
Cache-Control: private
Ckpacknum: 2
Ckrndstr: 90011d570
Content-Encoding: gzip
Content-Type: text/html;charset=utf-8
Date: Sat, 27 Nov 2021 04:20:29 GMT
P3p: CP=" OTI DSP COR IVA OUR IND COM "
Server: BWS/1.1
Set-Cookie: BDRCVFR[S4-dAuiWMmn]=I67x6TjHwwYf0; path=/; domain=.baidu.com
Set-Cookie: delPer=0; path=/; domain=.baidu.com
Set-Cookie: BD_CK_SAM=1;path=/
Set-Cookie: PSINO=2; domain=.baidu.com; path=/
Set-Cookie: BDSVRTM=12; path=/
Set-Cookie: H_PS_PSSID=34445_35104_35239_34584_34517_35245_34606_35320_26350_35209_35312_35145; path=/; domain=.baidu.com
Strict-Transport-Security: max-age=172800
Traceid: 1637986829039139149811456026574257771888
Vary: Accept-Encoding
X-Frame-Options: sameorigin
X-Ua-Compatible: IE=Edge,chrome=1
Content-Length: 12

aaaaaaaaaaaa` + "\r\n\r\n"

	jsonEcho := `HTTP/1.1 200 OK
Date: Thu, 16 Nov 2023 08:42:33 GMT
Content-Type: application/json

{"key":"value"}`

	tests := []struct {
		name     string
		args     args
		wantRsp  func(t *testing.T, rsp []byte)
		wantBody func(t *testing.T, rsp []byte)
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name: "Handle Gzip Encoding in HTTP/2 Response",
			args: args{raw: []byte(h2packet)},
			wantRsp: func(t *testing.T, rsp []byte) {
				assert.NotContains(t, string(rsp), "gzip")
				assert.NotContains(t, string(rsp), "Content-Encoding")
			},
			wantBody: func(t *testing.T, body []byte) {
				assert.Equal(t, "你好", string(body))
			},
			wantErr: assert.NoError,
		},
		{
			name:    "Handle Base64 Encoded Response",
			args:    args{raw: rawResp},
			wantRsp: nil,
			wantBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "提交带有不合法参数", "响应体中应该包含文本 '提交带有不合法参数'")
			},
			wantErr: assert.NoError,
		},
		{
			name:    "Handle JSON Response with Chinese Characters",
			args:    args{raw: []byte(jsonChineseOrigin)},
			wantRsp: nil,
			wantBody: func(t *testing.T, body []byte) {
				body = []byte(codec.JsonUnicodeDecode(string(body)))
				assert.Contains(t, string(body), "抓马短星闻", "响应体应该包含 '抓马短星闻'")
			},
			wantErr: assert.NoError,
		},
		{
			name:    "Handle Gzip Encoding in HTTP/1.1 Response",
			args:    args{raw: []byte(rapInput)},
			wantRsp: nil,
			wantBody: func(t *testing.T, rsp []byte) {
				assert.Equal(t, "aaaaaaaaaaaa"+"\r\n\r\n", string(rsp), "响应内容应该与 'aaaaaaaaaaaa\\r\\n\\r\\n' 相匹配")
			},
			wantErr: assert.NoError,
		},
		{
			name:    "Handle json format response",
			args:    args{raw: []byte(jsonEcho)},
			wantRsp: nil,
			wantBody: func(t *testing.T, rsp []byte) {
				assert.Equal(t, `{"key":"value"}`, string(rsp), "响应内容应该与 '{\"key\":\"value\"}' 相匹配")
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRsp, gotBody, err := FixHTTPResponse(tt.args.raw)
			if !tt.wantErr(t, err, fmt.Sprintf("FixHTTPResponse(%v)", tt.args.raw)) {
				return
			}
			if tt.wantRsp != nil {
				tt.wantRsp(t, gotRsp)
			}
			if tt.wantBody != nil {
				tt.wantBody(t, gotBody)
			}
		})
	}
}
