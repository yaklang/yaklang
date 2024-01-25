package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
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
	count, err := yakit.CountRiskByRuntimeId(consts.GetGormProfileDatabase(), runtimeID)
	if err != nil {
		panic(err)
	}
	if count != 1 {
		t.Fatal("count not match")
	}
}
