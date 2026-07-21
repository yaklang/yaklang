package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/multipart"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_mitm_upload_test_embed_file.jpg
var embedJPEG []byte

func TestMITM_UploadFile(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\n\n")
	})
	target := utils.HostPort(host, port)

	mitmPort := 0

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel
	hash1 := codec.Md5(string(embedJPEG))
	uid := uuid.New().String()
	NewMITMTestCase(t,
		CaseWithContext(ctx),
		CaseWithPort(func(i int) {
			mitmPort = i
		}),
		CaseWithServerStart(func() {
			rsp, _, err := poc.DoGET(
				`http://`+target+"/"+uid,
				poc.WithAppendHttpPacketUploadFile("file", "test.jpg", string(embedJPEG), "text/plain", "test"),
				poc.WithProxy("http://127.0.0.1:"+fmt.Sprint(mitmPort)), poc.WithSave(false),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, reqBody := lowhttp.SplitHTTPPacketFast(rsp.RawRequest)
			reader := multipart.NewReader(bytes.NewReader(reqBody))
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				fileContentRequest, _ := io.ReadAll(part)
				if len(fileContentRequest) <= 0 {
					continue
				}
				if ret := codec.Md5(string(fileContentRequest)); ret != hash1 {
					fmt.Println("origin  len: ", len(embedJPEG), "hash", hash1)
					fmt.Println("request len: ", len(fileContentRequest), "hash", ret)
					t.Fatal("build packet error")
				}
			}
			log.Info("Start to check request in table")
			flowMsg, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), localClient, &ypb.QueryHTTPFlowRequest{Keyword: uid, SourceType: "mitm"}, 1)
			require.NoError(t, err)
			flow := flowMsg.Data[0]
			log.Info("check flow in mitm")
			_, reqBody = lowhttp.SplitHTTPPacketFast(flow.Request)
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				fileContentRequest, _ := io.ReadAll(part)
				if len(fileContentRequest) <= 0 {
					continue
				}
				if ret := codec.Md5(string(fileContentRequest)); ret != hash1 {
					fmt.Println("origin       len: ", len(embedJPEG), "hash", hash1)
					fmt.Println("mitm request len: ", len(fileContentRequest), "hash", ret)
					t.Fatal("build packet error")
				}
			}
			log.Info("finished")
			cancel()
		}))
}

func TestMITM_LargeRequestWireForward(t *testing.T) {
	// Default spill cap is 200KB (History preview); GlobalMaxContentLength stays at ~10MB for MITM I/O.
	const bodySize = 300 * 1024
	token := uuid.New().String()
	var receivedBodyLen atomic.Int64

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		_, body := lowhttp.SplitHTTPPacketFast(req)
		receivedBodyLen.Store(int64(len(body)))
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	})
	target := utils.HostPort(host, port)
	body := strings.Repeat("Z", bodySize)

	mitmPort := 0
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	NewMITMTestCase(t,
		CaseWithContext(ctx),
		CaseWithPort(func(i int) { mitmPort = i }),
		CaseWithServerStart(func() {
			defer cancel()
			_, _, err := poc.DoPOST(
				"http://"+target+"/"+token,
				poc.WithBody(body),
				poc.WithProxy("http://127.0.0.1:"+fmt.Sprint(mitmPort)),
				poc.WithSave(false),
				poc.WithTimeout(15),
			)
			require.NoError(t, err)
			require.GreaterOrEqual(t, receivedBodyLen.Load(), int64(bodySize))

			flowMsg, err := QueryHTTPFlows(utils.TimeoutContextSeconds(5), localClient, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, 1)
			require.NoError(t, err)
			require.Len(t, flowMsg.Data, 1)
			require.True(t, flowMsg.Data[0].IsTooLargeRequest, "300KB body should spill at 200KB History threshold without lowering GlobalMaxContentLength")
		}),
	)
}

func TestMITM_InvalidUTF8Request(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\n\n")
	})
	target := "http://" + utils.HostPort(host, port)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	isRecvRequest := false

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})

		stream.Send(&ypb.MITMRequest{
			SetAutoForward:   true,
			AutoForwardValue: false,
		})
	}, func(stream ypb.Yak_MITMClient) {
		// Wait for SetAutoForward configuration to take effect before sending request
		time.Sleep(200 * time.Millisecond)
		b, _ := codec.Utf8ToGB18030([]byte(`你好`))
		poc.DoPOST(target, poc.WithProxy(fmt.Sprintf("http://127.0.0.1:%d", mitmPort)), poc.WithBody(b))
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
		request := msg.GetRequest()
		if len(request) == 0 {
			return
		}

		defer cancel()
		isRecvRequest = true
		require.Contains(t, string(request), `{{unquote("\xc4\xe3\xba\xc3")}}`, "request should be wrapped by unquote fuzztag")

		stream.Send(&ypb.MITMRequest{
			Forward: true,
		})
	})

	require.True(t, isRecvRequest, "mitm server should hijack request")

}

func TestGRPCMUSTPASS_MITMV2_InvalidUTF8Request(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\n\n")
	})
	target := "http://" + utils.HostPort(host, port)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	isRecvRequest := false

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
		stream.Send(&ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})

		stream.Send(&ypb.MITMV2Request{
			SetAutoForward:   true,
			AutoForwardValue: false,
		})
	}, func(stream ypb.Yak_MITMV2Client) {
		// Wait for SetAutoForward configuration to take effect before sending request
		time.Sleep(200 * time.Millisecond)
		b, _ := codec.Utf8ToGB18030([]byte(`你好`))
		poc.DoPOST(target, poc.WithProxy(fmt.Sprintf("http://127.0.0.1:%d", mitmPort)), poc.WithBody(b))
	}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
		if msg.ManualHijackListAction != Hijack_List_Add {
			return
		}
		require.Len(t, msg.ManualHijackList, 1)
		hijackTask := msg.ManualHijackList[0]
		require.Equal(t, hijackTask.Status, Hijack_Status_Request)
		request := hijackTask.GetRequest()

		defer cancel()
		isRecvRequest = true
		require.Contains(t, string(request), `{{unquote("\xc4\xe3\xba\xc3")}}`, "request should be wrapped by unquote fuzztag")

		stream.Send(&ypb.MITMV2Request{
			ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
				TaskID:  hijackTask.TaskID,
				Forward: true,
			},
			ManualHijackControl: true,
		})
	})

	require.True(t, isRecvRequest, "mitm server should hijack request")

}
