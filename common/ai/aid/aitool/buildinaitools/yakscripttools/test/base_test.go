package test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func TestGetYakScript(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tools := yakscripttools.GetAllYakScriptAiTools()
	hasDoHttp := false
	for _, ait := range tools {
		if ait.Name == "send_http_request_by_url" {
			hasDoHttp = true
			w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			ait.Callback(context.Background(), aitool.InvokeParams{
				"url": "http://" + host + ":" + strconv.Itoa(port),
			}, nil, w1, w2)
			assert.Assert(t, strings.Contains(w1.String(), flag))
		}
	}
	assert.Assert(t, hasDoHttp)
}

func TestSearchYakScript(t *testing.T) {
	tools := yakscripttools.GetYakScriptAiTools("http")
	assert.Assert(t, len(tools) > 0)
}
