package yakgrpc

import (
	"context"
	"testing"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_FixUploadPacket(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.FixUploadPacket(context.Background(), &ypb.FixUploadPacketRequest{
		Request: []byte(`POST /Pass-16/index.php HTTP/1.1
Host: 123.58.236.76:14700
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: identity
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Length: 26981
Content-Type: multipart/form-data; boundary=----WebKitFormBoundarySwLsJXBjLJWfghXP
Origin: http://123.58.236.76:14700
Referer: http://123.58.236.76:14700/Pass-16/index.php
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/102.0.5005.61 Safari/537.36

------WebKitFormBoundarySwLsJXBjLJWfghXP
Content-Disposition: form-data; name="upload_file"; filename="1_create_topic.png"
Content-Type: image/png

�PNG
�B
------WebKitFormBoundarySwLsJXBjLJWfghXP
Content-Disposition: form-data; name="submit"

上传
------WebKitFormBoundarySwLsJXBjLJWfghXP--
`),
	})
	if err != nil {
		panic(err)
	}
	println(string(rsp.GetRequest()))
}

func TestServer_FixUploadPacket_2(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.FixUploadPacket(context.Background(), &ypb.FixUploadPacketRequest{
		Request: []byte(`POST /Pass-16/index.php HTTP/1.1
Host: 123.58.236.76:14700
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: identity
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Length: 26981
Content-Type: multipart/form-data; boundary=----WebKitFormBoundarySwLsJXBjLJWfghXP
Origin: http://123.58.236.76:14700
Referer: http://123.58.236.76:14700/Pass-16/index.php
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/102.0.5005.61 Safari/537.36

------WebKitFormBoundarySwLsJXBjLJWfghXP
Content-Disposition: form-data; name="upload_file"; filename="1_create_topic.png"
------WebKitFormBoundarySwLsJXBjLJWfghXP
Content-Disposition: form-data; name="submit"

上传
------WebKitFormBoundarySwLsJXBjLJWfghXP--
`),
	})
	if err != nil {
		panic(err)
	}
	println(string(rsp.GetRequest()))
}
