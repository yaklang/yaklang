package openai

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
)

func getKey() string {
	raw, _ := ioutil.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
	return strings.TrimSpace(string(raw))
}

func TestClient_Chat(t *testing.T) {
	rsp, err := NewOpenAIClient(
		WithProxy("http://127.0.0.1:7890"),
		WithAPIKey(getKey()),
	).Chat("Hello")
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
}

func TestClient_FunctionCall(t *testing.T) {
	result := functionCall("What is the weather like in Boston?", "get_current_weather", "Get the current weather in a given location",
		WithAPIKey(getKey()),
		WithProxy("http://127.0.0.1:7890"),
		WithFunctionParameterType("object"),
		WithFunctionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
		WithFunctionRequired("location"),
	)
	if len(result) == 0 {
		t.Fail()
	}
	spew.Dump(result)
}
