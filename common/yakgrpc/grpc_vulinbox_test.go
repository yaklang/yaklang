package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_InstallVulinbox(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	stream, err := client.InstallVulinbox(context.Background(), &ypb.InstallVulinboxRequest{Proxy: ""})
	if err != nil {
		panic(err)
	}

	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
}

func TestServer_StartVulinbox(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	stream, err := client.StartVulinbox(context.Background(), &ypb.StartVulinboxRequest{
		Host:     "127.0.0.1",
		Port:     "8787",
		NoHttps:  true,
		SafeMode: false,
	})
	if err != nil {
		panic(err)
	}

	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
}
