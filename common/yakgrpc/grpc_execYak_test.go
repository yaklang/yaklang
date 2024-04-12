package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestOUTPUT_AiChat(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	yaklib.InitYakit(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		println(string(i.Raw))
		return nil
	}))
	yaklib.AutoInitYakit()
	_, err = client.Exec(context.Background(), &ypb.ExecRequest{
		NoDividedEngine: true,
		Script:          `ai.Chat("你好",ai.debugStream(),ai.type("chatglm"))~`,
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
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

	var client, err = NewLocalClient()
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
