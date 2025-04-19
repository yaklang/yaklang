package yakscripttools

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"testing"
)

func TestGetYakScript(t *testing.T) {
	tools := GetYakScriptAiTools()
	for _, ait := range tools {
		spew.Dump(ait)
		if ait.Name == "http_get" {
			w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			ait.Callback(aitool.InvokeParams{
				"url": "http://www.example.com",
			}, w1, w2)
		}
	}
}
