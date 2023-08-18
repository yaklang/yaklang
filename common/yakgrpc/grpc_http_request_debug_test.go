package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
)

func TestServer_DebugPlugin_MITM(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	})
	var targetUrl = "http://" + utils.HostPort(host, port)

	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code: `mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	dump(url)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path: []string{"a?a=1", "b?b=1"},
		},
		Input: targetUrl,
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}
