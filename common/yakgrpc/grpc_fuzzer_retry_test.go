package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net"
	"sync/atomic"
	"testing"
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
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nHello\r\n"))
	})

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "token",
				Value: token,
			},
		},
		Request: `GET /?token={{params(token)}}c={{int(1-10)}} HTTP/1.1
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
	if rspCount != 10 {
		t.Fatalf("want 10 responses, but got %d", rspCount)
	}
	if rspSuccessCount != 5 {
		t.Fatalf("want 5 success responses, but got %d", rspSuccessCount)
	}

	retryTestCases := []int{7, 9, 9, 10}
	needTaskRespCount := []int{10, 5, 3, 1}
	for i, wantSuccessCount := range retryTestCases {
		require.NoError(t, utils.AttemptWithDelayFast(func() error {
			taskRespCount, err := yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(taskID))
			if err != nil {
				return err
			}
			if taskRespCount != needTaskRespCount[i] {
				return utils.Errorf("want 10 task resp ,but got %d", taskRespCount)
			}
			return nil
		}))
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

		if rspCount != 10 {
			t.Fatalf("[retry %d] want 10 responses, but got %d", i+1, rspCount)
		}
		if rspSuccessCount != wantSuccessCount {
			t.Fatalf("[retry %d] want %d success responses, but got %d", i+1, wantSuccessCount, rspSuccessCount)
		}
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
	if taskData.BasicInfo.HTTPFlowTotal != 10 && taskData.BasicInfo.HTTPFlowSuccessCount != 10 {
		t.Fatalf("task check failed: %#v", taskData.BasicInfo)
	}
}
