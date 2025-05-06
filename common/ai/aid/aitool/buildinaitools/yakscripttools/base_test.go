package yakscripttools

import (
	"bytes"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gotest.tools/v3/assert"
)

func TestGetYakScript(t *testing.T) {
	tools := GetAllYakScriptAiTools()
	for _, ait := range tools {
		spew.Dump(ait)
		if ait.Name == "do_http" {
			w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			ait.Callback(aitool.InvokeParams{
				"url": "http://www.example.com",
			}, w1, w2)
		}
	}
}

func TestSearchYakScript(t *testing.T) {
	tools := GetYakScriptAiTools("http")
	assert.Assert(t, len(tools) > 0)
}
