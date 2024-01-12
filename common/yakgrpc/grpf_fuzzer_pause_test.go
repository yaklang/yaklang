package yakgrpc

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_Pause(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(""))
	})
	target := utils.HostPort(host, port)
	req := &ypb.FuzzerRequest{
		Request: "GET /?a={{int(1-10)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
	}
	req.ForceFuzz = true
	req.Concurrent = 1
	stream, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), req)
	if err != nil {
		t.Fatal(err)
	}
	count, taskID := 0, int64(0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var finalErr error

	go func(t *testing.T) {
		defer wg.Done()
		inPause := false
		for {
			rsp, err := stream.Recv()
			if err != nil {
				finalErr = err
				return
			} else if inPause {
				finalErr = utils.Error("should not receive any response when in pause")
				return
			}
			taskID = rsp.TaskId
			count++
			if count == 2 {
				client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
					PauseTaskID: taskID,
					IsPause:     true,
				})
				log.Info("start pause")
				inPause = true
				go func() {
					time.Sleep(1 * time.Second)
					client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
						PauseTaskID: taskID,
						IsPause:     false,
					})
					log.Info("start continue")
					inPause = false
				}()
			} else if count == 10 {
				return
			}
		}
	}(t)

	wg.Wait()
	if finalErr != nil {
		t.Fatal(finalErr)
	}
	if count != 10 {
		t.Fatalf("expected 10 times, got %d", count)
	}
}
