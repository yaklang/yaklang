package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_HTTPRequestBuilder(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsRawHTTPRequest: true,
		RawHTTPRequest: []byte(`GET / HTTP/1.1
Host: baidu.com

`),
	})
	if err != nil {
		panic(err)
	}
	if !strings.Contains(rsp.Templates, `Host: {{Hostname}}`) {
		panic("raw packet build failed")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Path: []string{"/admin-123", "/.wp?c=123"},
		GetParams: []*ypb.KVPair{
			{Key: "aaa", Value: "ccc"},
		},
		PostParams: []*ypb.KVPair{
			{Key: "cc", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
			{Key: "c1c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
			{Key: "casdfa(*)(*()c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
		},
	})
	if err != nil {
		panic(err)
	}
	println(rsp.Templates)
	if !strings.Contains(rsp.Templates, `{{BaseURL}}/admin-123?aaa=ccc`) {
		panic("raw packet build failed")
	}
}
