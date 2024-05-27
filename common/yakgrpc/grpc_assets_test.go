package yakgrpc

import (
	"context"
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
		queryFuzzerTaskRsp, err := c.QueryHistoryHTTPFuzzerTaskEx(ctx, &ypb.QueryHistoryHTTPFuzzerTaskExParams{
			FuzzerTabIndex: fuzzerTabIndex,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		require.Equal(t, wantNum, int(queryFuzzerTaskRsp.Total))
	}

	checkFuzzerResponse := func(t *testing.T, ctx context.Context, taskID int64, wantNum int) {
		t.Helper()
		queryFuzzerResponseRsp, err := c.QueryHTTPFuzzerResponseByTaskId(ctx, &ypb.QueryHTTPFuzzerResponseByTaskIdRequest{
			TaskId: taskID,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
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
