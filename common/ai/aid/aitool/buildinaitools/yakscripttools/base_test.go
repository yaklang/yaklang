package yakscripttools

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"gotest.tools/v3/assert"
)

func TestGetYakScript(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tools := GetAllYakScriptAiTools()
	hasDoHttp := false
	for _, ait := range tools {
		if ait.Name == "do_http" {
			hasDoHttp = true
			w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			ait.Callback(aitool.InvokeParams{
				"url": "http://" + host + ":" + strconv.Itoa(port),
			}, w1, w2)
			assert.Assert(t, strings.Contains(w1.String(), flag))
		}
	}
	assert.Assert(t, hasDoHttp)
}

func TestSearchYakScript(t *testing.T) {
	tools := GetYakScriptAiTools("http")
	assert.Assert(t, len(tools) > 0)
}
