package yakgrpc

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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

func TestGRPCMUSTPASS_HTTPFuzzer_MatcherActionFailRetry(t *testing.T) {
	c, err := NewLocalClient()
	require.NoError(t, err)

	retryToken := "retry-" + uuid.NewString()[:8]
	okToken := "ok-" + uuid.NewString()[:8]
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		index, _ := strconv.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
		body := okToken
		if index%2 == 1 {
			body = retryToken
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	})

	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Request:   "GET /?a={{i(0-5)}} HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "and",
				Group:       []string{retryToken},
				ExprType:    "nuclei-dsl",
				Action:      Action_Fail,
				HitColor:    "orange",
			},
		},
	})
	require.NoError(t, err)

	var taskID int64
	totalCount := 0
	failedCount := 0
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		totalCount++
		taskID = rsp.GetTaskId()
		if strings.Contains(string(rsp.ResponseRaw), retryToken) {
			require.False(t, rsp.Ok)
			require.Equal(t, matcherActionFailReason, rsp.Reason)
			require.True(t, rsp.MatchedByMatcher)
			require.Equal(t, "orange", rsp.HitColor)
			failedCount++
		} else {
			require.True(t, rsp.Ok)
		}
	}
	require.Equal(t, 6, totalCount)
	require.Equal(t, 3, failedCount)
	require.NotZero(t, taskID)

	require.NoError(t, utils.AttemptWithDelay(10, 200*time.Millisecond, func() error {
		responseRsp, err := c.QueryHTTPFuzzerResponseByTaskId(context.Background(), &ypb.QueryHTTPFuzzerResponseByTaskIdRequest{
			TaskId: taskID,
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   20,
				OrderBy: "id",
				Order:   "desc",
			},
		})
		if err != nil {
			return err
		}
		if responseRsp.GetTotal() != 6 {
			return utils.Errorf("unexpected response total: %d", responseRsp.GetTotal())
		}
		savedFailedCount := 0
		for _, item := range responseRsp.GetData() {
			if !item.GetOk() {
				savedFailedCount++
			}
		}
		if savedFailedCount != 3 {
			return utils.Errorf("unexpected saved failed count: %d", savedFailedCount)
		}

		taskRsp, err := c.QueryHistoryHTTPFuzzerTaskEx(context.Background(), &ypb.QueryHistoryHTTPFuzzerTaskExParams{
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   20,
				OrderBy: "id",
				Order:   "desc",
			},
		})
		if err != nil {
			return err
		}
		for _, item := range taskRsp.GetData() {
			if int64(item.GetBasicInfo().GetId()) != taskID {
				continue
			}
			if item.GetBasicInfo().GetHTTPFlowFailedCount() != 3 || item.GetBasicInfo().GetHTTPFlowSuccessCount() != 3 {
				return utils.Errorf("unexpected task stats: %#v", item.GetBasicInfo())
			}
			return nil
		}
		return utils.Errorf("task %d not found", taskID)
	}))

	retryStream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RetryTaskID: taskID,
	})
	require.NoError(t, err)

	retryCount := 0
	for {
		rsp, err := retryStream.Recv()
		if err != nil {
			break
		}
		if len(rsp.GetRequestRaw()) == 0 {
			continue
		}
		index, _ := strconv.Atoi(lowhttp.GetHTTPRequestQueryParam(rsp.GetRequestRaw(), "a"))
		if index%2 == 0 {
			require.True(t, rsp.Ok)
			continue
		}
		retryCount++
		require.True(t, rsp.Ok)
		require.Contains(t, string(rsp.ResponseRaw), retryToken)
	}
	require.Equal(t, 3, retryCount)
}

func TestGRPCMUSTPASS_HTTPFuzzer_RetryReMatchTaskOnlyRetriesFailedResponses(t *testing.T) {
	c, err := NewLocalClient()
	require.NoError(t, err)

	marker := "rematch-" + uuid.NewString()[:8]
	retryToken := "retry-" + marker
	okToken := "ok-" + marker
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		index, _ := strconv.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
		body := okToken
		if index%2 == 1 {
			body = retryToken
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	})
	target := utils.HostPort(host, port)

	historyStream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Request:   "GET /?marker=" + marker + "&a={{i(0-5)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
	})
	require.NoError(t, err)

	var historyTaskID int64
	historyCount := 0
	for {
		rsp, err := historyStream.Recv()
		if err != nil {
			break
		}
		historyCount++
		historyTaskID = rsp.GetTaskId()
		require.True(t, rsp.Ok)
	}
	require.Equal(t, 6, historyCount)
	require.NotZero(t, historyTaskID)

	require.NoError(t, utils.AttemptWithDelay(10, 200*time.Millisecond, func() error {
		taskRespCount, err := yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(historyTaskID))
		if err != nil {
			return err
		}
		if taskRespCount != 6 {
			return utils.Errorf("want 6 history task resp, but got %d", taskRespCount)
		}
		return nil
	}))

	rematchStream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "and",
				Group:       []string{retryToken},
				ExprType:    "nuclei-dsl",
				Action:      Action_Fail,
				HitColor:    "orange",
			},
		},
		HistoryWebFuzzerId: int32(historyTaskID),
		ReMatch:            true,
	})
	require.NoError(t, err)

	var rematchTaskID int64
	rematchCount := 0
	failedIndexes := make([]int, 0, 3)
	for {
		rsp, err := rematchStream.Recv()
		if err != nil {
			break
		}
		rematchCount++
		require.NotZero(t, rsp.GetTaskId())
		if rematchTaskID == 0 {
			rematchTaskID = rsp.GetTaskId()
		} else {
			require.Equal(t, rematchTaskID, rsp.GetTaskId())
		}

		index, _ := strconv.Atoi(lowhttp.GetHTTPRequestQueryParam(rsp.GetRequestRaw(), "a"))
		if strings.Contains(string(rsp.GetResponseRaw()), retryToken) {
			require.False(t, rsp.Ok)
			require.Equal(t, matcherActionFailReason, rsp.Reason)
			require.True(t, rsp.MatchedByMatcher)
			failedIndexes = append(failedIndexes, index)
			continue
		}
		require.True(t, rsp.Ok)
	}
	require.Equal(t, 6, rematchCount)
	require.ElementsMatch(t, []int{1, 3, 5}, failedIndexes)
	require.NotZero(t, rematchTaskID)

	require.NoError(t, utils.AttemptWithDelay(10, 200*time.Millisecond, func() error {
		taskRespCount, err := yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(rematchTaskID))
		if err != nil {
			return err
		}
		if taskRespCount != 6 {
			return utils.Errorf("want 6 rematch task resp, but got %d", taskRespCount)
		}
		return nil
	}))

	retryStream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RetryTaskID: rematchTaskID,
	})
	require.NoError(t, err)

	var retryTaskID int64
	replayedIndexes := make([]int, 0, 3)
	retriedIndexes := make([]int, 0, 3)
	for {
		rsp, err := retryStream.Recv()
		if err != nil {
			break
		}
		if len(rsp.GetRequestRaw()) == 0 {
			continue
		}
		require.True(t, rsp.Ok)
		require.Equal(t, marker, lowhttp.GetHTTPRequestQueryParam(rsp.GetRequestRaw(), "marker"))
		index, _ := strconv.Atoi(lowhttp.GetHTTPRequestQueryParam(rsp.GetRequestRaw(), "a"))
		if rsp.GetTaskId() == rematchTaskID {
			require.NotContains(t, string(rsp.GetResponseRaw()), retryToken)
			replayedIndexes = append(replayedIndexes, index)
			continue
		}

		require.NotZero(t, rsp.GetTaskId())
		if retryTaskID == 0 {
			retryTaskID = rsp.GetTaskId()
		} else {
			require.Equal(t, retryTaskID, rsp.GetTaskId())
		}
		retriedIndexes = append(retriedIndexes, index)
		require.Contains(t, string(rsp.GetResponseRaw()), retryToken)
	}
	require.ElementsMatch(t, []int{0, 2, 4}, replayedIndexes)
	require.ElementsMatch(t, []int{1, 3, 5}, retriedIndexes)
	require.NotZero(t, retryTaskID)
	require.NotEqual(t, rematchTaskID, retryTaskID)

	require.NoError(t, utils.AttemptWithDelay(10, 200*time.Millisecond, func() error {
		taskRespCount, err := yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(retryTaskID))
		if err != nil {
			return err
		}
		if taskRespCount != 3 {
			return utils.Errorf("want 3 retry task resp, but got %d", taskRespCount)
		}
		return nil
	}))
}
