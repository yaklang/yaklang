package yakgrpc

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_GenerateYakCodeByPacketFixEOFError(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	rsp, err := client.GenerateCSRFPocByPacket(context.Background(), &ypb.GenerateCSRFPocByPacketRequest{
		Request: []byte(`POST / HTTP/1.1
Content-Type: multipart/form-data; boundary=63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Host: www.example.com
Content-Length: 183

--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618
Content-Disposition: form-data; name="key"

value
--63dee9b440dfdc85aab452b088e80a7484ef13a44fc4a4fba0b9affe8618--`),
	})
	require.NoError(t, err)
	require.Contains(t, string(rsp.Code), `<input type="hidden" name="key" value="value"/>`)
}

func TestServer_GenerateYakCodeByPacket(t *testing.T) {
	result := extractPacketToGenerateParams(true, []byte(`GET /_sockets/u/13946521/ws?session=eyJ2IjoiVjMiLCJ1IjoxMzk0NjUyMSwicyI6Nzg3ODA2Mjc3LCJjIjoyNTE4NjU0OTYzLCJ0IjoxNjUxODkzNjczfQ%3D%3D--18c938b8dfe75b4563893d59c29dd7379ce53a7cdb6972f83cb7e35e4d70e77d&shared=true&p=1520115733_1651762913.437 HTTP/1.1
Host: baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cache-Contr`+"`"+``+"`"+`ol: no-cache
Cookie: 111
Origin: https://github.com
Pragma: n`+"`"+``+"`"+`-cache
Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
Sec-WebSocket-Key: M8o6+1oL5LWWpF2K3DTuAw==
Sec-WebSocket-Version: 13
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.109 Safari/537.36

1231231`+"`"+`123123`))
	// spew.Dump(result)

	var buf bytes.Buffer
	err := OrdinaryPoCTemplate.Execute(&buf, result)
	if err != nil {
		panic(err)
	}
	println(buf.String())
}

func TestServer_GenerateYakCodeByPacket_Multipart(t *testing.T) {
	result := extractPacketToGenerateParams(true, []byte(`POST /CuteNews/index.php HTTP/1.1
Host: 10.129.106.34
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Length: 29408
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryL9EjCsrvV7xykqHB
Cookie: CUTENEWS_SESSION=crtcb2v2seae10jh1g0pon4gm7; _dd_s=logs=1&id=476d8350-b3dd-454c-a3ee-9c308557c1fc&created=1651983843241&expire=1651985088916
Origin: http://10.129.106.34
Referer: http://10.129.106.34/CuteNews/index.php
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.109 Safari/537.36

------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="mod"

main
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="opt"

personal
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="__signature_key"

bd7d045ecc29c6f5dc0a18eb79e9625f-abc
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="__signature_dsi"

a8e511e678820834e925ace21cdc2f39
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="editpassword"


------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="confirmpassword"


------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="editnickname"

abc
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="avatar_file"; filename="monkey0.jpg"
Content-Type: image/jpeg

����
------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="more[site]"


------WebKitFormBoundaryL9EjCsrvV7xykqHB
Content-Disposition: form-data; name="more[about]"


------WebKitFormBoundaryL9EjCsrvV7xykqHB--

`))
	// spew.Dump(result)

	var buf bytes.Buffer
	err := OrdinaryPoCTemplate.Execute(&buf, result)
	if err != nil {
		panic(err)
	}
	println(buf.String())
}
