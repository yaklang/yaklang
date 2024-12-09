package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"testing"
	"time"
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

func TestGRPCMUSTPASS_HTTPFuzzer_SNI(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	dataToken := utils.RandStringBytes(10)
	addr := utils.GetRandomLocalAddr()
	address, port := utils.DebugMockHTTPServerWithContextWithAddress(utils.TimeoutContext(30*time.Second), addr, true, false, false, false, false, true, func(bytes []byte) []byte {
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length:10\r\n\r\n%s", dataToken))
	})
	target := utils.HostPort(address, port)

	t.Run("sni overwriter empty", func(t *testing.T) {
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw:    []byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n"),
			SNI:           "",
			OverwriteSNI:  true,
			IsHTTPS:       true,
			NoSystemProxy: true,
		})
		require.NoError(t, err)
		handShankErrorCheck := false
		for {
			recv, err := fuzzer.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			if strings.Contains(recv.Reason, "all tls strategy failed") {
				handShankErrorCheck = true
			}
		}
		require.True(t, handShankErrorCheck)
	})

	t.Run("sni overwriter ", func(t *testing.T) {
		token := utils.RandStringBytes(10)
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw:    []byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n"),
			SNI:           token,
			OverwriteSNI:  true,
			IsHTTPS:       true,
			NoSystemProxy: true,
		})
		require.NoError(t, err)
		handShankErrorCheck := false
		for {
			recv, err := fuzzer.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			if strings.Contains(recv.Reason, "all tls strategy failed") || strings.Contains(recv.Reason, token) {
				handShankErrorCheck = true
			}
		}
		require.True(t, handShankErrorCheck)
	})

	t.Run("sni auto ", func(t *testing.T) {
		token := utils.RandStringBytes(10)
		fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			RequestRaw:    []byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n"),
			SNI:           token,
			IsHTTPS:       true,
			NoSystemProxy: true,
		})
		require.NoError(t, err)
		handShankOK := false
		for {
			recv, err := fuzzer.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			if bytes.Contains(recv.ResponseRaw, []byte(dataToken)) {
				handShankOK = true
			}
		}
		require.True(t, handShankOK)
	})

}
