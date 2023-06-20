package coreplugin

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_VULTEST(t *testing.T) {
	var client, err = NewLocalClient()
	if err != nil {
		t.Fatalf("start mitm local client failed: %s", err)
	}
	OverWriteCorePluginToLocal()

	var vulinboxPort = utils.GetRandomAvailableTCPPort()
	v, err := vulinbox.NewVulinServerEx(context.Background(), false, "127.0.0.1", vulinboxPort)
	if err != nil {
		t.Fatalf("start vulinbox server failed: %s", err)
	}
	time.Sleep(time.Second)
	t.Logf("vulinbox server started: %s", v)

	stream, err := client.MITM(context.Background())
	if err != nil {
		t.Fatalf("start mitm stream failed: %s", err)
	}
	var port = utils.GetRandomAvailableTCPPort()
	err = stream.Send(&ypb.MITMRequest{
		Host:           "0.0.0.0",
		Port:           uint32(port),
		Recover:        true,
		SetAutoForward: true,
		InitPluginNames: []string{
			"",
		},
	})
	if err != nil {
		t.Fatalf("send mitm request failed: %s", err)
	}

	go func() {
		c, err := crawler.NewCrawler(
			v,
			crawler.WithDomainWhiteList("127.0.0.1*"),
			crawler.WithProxy("http://127.0.0.1:"+fmt.Sprint(port)),
		)
		if err != nil {
			t.Fatalf("create basic crawler failed: %s", err)
		}
		err = c.Run()
		if err != nil {
			panic(1)
		}
	}()

	for {
		var rsp, err = stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("recv mitm response failed: %s", err)
		}
		spew.Dump(rsp)
	}
}
