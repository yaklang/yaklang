package yakgrpc

import (
	"context"
	"yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_DownloadOnlinePluginAll(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	res, err := client.DownloadOnlinePluginAll(context.Background(), &ypb.DownloadOnlinePluginByTokenRequest{})
	if err != nil {
		panic(err)
	}

	for {
		r, err := res.Recv()
		if err != nil {
			panic(err)
			return
		}
		println(r.Progress)
	}
}
