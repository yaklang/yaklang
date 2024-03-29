package openai

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
func TestExtractDataByAi(t *testing.T) {
	consts.GetGormProjectDatabase()
	yakit.ConfigureNetWork(yakit.GetNetworkConfig())
	result, err := ExtractDataByAi(`老板，你听我狡辩，今天下雨，路上车堵得很，所以迟到了`, "这是一个提取文本内容的函数", map[string]string{
		"原因": "事件发生的原因",
		"结果": "事件造成的结果",
	})
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
	//(map[string]interface {}) (len=2) {
	//	(string) (len=6) "原因": (string) (len=21) "下雨，路上车堵",
	//		(string) (len=6) "结果": (string) (len=6) "迟到"
	//}
}

func TestClient_Session(t *testing.T) {
	s := NewSession(
		WithAPIKey(getKey()),
		WithProxy("http://127.0.0.1:7890"),
	)

	details, err := s.Chat(aispec.NewUserChatDetail("1+=1=?"))
	if err != nil {
		t.Fatal(err)
	}
	result := details.FirstString()
	spew.Dump(result)
	details, err = s.Chat(aispec.NewUserChatDetail("Repeat the first sentence assistant just replied"))
	if err != nil {
		t.Fatal(err)
	}
	result2 := details.FirstString()
	spew.Dump(result2)
	if result != result2 {
		t.Fail()
	}
}

func TestClient_ChatEx(t *testing.T) {
	// d = openai.ChatEx(
	// 	[
	// 	  openai.userMessage("What is the weather like in Boston?")
	// 	],
	// 	openai.newFunction(
	// 	  "get_current_weather",
	// 	  "Get the current weather in a given location",
	// 	  openai.functionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
	// 	  openai.functionRequired("location"),
	// 	  ),
	// 	openai.proxy("http://127.0.0.1:7890"),
	//   )~

	//   println(d.FunctionCallResult())

	result, err := chatEx(
		[]aispec.ChatDetail{
			aispec.NewUserChatDetail("What is the weather like in Boston?"),
		},
		WithFunction(
			"get_current_weather",
			"Get the current weather in a given location",
			WithFunctionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
			WithFunctionRequired("location"),
		),
		WithAPIKey(getKey()),
		WithProxy("http://127.0.0.1:7890"),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
