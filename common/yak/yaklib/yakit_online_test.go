package yaklib

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestNewOnlineClient(t *testing.T) {
	client := NewOnlineClient("https://www.yaklang.com")
	//for result := range client.DownloadYakitPluginAll(context.Background()).Chan {
	//	spew.Dump(result)
	//}

	stream := client.DownloadYakitPluginAll(context.Background())
	for result := range stream.Chan {
		_ = result
		//spew.Dump(result)
	}

	plugin, err := client.DownloadYakitPluginById("", "91bdb83b-ddad-4828-b408-b0c9d0f8c13b")
	if err != nil {
		panic(err)
	}
	spew.Dump(plugin)

	client.Save(consts.GetGormProfileDatabase(), plugin)
}

func TestDownloadOnlinePlugins(t *testing.T) {
	client := NewOnlineClient("https://www.yaklang.com")

	stream := client.DownloadOnlinePluginsBatch(context.Background(), "", []bool{}, "", []string{}, []string{}, "", 0, "", []string{}, "mine", []int64{}, []string{}, []string{}, nil)
	for result := range stream.Chan {
		client.Save(consts.GetGormProfileDatabase(), result.Plugin)
	}

	plugins := client.DownloadOnlinePluginByPluginName(context.Background(), "", []string{})
	for ret := range plugins.Chan {
		client.Save(consts.GetGormProfileDatabase(), ret.Plugin)
	}

}

func TestQueryOnlinePlugins(t *testing.T) {
	client := NewOnlineClient(consts.GetOnlineBaseUrl())
	req := &ypb.QueryOnlinePluginsRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   1,
			OrderBy: "updated_at",
			Order:   "desc",
		},
		Data: &ypb.DownloadOnlinePluginsRequest{},
	}
	rsp, _, err := client.QueryPlugins(req)
	assert.Nil(t, err)
	assert.NotNil(t, rsp)
	assert.Len(t, rsp, 1)
}
