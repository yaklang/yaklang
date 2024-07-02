package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
)

func TestGRPCMUSTPASS_HTTPFuzzer_History_Detail(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	t.Run("single request", func(t *testing.T) {
		client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			Request: `GET /?c=1 HTTP/1.1
	Host: ` + utils.HostPort(targetHost, targetPort) + `
	`,
		})
		if err != nil {
			t.Fatal(err)
		}

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
		if taskID == 0 {
			t.Fatal("No Response")
		}

		client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			HistoryWebFuzzerId: int32(taskID),
		})
		count := 0
		for {
			_, err := client.Recv()
			if err != nil {
				break
			}
			count++
		}
		if count != 1 {
			t.Fatalf("Get History WebFuzzer Detail Failed, want 1 response, but got %d", count)
		}
	})

	t.Run("multi request", func(t *testing.T) {
		client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			ForceFuzz:  true,
			Concurrent: 10,
			Request: `GET /?c={{int(1-10)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
		})
		if err != nil {
			t.Fatal(err)
		}

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
		if taskID == 0 {
			t.Fatal("TaskID not found in response")
		}

		err = utils.AttemptWithDelayFast(func() error {
			client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				HistoryWebFuzzerId: int32(taskID),
			})
			count := 0
			for {
				_, err := client.Recv()
				if err != nil {
					break
				}
				count++
			}
			if count != 10 {
				return utils.Errorf("Get History WebFuzzer Detail Failed, want 10 responses, but got %d", count)
			}
			return nil
		})
		require.NoError(t, err)
	})
}
