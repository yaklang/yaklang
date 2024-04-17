package yakgrpc

import (
	"context"
	"fmt"
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
					PauseTaskID:    taskID,
					IsPause:        true,
					SetPauseStatus: true,
				})
				log.Info("start pause")
				inPause = true
				go func() {
					time.Sleep(1 * time.Second)
					client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
						PauseTaskID:    taskID,
						IsPause:        false,
						SetPauseStatus: true,
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

func TestHTTPFuzzer_Pause_SetPauseStatus(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	isPause := false
	targetHost, targetPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		fmt.Println("send request")
		if isPause {
			panic("pause failed")
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n")
	})

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RepeatTimes: 200000,
		ForceFuzz:   true,
		Concurrent:  1,
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `

`,
	})
	if err != nil {
		t.Fatal(err)
	}

	var taskID int64
	rsp, err := client.Recv()
	if err != nil {
		t.Fatalf("recv failed: %v", err)
	}
	taskID = rsp.TaskId

	_, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		PauseTaskID:    taskID,
		IsPause:        true,
		SetPauseStatus: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	isPause = true

	_, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		PauseTaskID:    taskID,
		IsPause:        false,
		SetPauseStatus: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)

}
