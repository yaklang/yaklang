package aibp

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestXSS(t *testing.T) {
	yakit.CallPostInitDatabase()
	res, err := ExecuteForge("xss", `http://127.0.0.1:8787/xss/js/in-str?name=admin`)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(res)
}
