package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_DeleteHistory(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	checkFuzzerTask := func(t *testing.T, ctx context.Context, fuzzerTabIndex string, wantNum int) {
		t.Helper()
		var queryFuzzerTaskRsp *ypb.HistoryHTTPFuzzerTasksResponse
		var err error
		err = utils.AttemptWithDelayFast(func() error {
			queryFuzzerTaskRsp, err = c.QueryHistoryHTTPFuzzerTaskEx(ctx, &ypb.QueryHistoryHTTPFuzzerTaskExParams{
				FuzzerTabIndex: fuzzerTabIndex,
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 10,
				},
			})
			if err != nil {
				return err
			}
			if int(queryFuzzerTaskRsp.Total) != wantNum {
				return utils.Errorf("want %d, got %d", wantNum, int(queryFuzzerTaskRsp.Total))
			}
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, wantNum, int(queryFuzzerTaskRsp.Total))
	}

	checkFuzzerResponse := func(t *testing.T, ctx context.Context, taskID int64, wantNum int) {
		t.Helper()

		var queryFuzzerResponseRsp *ypb.QueryHTTPFuzzerResponseByTaskIdResponse
		var err error
		err = utils.AttemptWithDelayFast(func() error {
			queryFuzzerResponseRsp, err = c.QueryHTTPFuzzerResponseByTaskId(ctx, &ypb.QueryHTTPFuzzerResponseByTaskIdRequest{
				TaskId: taskID,
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 10,
				},
			})
			if err != nil {
				return err
			}
			if int(queryFuzzerResponseRsp.Total) != wantNum {
				return utils.Errorf("want %d, got %d", wantNum, int(queryFuzzerResponseRsp.Total))
			}
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, wantNum, int(queryFuzzerResponseRsp.Total))
	}

	t.Run("id", func(t *testing.T) {
		fuzzerTabIndex := uuid.NewString()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := c.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
			FuzzerTabIndex: fuzzerTabIndex,
		})
		require.NoError(t, err)

		var taskID int64 = 0
		for {
			rsp, err := client.Recv()
			if err != nil {
				break
			}
			if taskID == 0 {
				taskID = rsp.GetTaskId()
			}
		}

		require.NotEqual(t, taskID, 0, "No Response")

		// before delete
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 1)
		checkFuzzerResponse(t, ctx, taskID, 1)

		// delete
		c.DeleteHistoryHTTPFuzzerTask(ctx, &ypb.DeleteHistoryHTTPFuzzerTaskRequest{
			Id: int32(taskID),
		})
		// check
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 0)
		checkFuzzerResponse(t, ctx, taskID, 0)
	})

	t.Run("fuzzer index", func(t *testing.T) {
		fuzzerTabIndex := uuid.NewString()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := c.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request: `GET /?c={{int(1-10)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
			FuzzerTabIndex: fuzzerTabIndex,
			ForceFuzz:      true,
			Concurrent:     10,
		})
		require.NoError(t, err)

		var taskID int64 = 0
		for {
			rsp, err := client.Recv()
			if err != nil {
				break
			}
			if taskID == 0 {
				taskID = rsp.GetTaskId()
			}
		}

		require.NotEqual(t, taskID, 0, "No Response")

		// before delete
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 1)
		checkFuzzerResponse(t, ctx, taskID, 10)

		// delete
		c.DeleteHistoryHTTPFuzzerTask(ctx, &ypb.DeleteHistoryHTTPFuzzerTaskRequest{
			WebFuzzerIndex: fuzzerTabIndex,
		})
		// check
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 0)
		checkFuzzerResponse(t, ctx, taskID, 0)
	})
}

func TestSetTagForRisk(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	r := yakit.CreateRisk("http://127.0.0.1")
	err = yakit.SaveRisk(r)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SetTagForRisk(context.Background(), &ypb.SetTagForRiskRequest{
		Hash: r.Hash,
		Tags: []string{"误报, 忽略, 待处理"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryRiskTags(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QueryRiskTags(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRiskFieldGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RiskFieldGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryRisks(t *testing.T) {
	testRuntimeId := uuid.New().String()
	randInt := rand.Intn(10) + 1
	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId,
		})
		require.NoError(t, err)
	}
	defer func() {
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId,
		})
	}()
	client, err := NewLocalClient()
	require.NoError(t, err)
	res, err := client.QueryRisks(context.Background(), &ypb.QueryRisksRequest{
		RuntimeId: testRuntimeId,
	})
	require.NoError(t, err)
	require.Equal(t, int64(randInt), res.Total)
}

func TestQueryRisksWithRuntimeIds(t *testing.T) {
	testRuntimeId := uuid.New().String()
	testRuntimeId2 := uuid.New().String()
	randInt := rand.Intn(10) + 1
	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId,
		})
		require.NoError(t, err)
	}

	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId2,
		})
		require.NoError(t, err)
	}
	defer func() {
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId,
		})
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId2,
		})
	}()
	client, err := NewLocalClient()
	require.NoError(t, err)
	res, err := client.QueryRisks(context.Background(), &ypb.QueryRisksRequest{
		RuntimeIds: []string{testRuntimeId, testRuntimeId2},
	})
	require.NoError(t, err)
	require.Equal(t, int64(randInt)*2, res.Total)
}
