package yakgrpc

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzerGroup_Basic(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var (
		counterLock sync.Mutex
		counter     = map[string]int{}
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`ok`))
	})

	requests := []*ypb.FuzzerRequest{
		{
			FuzzerIndex: "fuzzer-a",
			Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /a HTTP/1.1
Host: example.com

`), "Host", utils.HostPort(host, port))),
			IsHTTPS:                  false,
			PerRequestTimeoutSeconds: 5,
			ForceFuzz:                true,
		},
		{
			FuzzerIndex: "fuzzer-b",
			Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /b HTTP/1.1
Host: example.com

`), "Host", utils.HostPort(host, port))),
			IsHTTPS:                  false,
			PerRequestTimeoutSeconds: 5,
			ForceFuzz:                true,
		},
	}

	stream, err := client.HTTPFuzzerGroup(utils.TimeoutContextSeconds(10), &ypb.GroupHTTPFuzzerRequest{
		Requests:   requests,
		Concurrent: 2,
	})
	require.NoError(t, err)

	for {
		resp, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		if resp == nil || resp.GetRequest() == nil {
			continue
		}
		idx := resp.GetRequest().GetFuzzerIndex()
		if idx == "" {
			continue
		}
		counterLock.Lock()
		counter[idx]++
		counterLock.Unlock()
	}

	counterLock.Lock()
	defer counterLock.Unlock()
	require.Greater(t, counter["fuzzer-a"], 0)
	require.Greater(t, counter["fuzzer-b"], 0)
}
