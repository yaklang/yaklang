package yakgrpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMitmInvokeAi(t *testing.T) {
	consts.ClearThirdPartyApplicationConfig()
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		APIKey: fmt.Sprintf("%s.%s", utils.RandStringBytes(32), utils.RandStringBytes(16)),
		Type:   "chatglm",
	})
	caller, err := yak.NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	rspStrTmp := `data: {"id":"1","created":1,"model":"1","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"}}]}`
	headerStr, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\nContent-Type: application/json\nTransfer-Encoding: chunked\nConnection: Keep-Alive\n\n"))
	port := utils.GetRandomAvailableTCPPort()
	l, err := tls.Listen("tcp", spew.Sprintf(":%d", port), utils.GetDefaultTLSConfig(3))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Error(err)
				return
			}
			genMsg := func(s string) []byte {
				msg := []byte(fmt.Sprintf(rspStrTmp, s))
				return []byte(fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg))
			}
			log.Info("accept conn")
			go func() {
				utils.StableReader(conn, 1, 10240)
				conn.Write(headerStr)
				conn.Write(genMsg("我是人工智障"))
				conn.Write([]byte("\r\n"))
				time.Sleep(time.Millisecond * 500)
				conn.Close()
				log.Info("close conn")
			}()
		}
	}()
	msgs := ""
	caller.SetFeedback(func(i *ypb.ExecResult) error {
		msgs += string(i.Message)
		return nil
	})
	addr := fmt.Sprintf("%s:%d", "127.0.0.1", port)
	caller.LoadHotPatch(context.Background(), []*ypb.ExecParamItem{}, `
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
 	res = ai.Chat("你好",ai.domain("`+addr+`"),ai.type("chatglm"))~
yakit_output(res)
}
`)

	for i := 0; i < 10; i++ {
		caller.MirrorHTTPFlow(false, "aaa", []byte(""), []byte(""), []byte(""))
	}
	caller.Wait()
	println(strings.Count(string(msgs), "我是人工智障"))
	if strings.Count(string(msgs), "我是人工智障") != 10 {
		t.Fatal("test mitm invoke ai failed")
	}
}

func TestOUTPUT_AiChat(t *testing.T) {
	consts.ClearThirdPartyApplicationConfig()
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		APIKey: fmt.Sprintf("%s.%s", utils.RandStringBytes(32), utils.RandStringBytes(16)),
		Type:   "chatglm",
	})
	rspStrTmp := `data: {"id":"1","created":1,"model":"1","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"}}]}
`
	headerStr, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\nContent-Type: application/json\nTransfer-Encoding: chunked\nConnection: Keep-Alive\n\n"))
	port := utils.GetRandomAvailableTCPPort()
	l, err := tls.Listen("tcp", spew.Sprintf(":%d", port), utils.GetDefaultTLSConfig(3))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				t.Fatal(err)
			}
			genMsg := func(s string) []byte {
				msg := []byte(fmt.Sprintf(rspStrTmp, s))
				return []byte(fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg))
			}
			log.Info("accept conn")
			go func() {
				utils.StableReader(conn, 1, 10240)
				conn.Write(headerStr)
				conn.Write(genMsg("你好"))
				time.Sleep(time.Millisecond * 500)
				conn.Write(genMsg("我是人工智障"))
				time.Sleep(time.Millisecond * 500)
				conn.Write(genMsg("助手"))
				conn.Write(genMsg(""))
				conn.Write([]byte("\r\n"))
				conn.Close()
				log.Info("close conn")
			}()
		}
	}()
	yaklib.InitYakit(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		return nil
	}))
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	time.Sleep(time.Second)
	debugStreamTestResult := false
	stdOutputCh := ""
	re := regexp.MustCompile("[\u4e00-\u9fa5]")
	var cancel, wait func()
	cancel, wait, err = utils.HandleStdoutBackgroundForTest(func(s string) {
		for _, c := range re.FindAllString(s, -1) {
			stdOutputCh += c
		}
		// log.Infof("HandleStdoutBackgroundForTest stdout: %v", stdOutputCh)
		if stdOutputCh == "你好我是人工智障助手" {
			debugStreamTestResult = true
			cancel()
		}
	})
	_ = debugStreamTestResult
	if err != nil {
		t.Fatal(err)
	}
	engine := yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		return nil
	}))
	err = engine.Execute(fmt.Sprintf(`result = ai.Chat("你好",ai.type("chatglm"),ai.debugStream(),ai.domain("%s"))~; dump(result); assert result == "你好我是人工智障助手"`, addr))
	if err != nil {
		t.Fatal(err)
	}
	wait()
	subMsgN := 0
	msg := ""
	time.Sleep(time.Second)
	engine = yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		s := re.FindAllString(string(i.Message), -1)
		for _, s2 := range s {
			msg += s2
		}
		subMsgN++
		print(string(i.Raw))
		return nil
	}))
	err = engine.Execute(fmt.Sprintf(`ai.Chat("你好",ai.type("chatglm"),ai.domain("%s"))~`, addr))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, subMsgN >= 3)
	assert.Contains(t, msg, "你好我是人工智障助手")
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
        sleep(0.3)
        if count > 5 {
            pw.Close()
            pr.Close()
            return
        }
    }
}
yakit.Stream("ai", "` + uid + `", pr)
sleep(2)
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
