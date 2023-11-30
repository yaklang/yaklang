package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func TestDownloadOnlinePluginBatch(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	_, err = client.DownloadOnlinePluginBatch(context.Background(), &ypb.DownloadOnlinePluginsRequest{})
	if err != nil {
		panic(err)
	}
}

func TestDownloadOnlinePlugins(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	res, err := client.DownloadOnlinePlugins(context.Background(), &ypb.DownloadOnlinePluginsRequest{})
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

func TestDownloadOnlinePluginByPluginName(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	_, err = client.DownloadOnlinePluginByPluginName(context.Background(), &ypb.DownloadOnlinePluginByScriptNamesRequest{
		ScriptNames: []string{"基础 XSS 检测"},
		Token:       "",
	})
	if err != nil {
		panic(err)
	}
}

func TestSaveYakScriptToOnline(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	_, err = client.SaveYakScriptToOnline(context.Background(), &ypb.SaveYakScriptToOnlineRequest{
		//ScriptNames: []string{"testlimin1113"},
		Token:     "74_nXiaH-Z-elUDSS2RnXACUlDZx-645BGzlU4-rkss3H-9Z-B8SVxvEf9Omv1MXO2tFcRPMHm_vsNP3aaq1xKIj8ks3A59-igjjb2VDrUzwpM",
		IsPrivate: false,
		All:       true,
	})
	if err != nil {
		panic(err)
	}
}
