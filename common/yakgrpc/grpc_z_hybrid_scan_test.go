package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_HybridScan(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			Input: "http://www.example.com",
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{"基础 XSS 检测"},
		},
	})
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		spew.Dump(rsp)
	}
}

func TestGRPCMUSTPASS_HybridScan_status(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\nHello, World!"))
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			Input: "http://" + utils.HostPort(host, port) + "?a=c",
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{"基础 XSS 检测"},
		},
	})
	var total, finish int
	var taskID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		total = int(rsp.TotalTasks)
		finish = int(rsp.FinishedTasks)
		taskID = rsp.HybridScanTaskId
	}

	statusStream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	statusStream.Send(&ypb.HybridScanRequest{
		Control:        true,
		ResumeTaskId:   taskID,
		HybridScanMode: "status",
	})
	for {
		rsp, err := statusStream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		if total != int(rsp.TotalTasks) || finish != int(rsp.FinishedTasks) {
			t.Fatal("status not match")
		}
	}

}

func TestGRPCMUSTPASS_HybridScan_new(t *testing.T) {
	addr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			Input: addr + "/xss/echo?name=admin",
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{"基础 XSS 检测"},
		},
	})
	var runtimeID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		runtimeID = rsp.HybridScanTaskId
		spew.Dump(rsp)
	}
	count, err := yakit.CountRiskByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
	if err != nil {
		panic(err)
	}
	if count != 1 {
		t.Fatal("count not match")
	}
}

func TestGRPCMUSTPASS_HybridScan_HTTPFlow_At_Least(t *testing.T) {
	scriptName, err := yakit.CreateTemporaryYakScript("mitm", "")
	if err != nil {
		panic(err)
	}
	target := utils.HostPort(utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\nHello, World!")))
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			Input: target,
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{scriptName},
		},
	})
	var runtimeID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		runtimeID = rsp.HybridScanTaskId
		spew.Dump(rsp)
	}
	var count int
	consts.GetGormProjectDatabase().Model(&yakit.HTTPFlow{}).Where("runtime_id = ?", runtimeID).Count(&count)
	if count < 1 {
		t.Fatal("count not match")
	}
	spew.Dump(count)
}

var StopTestCode = `
mirrorHTTPFlow = func(isHttps , url , req , rsp , body) { 
    for { // 死循环,每秒发一次请求
        poc.Get("http://%s",poc.timeout(1))
		yakit.Info("test information")
        sleep(1)
    }
}`

// ! remove because of unstable
// func TestGRPCMUSTPASS_HybridScan_Stop_Smoking(t *testing.T) {
// 	scriptNameList := make([]string, 10)
// 	defer func() {
// 		for _, name := range scriptNameList { // 清理临时插件
// 			yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name)
// 		}
// 	}()

// 	var check = true
// 	var sendStop = false
// 	target := utils.HostPort(utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if sendStop {
// 			check = false // 如果已经向前端发送停止信号,还有对mock服务的请求,则停止失败
// 		}
// 		w.Write([]byte("Hello, World!"))
// 	}))

// 	for i := 0; i < 10; i++ {
// 		scriptName, err := yakit.CreateTemporaryYakScript("mitm", fmt.Sprintf(StopTestCode, target))
// 		if err != nil {
// 			panic(err)
// 		}
// 		scriptNameList = append(scriptNameList, scriptName)
// 	}

// 	client, err := NewLocalClient()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	streamContext, streamCancel := context.WithCancel(context.Background())
// 	defer streamCancel()
// 	stream, err := client.HybridScan(streamContext)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	stream.Send(&ypb.HybridScanRequest{
// 		Control:        true,
// 		HybridScanMode: "new",
// 	})

// 	targetList := []string{}
// 	for i := 0; i < 5; i++ { // 构造 5 个目标
// 		targetList = append(targetList, "http://"+target+"/"+utils.RandStringBytes(3))
// 	}

// 	stream.Send(&ypb.HybridScanRequest{
// 		Targets: &ypb.HybridScanInputTarget{
// 			Input: strings.Join(targetList, ","),
// 		},
// 		Plugin: &ypb.HybridScanPluginConfig{
// 			PluginNames: scriptNameList,
// 		},
// 	})

// 	streamReturnCheck := false
// 	for {
// 		rsp, err := stream.Recv()
// 		if err != nil {
// 			if strings.Contains(err.Error(), "task manager cancled") { // 如果返回的错误不是 task manager cancled 则代表在2秒内未成功返回停止信号给client
// 				streamReturnCheck = true
// 			}
// 			break
// 		}
// 		if rsp.ExecResult != nil && rsp.ExecResult.IsMessage {
// 			if bytes.Contains(rsp.ExecResult.Message, []byte("test information")) {
// 				stream.Send(&ypb.HybridScanRequest{
// 					Control:        false,
// 					HybridScanMode: "stop",
// 				})
// 				go func() {
// 					time.Sleep(2 * time.Second) //等待 2 秒后手动关闭连接
// 					streamCancel()
// 				}()
// 			}
// 		}
// 		spew.Dump(rsp)
// 	}
// 	if !streamReturnCheck {
// 		t.Fatal("return front fail")
// 	}
// 	sendStop = true             // 已经向前端发送停止信号,检查是否成功停止
// 	time.Sleep(4 * time.Second) // 等待 4 秒,是否还有请求mock服务
// 	if !check {
// 		t.Fatal("stop hybridScan fail")
// 	}

// }

func TestGRPCMUSTPASS_HybridScan_HttpflowID(t *testing.T) {
	token := utils.RandSecret(10)
	scriptName, err := yakit.CreateTemporaryYakScript("mitm", fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps , url , req , rsp , body) { 
	dump(req)
	if str.Contains(string(req),"%s"){
    yakit.Output("ok")
	}
}
`, token))
	if err != nil {
		panic(err)
	}
	defer yakit.DeleteYakScriptByName(consts.GetGormProjectDatabase(), scriptName)
	target := utils.HostPort(utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\nHello, World!")))

	packet := fmt.Sprintf("POST /\r\nHost: %s\r\n\r\n"+
		"%s", target, token)
	for i := 0; i < 3; i++ {
		lowhttp.HTTPWithoutRedirect(lowhttp.WithRequest(packet))
	}
	_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		Keyword: token,
	})
	if err != nil {
		t.Fatal(err)
	}

	ids := []int64{}
	for _, flow := range flows {
		ids = append(ids, int64(flow.ID))
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
				IsHttpFlowId: true,
				HTTPFlowId:   ids,
			},
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{scriptName},
		},
	})
	var checkCount int
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		if rsp.ExecResult != nil && rsp.ExecResult.IsMessage {
			if bytes.Contains(rsp.ExecResult.Message, []byte("ok")) {
				checkCount++
			}
		}
		spew.Dump(rsp)
	}
	spew.Dump(checkCount)
	if checkCount != 3 {
		t.Fatal("count not match")
	}

}
