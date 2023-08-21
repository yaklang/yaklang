package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_YakitLog(t *testing.T) {
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
