package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise real TLS, SSE parsing and Yak output with a complete local response.
// Returning from the handler terminates the HTTP body; no idle-read timeout is needed.
func mockChatStream(t *testing.T, chunks ...string) string {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%s}}]}\n\n", utils.Jsonify(chunk))
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "https://")
}

func TestMitmInvokeAi(t *testing.T) {
	consts.ClearThirdPartyApplicationConfig()
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		APIKey: fmt.Sprintf("%s.%s", utils.RandStringBytes(32), utils.RandStringBytes(16)),
		Type:   "chatglm",
	})
	caller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	addr := mockChatStream(t, "我是人工智障")
	var mu sync.Mutex
	var msgs strings.Builder
	caller.SetFeedback(func(i *ypb.ExecResult) error {
		mu.Lock()
		defer mu.Unlock()
		msgs.Write(i.Message)
		return nil
	})
	require.NoError(t, caller.LoadHotPatch(context.Background(), []*ypb.ExecParamItem{}, `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
    res = ai.Chat("你好", ai.domain("`+addr+`"), ai.type("chatglm"))~
    yakit_output(res)
}
`))
	for i := 0; i < 10; i++ {
		caller.MirrorHTTPFlow(false, "aaa", []byte(""), []byte(""), []byte(""))
	}
	caller.Wait()
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 10, strings.Count(msgs.String(), "我是人工智障"))
}

func TestOUTPUT_AiChat(t *testing.T) {
	consts.ClearThirdPartyApplicationConfig()
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		APIKey: fmt.Sprintf("%s.%s", utils.RandStringBytes(32), utils.RandStringBytes(16)),
		Type:   "chatglm",
	})
	addr := mockChatStream(t, "你好", "我是人工智障", "助手")
	yaklib.InitYakit(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error { return nil }))
	re := regexp.MustCompile("[\u4e00-\u9fa5]")
	var stdout strings.Builder
	var mu sync.Mutex
	cancel, wait, err := utils.HandleStdoutBackgroundForTest(func(s string) {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range re.FindAllString(s, -1) {
			stdout.WriteString(c)
		}
	})
	require.NoError(t, err)
	engine := yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error { return nil }))
	err = engine.Execute(fmt.Sprintf(`result = ai.Chat("你好", ai.type("chatglm"), ai.debugStream(), ai.domain("%s"))~; assert result == "你好我是人工智障助手"`, addr))
	cancel()
	wait()
	require.NoError(t, err)
	mu.Lock()
	assert.Contains(t, stdout.String(), "你好我是人工智障助手")
	mu.Unlock()

	subMsgN := 0
	var msg strings.Builder
	engine = yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		for _, s := range re.FindAllString(string(i.Message), -1) {
			msg.WriteString(s)
		}
		subMsgN++
		return nil
	}))
	require.NoError(t, engine.Execute(fmt.Sprintf(`ai.Chat("你好", ai.type("chatglm"), ai.domain("%s"))~`, addr)))
	assert.GreaterOrEqual(t, subMsgN, 3)
	assert.Contains(t, msg.String(), "你好我是人工智障助手")
}

func TestOUTPUT_STREAMYakitStream(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	uid := uuid.New().String()

	stream, err := client.Exec(context.Background(), &ypb.ExecRequest{
		NoDividedEngine: true,
		Script: `yakit.AutoInitYakit()

# Input your code!
pr, pw = io.Pipe()~
go func{
    count = 0
    for {
        count++
        pw.Write("Hello1")
        if count > 5 {
            pw.Close()
            return
        }
    }
}
yakit.Stream("ai", "` + uid + `", pr)
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	var dataBuf bytes.Buffer
	haveStart := false
	haveStop := false
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}

		if data.IsMessage {
			data := string(data.Message)
			if data == "" {
				continue
			}
			data = codec.AnyToString(jsonpath.Find(data, "$.content.data"))
			id := jsonpath.Find(data, "$.streamId")
			if id != uid {
				t.Fatal("streamId is not right")
			}
			if codec.AnyToString(jsonpath.Find(data, "$.action")) == "start" {
				haveStart = true
			}
			if codec.AnyToString(jsonpath.Find(data, "$.action")) == "stop" {
				haveStop = true
			}
			if codec.AnyToString(jsonpath.Find(data, "$.action")) == "data" {
				dataBuf.WriteString(codec.AnyToString(jsonpath.Find(data, "$.data")))
			}
			spew.Dump(data)
		}
	}
	if !haveStart {
		t.Fatal("stream start not found")
	}
	if !haveStop {
		t.Fatal("stream stop not found")
	}
	if dataBuf.String() != "Hello1Hello1Hello1Hello1Hello1Hello1" {
		t.Fatal("stream data not found")
	}
}

func TestGRPCMUSTPASS_LANGUAGE_YakitLog(t *testing.T) {
	testCase1 := [][]string{
		{"yakit.Info(\"yakit_info\")", "yakit_info"},
		{"yakit.Info(\"yakit_%v\",\"info\")", "yakit_info"},
		{"risk.NewRisk(\"1.1.1.1\")", ""},
		{"yakit.Output(yakit.TableData(\"table\", {\n    \"id\": 1,\n    \"name\": \"张三\",\n}))", ""},
	}
	testCase2 := [][]string{
		{"println(x\"{{base64(Hello Yak)}}\")", "SGVsbG8gWWFr"},
		{"println(\"println\")", "println"},
		{"println(\"print\")", "print"},
		{"dump(\"dump\")", "dump"},
		{"log.info(\"log_info\")", "log_info"},
		{"log.infof(\"log_%s\",\"info\")", "log_info"},
	}
	code := ""
	for _, v := range testCase1 {
		code += v[0] + "\n"
	}
	for _, v := range testCase2 {
		code += v[0] + "\n"
	}

	client, err := NewLocalClient()
	stream, err := client.Exec(context.Background(), &ypb.ExecRequest{
		Script:          code,
		NoDividedEngine: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	i := 0
	otherLog := ""
	for {
		res, err := stream.Recv()
		if err != nil {
			break
		}
		info := make(map[string]interface{})
		err = json.Unmarshal(res.Message, &info)
		if err != nil {
			otherLog += string(res.Raw)
		}
		if i >= len(testCase1) {
			break
		}
		if info["type"] == "log" {
			if v, ok := info["content"].(map[string]interface{}); ok {
				if !strings.Contains(utils.InterfaceToString(v["data"]), testCase1[i][1]) {
					t.Fatal("log error")
				}
			} else {
				t.Fatal("invalid log format")
			}
		}
		i++
	}
	_ = otherLog
	// 由于CombinedOutput是异步的，可能由于延迟导致这里没有获取到全部输出
	//for _, testCase := range testCase2 {
	//	if !strings.Contains(otherLog, testCase[1]) {
	//		t.Fatal("log stream not contains", testCase[1])
	//	}
	//}
}

func TestGRPCMUSTPASS_ScriptPath(t *testing.T) {
	client, err := NewLocalClient()
	expectedMessage := "Hello Yak"
	filename, err := utils.SaveTempFile(fmt.Sprintf(`println("%s")`, expectedMessage), "temp-yak-scriptPath")
	require.NoError(t, err)
	stream, err := client.Exec(context.Background(), &ypb.ExecRequest{
		ScriptPath:      filename,
		NoDividedEngine: true,
	})

	for {
		res, err := stream.Recv()
		if err != nil {
			break
		}
		info := make(map[string]interface{})
		err = json.Unmarshal(res.Message, &info)
		if info["type"] == "log" {
			if v, ok := info["content"].(map[string]interface{}); ok {
				require.Contains(t, utils.InterfaceToString(v["data"]), expectedMessage)
			} else {
				t.Fatal("invalid log format")
			}
		}
	}
}

func TestGRPCMUSTPASS_NotExistScriptPath(t *testing.T) {
	client, err := NewLocalClient()
	expectedMessage := "Hello Yak"
	token := utils.RandStringBytes(16)
	stream, err := client.Exec(context.Background(), &ypb.ExecRequest{
		ScriptPath:      token + ".yak", // not exist path
		Script:          fmt.Sprintf(`println("%s")`, expectedMessage),
		NoDividedEngine: true,
	})
	require.NoError(t, err)

	for {
		res, err := stream.Recv()
		if err != nil {
			break
		}
		info := make(map[string]interface{})
		err = json.Unmarshal(res.Message, &info)
		if info["type"] == "log" {
			if v, ok := info["content"].(map[string]interface{}); ok {
				require.Contains(t, utils.InterfaceToString(v["data"]), expectedMessage)
			} else {
				t.Fatal("invalid log format")
			}
		}
	}
}
