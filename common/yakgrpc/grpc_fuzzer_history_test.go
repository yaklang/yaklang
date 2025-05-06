package yakgrpc

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_History_Detail(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	var targetHost string
	var targetPort int
	var serverStarted bool

	// 尝试5次启动服务器，只要有一次成功就继续测试
	for i := 0; i < 5; i++ {
		targetHost, targetPort = utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte("Hello"))
		})

		if utils.WaitConnect(utils.HostPort(targetHost, targetPort), 3) == nil {
			serverStarted = true
			break
		}

		log.Infof("attempt %d to start debug server failed, retrying...", i+1)
	}

	if !serverStarted {
		t.Fatal("debug server failed after 5 attempts")
	}

	t.Run("single request", func(t *testing.T) {
		var success bool
		var lastErr error

		for i := 0; i < 5; i++ {
			func() {
				client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
					Request: `GET /?c=1 HTTP/1.1
	Host: ` + utils.HostPort(targetHost, targetPort) + `
	`,
				})
				if err != nil {
					lastErr = err
					return
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
					lastErr = utils.Error("No Response")
					return
				}

				client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
					HistoryWebFuzzerId: int32(taskID),
				})
				if err != nil {
					lastErr = err
					return
				}

				count := 0
				for {
					_, err := client.Recv()
					if err != nil {
						break
					}
					count++
				}
				if count != 1 {
					lastErr = utils.Errorf("Get History WebFuzzer Detail Failed, want 1 response, but got %d", count)
					return
				}

				success = true
			}()

			if success {
				break
			}
		}

		if !success {
			t.Fatal(lastErr)
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
