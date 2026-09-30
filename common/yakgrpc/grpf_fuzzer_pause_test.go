package yakgrpc

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_Pause(t *testing.T) {
	testHTTPFuzzerPause(t, false)
}

func TestGRPCMUSTPASS_HTTPFUZZER_Pause_SetPauseStatus(t *testing.T) {
	testHTTPFuzzerPause(t, true)
}

func testHTTPFuzzerPause(t *testing.T, checkUnsetFlag bool) {
	t.Helper()
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Hold later requests while the pause RPC is acknowledged. This avoids
	// mistaking responses already in flight for a failure of the pause switch.
	permits := make(chan struct{}, 10)
	thirdArrived, thirdServed := make(chan struct{}), make(chan struct{})
	var arrivals atomic.Int32
	permits <- struct{}{}
	permits <- struct{}{}
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		n := arrivals.Add(1)
		if n == 3 {
			close(thirdArrived)
		}
		select {
		case <-permits:
			_, _ = w.Write([]byte("resumed request"))
			if n == 3 {
				close(thirdServed)
			}
		case <-r.Context().Done():
		}
	})
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:    "GET /?a={{int(1-10)}} HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		ForceFuzz:  true,
		Concurrent: 1,
	})
	require.NoError(t, err)
	var taskID int64
	for i := 0; i < 2; i++ {
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.GetOk())
		taskID = rsp.GetTaskId()
	}
	setPause := func(paused bool) {
		control, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{PauseTaskID: taskID, IsPause: paused, SetPauseStatus: true})
		require.NoError(t, err)
		_, err = control.Recv()
		require.ErrorIs(t, err, io.EOF, "the control RPC must finish before checking its state")
	}
	select {
	case <-thirdArrived:
	case <-ctx.Done():
		t.Fatal("third request did not reach the response gate")
	}
	setPause(true)
	value, ok := _FuzzerTaskSwitchMap.Load(uint(taskID))
	require.True(t, ok)
	sw := value.(*utils.Switch)
	isOpen := func() bool {
		sw.L.Lock()
		defer sw.L.Unlock()
		return sw.Condition()
	}
	require.False(t, isOpen())
	if checkUnsetFlag {
		control, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{PauseTaskID: taskID, IsPause: false, SetPauseStatus: false})
		require.NoError(t, err)
		_, err = control.Recv()
		require.Error(t, err, "a request without SetPauseStatus must not resume the task")
		require.False(t, isOpen(), "SetPauseStatus=false must leave the original task paused")
	}
	close(permits)
	select {
	case <-thirdServed:
	case <-ctx.Done():
		t.Fatal("in-flight response was not released")
	}
	type responseResult struct {
		rsp *ypb.FuzzerResponse
		err error
	}
	next := make(chan responseResult, 1)
	go func() { rsp, err := stream.Recv(); next <- responseResult{rsp, err} }()
	// A short negative observation remains necessary: a paused task must not
	// deliver the now-complete in-flight response or start another request.
	select {
	case <-next:
		t.Fatal("received a response while the acknowledged pause was active")
	case <-time.After(20 * time.Millisecond):
	}
	// The pool checks the switch before acquiring its concurrency semaphore;
	// one request can already be admitted behind the in-flight third request.
	require.LessOrEqual(t, arrivals.Load(), int32(4), "pause must prevent requests beyond the already admitted request")
	setPause(false)
	require.True(t, isOpen())
	result := <-next
	require.NoError(t, result.err)
	require.True(t, result.rsp.GetOk())
	for i := 3; i < 10; i++ {
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.GetOk())
		require.Equal(t, taskID, rsp.GetTaskId())
	}
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF, "all ten requests must finish after resuming")
}
