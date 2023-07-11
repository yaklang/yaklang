package yaklib

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"testing"
)

func TestNewOnlineClient(t *testing.T) {
	client := NewOnlineClient("http://www.yaklang.com")
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
