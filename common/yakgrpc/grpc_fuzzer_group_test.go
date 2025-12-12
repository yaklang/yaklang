package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

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

func TestGRPCMUSTPASS_HTTPFuzzerGroup_OverrideRepeatTimes(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var (
		hitLock sync.Mutex
		hit     int
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`ok`))
		hitLock.Lock()
		hit++
		hitLock.Unlock()
	})

	stream, err := client.HTTPFuzzerGroup(utils.TimeoutContextSeconds(10), &ypb.GroupHTTPFuzzerRequest{
		Requests: []*ypb.FuzzerRequest{
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /repeat HTTP/1.1
Host: example.com

`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				ForceFuzz:                true,
			},
		},
		Concurrent: 1,
		Overrides: &ypb.GroupHTTPFuzzerOverrides{
			RepeatTimes: 2,
		},
	})
	require.NoError(t, err)

	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			goto done
		default:
		}
		resp, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		if resp == nil {
			continue
		}
	}

done:
	hitLock.Lock()
	defer hitLock.Unlock()
	require.GreaterOrEqual(t, hit, 2)
}

func BenchmarkHTTPFuzzerGroup_Concurrent(b *testing.B) {
	client, err := NewLocalClient()
	if err != nil {
		b.Fatalf("new client: %v", err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`ok`))
	})

	const (
		totalRequests = 2000
		concurrent    = 50
	)

	requests := make([]*ypb.FuzzerRequest, 0, totalRequests)
	for i := 0; i < totalRequests; i++ {
		packet := fmt.Sprintf("GET /bench-%d HTTP/1.1\nHost: example.com\n\n", i%5)
		req := &ypb.FuzzerRequest{
			FuzzerIndex:              fmt.Sprintf("bench-%d", i),
			Request:                  string(lowhttp.ReplaceHTTPPacketHeader([]byte(packet), "Host", utils.HostPort(host, port))),
			IsHTTPS:                  false,
			PerRequestTimeoutSeconds: 5,
			ForceFuzz:                true,
		}
		requests = append(requests, req)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		stream, err := client.HTTPFuzzerGroup(ctx, &ypb.GroupHTTPFuzzerRequest{
			Requests:   requests,
			Concurrent: concurrent,
		})
		if err != nil {
			cancel()
			b.Fatalf("HTTPFuzzerGroup: %v", err)
		}
		for {
			if _, recvErr := stream.Recv(); recvErr != nil {
				if recvErr != io.EOF {
					cancel()
					b.Fatalf("recv: %v", recvErr)
				}
				break
			}
		}
		cancel()
	}
}
