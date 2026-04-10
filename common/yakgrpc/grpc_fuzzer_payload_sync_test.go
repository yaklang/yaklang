package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func waitBody(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case body := <-ch:
		return body
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting request body")
		return ""
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_SyncPayloadGroup_UnequalLength(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	groupShort := "sync-short-" + uuid.NewString()
	groupLong := "sync-long-" + uuid.NewString()
	save2file(client, t, groupShort, "", "alpha\nbeta\n")
	save2file(client, t, groupLong, "", "one\ntwo\nthree\n")
	defer deleteGroup(client, t, groupShort)
	defer deleteGroup(client, t, groupLong)

	bodyCh := make(chan string, 3)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodyCh <- string(raw)
		_, _ = w.Write([]byte("ok"))
	})

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n\r\n{{payload::1(%s)}}---{{payload::1(%s)}}", utils.HostPort(host, port), groupShort, groupLong),
		ForceFuzz:        true,
		FuzzTagSyncIndex: true,
		Concurrent:       1,
	})
	require.NoError(t, err)

	var bodies []string
	for i := 0; i < 3; i++ {
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.Ok)
		bodies = append(bodies, waitBody(t, bodyCh))
	}

	require.Equal(t, []string{
		"alpha---one",
		"beta---two",
		"---three",
	}, bodies)
}

func TestGRPCMUSTPASS_HTTPFuzzer_SyncPayloadGroup_PreserveAtSign(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	groupLeft := "sync-left-" + uuid.NewString()
	groupRight := "sync-right-" + uuid.NewString()
	save2file(client, t, groupLeft, "", "left\n")
	save2file(client, t, groupRight, "", "17@29@32@58@17@37@5@43\n")
	defer deleteGroup(client, t, groupLeft)
	defer deleteGroup(client, t, groupRight)

	bodyCh := make(chan string, 1)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodyCh <- string(raw)
		_, _ = w.Write([]byte("ok"))
	})

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n\r\n{{payload::1(%s)}}---{{payload::1(%s)}}", utils.HostPort(host, port), groupLeft, groupRight),
		ForceFuzz:        true,
		FuzzTagSyncIndex: true,
		Concurrent:       1,
	})
	require.NoError(t, err)

	rsp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, rsp.Ok)

	require.Equal(t, "left---17@29@32@58@17@37@5@43", waitBody(t, bodyCh))
	require.Contains(t, string(rsp.RequestRaw), "17@29@32@58@17@37@5@43")
	require.NotContains(t, string(rsp.RequestRaw), "%40")
}

func TestGRPCMUSTPASS_HTTPFuzzer_SyncPayloadGroup_RepLabel(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	groupShort := "sync-rep-short-" + uuid.NewString()
	groupLong := "sync-rep-long-" + uuid.NewString()
	save2file(client, t, groupShort, "", "alpha\nbeta\n")
	save2file(client, t, groupLong, "", "one\ntwo\nthree\n")
	defer deleteGroup(client, t, groupShort)
	defer deleteGroup(client, t, groupLong)

	bodyCh := make(chan string, 3)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodyCh <- string(raw)
		_, _ = w.Write([]byte("ok"))
	})

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n\r\n{{payload::1::rep(%s)}}---{{payload::1(%s)}}", utils.HostPort(host, port), groupShort, groupLong),
		ForceFuzz:        true,
		FuzzTagSyncIndex: true,
		Concurrent:       1,
	})
	require.NoError(t, err)

	var bodies []string
	for i := 0; i < 3; i++ {
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.Ok)
		bodies = append(bodies, waitBody(t, bodyCh))
	}

	require.Equal(t, []string{
		"alpha---one",
		"beta---two",
		"beta---three",
	}, bodies)
}
