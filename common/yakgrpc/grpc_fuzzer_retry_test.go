package yakgrpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_Retry(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(16)
	count := uint64(0)
	targetHost, targetPort := utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		defer conn.Close()
		currentCount := atomic.AddUint64(&count, 1)
		_, err := conn.Read(make([]byte, 1))
		if err != nil || currentCount%2 == 0 {
			return
		}
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nHello"))
	})

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "token",
				Value: token,
			},
		},
		Request: `GET /?token={{params(token)}}c={{int(1-5)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	rspCount := 0
	rspSuccessCount := 0
	var taskID int64 = 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		rspCount++
		if rsp.Ok {
			rspSuccessCount++
		}
		taskID = rsp.GetTaskId()
	}
	if taskID == 0 {
		t.Fatal("TaskID not found in response")
	}
	if rspCount != 5 {
		t.Fatalf("want 5 responses, but got %d", rspCount)
	}
	if rspSuccessCount < 2 || rspSuccessCount > 3 {
		t.Fatalf("want 2-3 success responses, but got %d", rspSuccessCount)
	}

	// 优化：只测试一次重试，简化测试逻辑
	// 检查3秒内 taskRespCount 是否为5，每100ms检查一次
	var (
		taskRespCount int
		found         bool
	)
	startTime := time.Now()
	for time.Since(startTime) < 3*time.Second {
		taskRespCount, err = yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(taskID))
		if err != nil {
			t.Fatal(err)
		}
		if taskRespCount == 5 {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatalf("want 5 task resp within 3 seconds, but got %d", taskRespCount)
	}

	// 执行一次重试测试
	client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RetryTaskID: taskID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rspCount, rspSuccessCount = 0, 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if len(rsp.RequestRaw) > 0 {
			rspCount++
		}
		if rsp.Ok {
			rspSuccessCount++
		}
		taskID = rsp.TaskId
	}

	if rspCount != 5 {
		t.Fatalf("want 5 responses, but got %d", rspCount)
	}
	// 重试后应该有一些成功的响应
	if rspSuccessCount == 0 {
		t.Fatalf("retry should have some success responses, got %d", rspSuccessCount)
	}

	taskRsp, err := c.QueryHistoryHTTPFuzzerTaskEx(context.Background(), &ypb.QueryHistoryHTTPFuzzerTaskExParams{
		Keyword: token,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if taskRsp.Total != 1 {
		t.Fatalf("want 1 task, but got %d", taskRsp.Total)
	}
	taskData := taskRsp.Data[0]
	if taskData.BasicInfo.HTTPFlowTotal != 5 && taskData.BasicInfo.HTTPFlowSuccessCount != 5 {
		t.Fatalf("task check failed: %#v", taskData.BasicInfo)
	}
}
