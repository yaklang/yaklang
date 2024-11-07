package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_FuzzerConfig(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var pageIds []string
	saveFuzzerConfig := func(num int) {
		var Data []*ypb.FuzzerConfig
		for i := 0; i < num; i++ {
			pageId := uuid.New().String()
			pageIds = append(pageIds, pageId)
			Data = append(Data, &ypb.FuzzerConfig{
				PageId: pageId,
				Type:   "group",
				Config: "{\"isHTTPS\":true",
			})
			req := &ypb.SaveFuzzerConfigRequest{
				Data: Data,
			}
			_, err = client.SaveFuzzerConfig(context.Background(), req)
			require.NoError(t, err)
		}
	}
	queryAll := &ypb.QueryFuzzerConfigRequest{Pagination: &ypb.Paging{Limit: -1}}
	originResult, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.NoError(t, err)
	saveFuzzerConfig(100)

	newResult, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.NoError(t, err)
	require.Equal(t, len(originResult.GetData())+100, len(newResult.GetData()))

	res, err := client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Pagination: &ypb.Paging{Limit: 10}})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	res, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{PageId: pageIds[:10]})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	res, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{PageId: pageIds[:15]})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	res, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{PageId: pageIds[:10], Pagination: &ypb.Paging{Limit: 15}})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	_, err = client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{PageId: pageIds})
	require.NoError(t, err)
	result, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.Equal(t, len(originResult.GetData()), len(result.GetData()))
}
func TestGRPCMUSTPASS_MaxSize(t *testing.T) {
	consts.SetGlobalMaxContentLength(1024 * 1024)
	defer consts.SetGlobalMaxContentLength(1024 * 1024 * 10)
	t.Run("control by webfuzzer", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		data := bytes.Repeat([]byte("a"), 1024*1024*2)

		address, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %v\r\n\r\n%s", len(data), data)))
		require.NoError(t, err)
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw:  []byte("GET / HTTP/1.1\r\nHost: " + utils.HostPort(address, port) + "\r\n\r\n"),
			MaxBodySize: 1024 * 1024 * 1,
		})
		require.NoError(t, err)
		for {
			recv, err := fuzzer.Recv()
			require.NoError(t, err)
			require.True(t, len(recv.ResponseRaw) < len(data))
			require.True(t, len(recv.ResponseRaw) > 1024*1024*1)
			fmt.Println(len(recv.ResponseRaw))
			break
		}
	})
	t.Run("control by global", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		data := bytes.Repeat([]byte("a"), 1024*1024*2)
		address, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %v\r\n\r\n%s", len(data), data)))
		require.NoError(t, err)
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw: []byte("GET / HTTP/1.1\r\nHost: " + utils.HostPort(address, port) + "\r\n\r\n"),
		})
		require.NoError(t, err)
		for {
			recv, err := fuzzer.Recv()
			require.NoError(t, err)
			fmt.Println(len(recv.ResponseRaw))
			require.True(t, len(recv.ResponseRaw) < len(data))
			require.True(t, len(recv.ResponseRaw) > 1024*1024*1)
			break
		}
	})
	t.Run("webfuzzer priority", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)
		data := bytes.Repeat([]byte("a"), 1024*1024*2)
		address, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %v\r\n\r\n%s", len(data), data)))
		require.NoError(t, err)
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw:  []byte("GET / HTTP/1.1\r\nHost: " + utils.HostPort(address, port) + "\r\n\r\n"),
			MaxBodySize: 1024 * 1024 * 2,
		})
		require.NoError(t, err)
		for {
			recv, err := fuzzer.Recv()
			require.NoError(t, err)
			require.True(t, len(recv.ResponseRaw) > len(data))
			fmt.Println(len(recv.ResponseRaw))
			break
		}
	})
}
