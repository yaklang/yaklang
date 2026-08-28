package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/multipart"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
			cli, err := NewLocalClient()
			require.NoError(t, err)
			flowMsg, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), cli, &ypb.QueryHTTPFlowRequest{Keyword: uid, SourceType: "mitm"}, 1)
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
	// Wire forwarding must keep the full body even when History spill kicks in.
	// Lower 「转储数据包大小」 below body size so spill is exercised.
	// In CI, NewLocalClient dials the external yak grpc process: consts.Set*
	// in this test process does NOT affect MITM spill — use SetGlobalNetworkConfig.
	const bodySize = 300 * 1024
	const spillLimit = uint64(200 * 1024)

	client, err := NewLocalClient()
	require.NoError(t, err)
	cfg, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	require.NoError(t, err)
	prevMax := cfg.GetMaxContentLength()
	cfg.MaxContentLength = spillLimit
	_, err = client.SetGlobalNetworkConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		cfg.MaxContentLength = prevMax
		if prevMax == 0 {
			_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
			return
		}
		_, _ = client.SetGlobalNetworkConfig(context.Background(), cfg)
	})

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

			flowMsg, err := QueryHTTPFlows(utils.TimeoutContextSeconds(5), client, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, 1)
			require.NoError(t, err)
			require.Len(t, flowMsg.Data, 1)
			require.True(t, flowMsg.Data[0].IsTooLargeRequest, "300KB body should spill when GlobalMaxContentLength is 200KB")
		}),
	)
}

func TestGRPCMUSTPASS_MITM_ManualRequestRendersFileAndUserFuzzTag(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	token := uuid.New().String()
	registerHTTPFlowTokenCleanup(t, token)

	filePayload := []byte{0xff, 0x00, 'A'}
	filePath := filepath.Join(t.TempDir(), "mitm-v1-resource.bin")
	require.NoError(t, os.WriteFile(filePath, filePayload, 0o600))

	captured := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		select {
		case captured <- bytes.Clone(req):
		default:
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	})
	target := utils.HostPort(host, port)

	mitmPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	requestDone := make(chan error, 1)
	var submitted atomic.Bool

	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		require.NoError(t, stream.Send(&ypb.MITMRequest{Host: "127.0.0.1", Port: uint32(mitmPort)}))
		require.NoError(t, stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: false}))
	}, func(stream ypb.Yak_MITMClient) {
		time.Sleep(200 * time.Millisecond)
		_, _, requestErr := poc.DoPOST(
			"http://"+target+"/original?token="+token,
			poc.WithBody("original"),
			poc.WithProxy("http://127.0.0.1:"+fmt.Sprint(mitmPort)),
			poc.WithSave(false),
			poc.WithTimeout(10),
		)
		requestDone <- requestErr
		cancel()
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
		if len(msg.GetRequest()) == 0 || submitted.Swap(true) {
			return
		}
		edited := lowhttp.ReplaceHTTPPacketFirstLine(
			msg.GetRequest(),
			"POST /rendered-{{int(7)}}?token="+token+" HTTP/1.1",
		)
		edited = lowhttp.ReplaceHTTPPacketBody(edited, []byte("{{file("+filePath+")}}"), false)
		edited = lowhttp.ReplaceHTTPPacketHeader(edited, "Content-Length", "999")
		require.NoError(t, stream.Send(&ypb.MITMRequest{Id: msg.GetId(), Request: edited}))
	})

	select {
	case requestErr := <-requestDone:
		require.NoError(t, requestErr)
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for MITMv1 request completion")
	}
	select {
	case request := <-captured:
		require.Contains(t, string(request), "POST /rendered-7?token="+token+" HTTP/1.1")
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(request)
		require.Equal(t, filePayload, body)
		require.Equal(t, fmt.Sprint(len(filePayload)), lowhttp.GetHTTPPacketHeader(request, "Content-Length"))
	case <-time.After(2 * time.Second):
		t.Fatal("target server did not receive the rendered MITMv1 request")
	}
}

func TestGRPCMUSTPASS_MITM_ManualLargeMultipartUploadIsBounded(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	previousLimit := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(512 * 1024)
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(previousLimit) })

	payload := bytes.Repeat([]byte{0x00, 0xff, 0x10, 0x80}, 5*1024*1024/4)
	token := "mitm-v1-large-multipart-" + utils.RandStringBytes(8)
	registerHTTPFlowTokenCleanup(t, token)

	captured := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		select {
		case captured <- bytes.Clone(req):
		default:
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	})
	target := utils.HostPort(host, port)
	packet := buildMITMV2OutcomePacket(t, target, token, mitmV2RequestOutcomeCase{
		name:              "legacy-browser-upload-ten-times-D",
		originalPayload:   payload,
		multipart:         true,
		uploadFilename:    "large-upload.bin",
		uploadContentType: "application/octet-stream",
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	requestDone := make(chan error, 1)
	var forwarded atomic.Bool

	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		require.NoError(t, stream.Send(&ypb.MITMRequest{Host: "127.0.0.1", Port: uint32(mitmPort)}))
		require.NoError(t, stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: false}))
	}, func(stream ypb.Yak_MITMClient) {
		time.Sleep(200 * time.Millisecond)
		response, err := lowhttp.HTTP(
			lowhttp.WithPacketBytes(packet),
			lowhttp.WithProxy("http://127.0.0.1:"+fmt.Sprint(mitmPort)),
			lowhttp.WithTimeout(15*time.Second),
			lowhttp.WithSaveHTTPFlow(false),
		)
		if err == nil {
			require.Contains(t, string(response.RawPacket), "200 OK")
		}
		requestDone <- err
		cancel()
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
		if !bytes.Contains(msg.GetRequest(), []byte(token)) || forwarded.Swap(true) {
			return
		}
		require.True(t, utf8.Valid(msg.GetRequest()))
		require.Contains(t, string(msg.GetRequest()), "{{file(")
		_, editorBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(msg.GetRequest())
		require.LessOrEqual(t, len(editorBody), yakit.GetMaxHTTPFlowRequestBodyInDBBytes())
		require.NoError(t, stream.Send(&ypb.MITMRequest{Id: msg.GetId(), Forward: true}))
	})

	select {
	case err := <-requestDone:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for legacy MITM large multipart request")
	}
	select {
	case request := <-captured:
		require.Equal(t, payload, extractMITMV2OutcomePayload(t, mitmV2CapturedRequest{
			body:        lowhttp.GetHTTPPacketBody(request),
			contentType: lowhttp.GetHTTPPacketHeader(request, "Content-Type"),
		}, true))
	case <-time.After(2 * time.Second):
		t.Fatal("target server did not receive the legacy MITM upload")
	}

	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(8), client, &ypb.QueryHTTPFlowRequest{
		SearchURL: token, SourceType: "mitm", Full: true,
		Pagination: &ypb.Paging{Page: 1, Limit: 10},
	}, 1)
	require.NoError(t, err)
	require.Len(t, flows.GetData(), 1)
	flow := flows.GetData()[0]
	require.True(t, flow.GetIsTooLargeRequest())
	var persisted schema.HTTPFlow
	require.NoError(t, consts.GetGormProjectDatabase().First(&persisted, flow.GetId()).Error)
	require.FileExists(t, persisted.TooLargeRequestBodyFile)
	rebuilt, err := yakit.LoadHTTPFlowRequestPacket(&persisted)
	require.NoError(t, err)
	require.Equal(t, payload, extractMITMV2OutcomePayload(t, mitmV2CapturedRequest{
		body:        lowhttp.GetHTTPPacketBody(rebuilt),
		contentType: lowhttp.GetHTTPPacketHeader(rebuilt, "Content-Type"),
	}, true))
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
