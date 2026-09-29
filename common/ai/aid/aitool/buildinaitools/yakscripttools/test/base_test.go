package test

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// These tests exercise the global lookup API against an isolated fixture, not
// whichever built-in tools happen to be installed in the developer's profile.
func setupHTTPYakToolFixture(t *testing.T) {
	t.Helper()
	db := createTestDB(t)
	previousDB := consts.GetGormProfileDatabase()
	previousPath := consts.GetCurrentProfileDatabasePath()
	consts.BindProfileDatabase(db, "")
	t.Cleanup(func() {
		consts.BindProfileDatabase(previousDB, previousPath)
		_ = db.Close()
	})
	content, err := os.ReadFile("../yakscriptforai/http/send_http_request_by_url.yak")
	assert.NilError(t, err)
	tool := yakscripttools.LoadYakScriptToAiTools("send_http_request_by_url", string(content))
	assert.Assert(t, tool != nil)
	tool.Path = "http/send_http_request_by_url"
	assert.NilError(t, db.Create(tool).Error)
}

func TestGetYakScript(t *testing.T) {
	setupHTTPYakToolFixture(t)
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tools := yakscripttools.GetAllYakScriptAiTools()
	hasDoHttp := false
	for _, ait := range tools {
		if ait.Name == "send_http_request_by_url" {
			hasDoHttp = true
			w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			_, err := ait.Callback(context.Background(), aitool.InvokeParams{
				"url": "http://" + host + ":" + strconv.Itoa(port),
			}, nil, w1, w2)
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(w1.String(), flag))
		}
	}
	assert.Assert(t, hasDoHttp)
}

func TestSearchYakScript(t *testing.T) {
	setupHTTPYakToolFixture(t)
	tools := yakscripttools.GetYakScriptAiTools("http")
	assert.Equal(t, len(tools), 1)
	assert.Equal(t, tools[0].Name, "send_http_request_by_url")
}

func TestSendHTTPRequestByURLSavesRequestContent(t *testing.T) {
	setupHTTPYakToolFixture(t)
	host, port := utils.DebugMockHTTP([]byte("ok"))
	for _, tool := range yakscripttools.GetAllYakScriptAiTools() {
		if tool.Name != "send_http_request_by_url" {
			continue
		}
		stdout := bytes.NewBuffer(nil)
		_, err := tool.Callback(context.Background(), aitool.InvokeParams{
			"url":          "http://" + host + ":" + strconv.Itoa(port) + "/request-check",
			"show-request": "yes",
		}, nil, stdout, bytes.NewBuffer(nil))
		assert.NilError(t, err)
		match := regexp.MustCompile(`req packet saved to (\S+)`).FindStringSubmatch(stdout.String())
		if len(match) != 2 {
			t.Fatalf("saved request path missing: %s", stdout.String())
		}
		t.Cleanup(func() { _ = os.Remove(match[1]) })
		data, err := os.ReadFile(match[1])
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(data), "GET /request-check HTTP/1.1"), "saved file should contain raw request, got %q", data)
		return
	}
	t.Fatal("send_http_request_by_url not found")
}
