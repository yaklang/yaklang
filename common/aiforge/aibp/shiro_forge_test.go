package aibp

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestShiroForge(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	_, err := yak.ExecuteForge("shiro_detect", "http://127.0.0.1:8787/shiro/cbc", yak.WithAgreeYOLO())
	if err != nil {
		t.Fatal(err)
	}
}
