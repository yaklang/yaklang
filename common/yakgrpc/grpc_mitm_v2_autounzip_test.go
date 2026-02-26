package yakgrpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITMV2_ManualHijack_AutoUnzip_ViewPlainRequestAndResponse(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqPlainOriginal := "req-orig-" + uuid.NewString()
	reqPlainModified := "req-mod-" + uuid.NewString()
	respPlain := "resp-" + uuid.NewString()

	type serverSeen struct {
		contentEncoding string
		rawBody         []byte
		decodedBody     []byte
	}
	seenCh := make(chan serverSeen, 1)

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		ce := strings.TrimSpace(r.Header.Get("Content-Encoding"))
		decoded := raw
		if strings.Contains(strings.ToLower(ce), "gzip") {
			if out, err := utils.GzipDeCompress(raw); err == nil {
				decoded = out
			}
		}

		select {
		case seenCh <- serverSeen{contentEncoding: ce, rawBody: raw, decodedBody: decoded}:
		default:
		}

		respBody, _ := utils.GzipCompress([]byte(respPlain))
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(respBody)
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	target := "http://" + utils.HostPort(mockHost, mockPort) + "/autounzip"

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	}))

	require.NoError(t, stream.Send(&ypb.MITMV2Request{ResetFilter: true}))

	require.NoError(t, stream.Send(&ypb.MITMV2Request{
		SetAutoForward:   true,
		AutoForwardValue: false, // enable manual hijack
	}))
	require.NoError(t, stream.Send(&ypb.MITMV2Request{
		SetAutoUnzip:   true,
		AutoUnzipValue: true, // enable auto unzip/zip for manual hijack view
	}))

	clientErrCh := make(chan error, 1)
	started := false

	requestVerified := false
	responseVerified := false
	var taskID string

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}

		if msg.GetHaveMessage() && !started {
			msgStr := string(msg.GetMessage().GetMessage())
			if strings.Contains(msgStr, "starting mitm serve") || strings.Contains(msgStr, "starting mitm server") {
				started = true
				go func() {
					time.Sleep(200 * time.Millisecond)
					bodyGzip, _ := utils.GzipCompress([]byte(reqPlainOriginal))
					_, _, err := poc.DoPOST(target,
						poc.WithProxy(proxy),
						poc.WithTimeout(10),
						poc.WithNoRedirect(true),
						poc.WithBody(bodyGzip),
						poc.WithReplaceHttpPacketHeader("Content-Encoding", "gzip"),
					)
					if err != nil {
						clientErrCh <- err
					}
				}()
			}
		}

		select {
		case err := <-clientErrCh:
			t.Fatalf("client request failed: %v", err)
		default:
		}

		if len(msg.GetManualHijackList()) == 0 {
			continue
		}

		switch msg.GetManualHijackListAction() {
		case Hijack_List_Add:
			// Request enters manual hijack.
			for _, item := range msg.GetManualHijackList() {
				if requestVerified {
					continue
				}
				taskID = item.GetTaskID()
				require.NotEmpty(t, taskID)

				// Auto-unzip should make the "view" packet plain for the user (no Content-Encoding, no binary fuzztag).
				ce := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(item.GetRequest(), "Content-Encoding"))
				require.Equal(t, "", ce)
				require.False(t, bytes.Contains(item.GetRequest(), []byte("{{unquote")), "request view should not be converted to {{unquote}} when auto-unzip is enabled")

				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(item.GetRequest())
				require.Equal(t, []byte(reqPlainOriginal), body)

				// User edits plain request and sends it back; server should auto-zip to original encoding before forwarding.
				modifiedReq := lowhttp.ReplaceHTTPPacketBody(item.GetRequest(), []byte(reqPlainModified), false)
				require.NoError(t, stream.Send(&ypb.MITMV2Request{
					ManualHijackControl: true,
					ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
						TaskID:         taskID,
						HijackResponse: true,
						SendPacket:     true,
						Request:        modifiedReq,
					},
				}))

				requestVerified = true
			}

		case Hijack_List_Update:
			// Wait for response to be available in manual hijack view.
			for _, item := range msg.GetManualHijackList() {
				if responseVerified {
					continue
				}
				if item.GetTaskID() != taskID {
					continue
				}
				if item.GetStatus() != Hijack_Status_Response || item.GetResponse() == nil {
					continue
				}

				ce := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(item.GetResponse(), "Content-Encoding"))
				require.Equal(t, "", ce)
				require.False(t, bytes.Contains(item.GetResponse(), []byte("{{unquote")), "response view should not be converted to {{unquote}} when auto-unzip is enabled")

				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(item.GetResponse())
				require.Equal(t, []byte(respPlain), body)

				// Release the hijacked response and stop the test.
				require.NoError(t, stream.Send(&ypb.MITMV2Request{
					ManualHijackControl: true,
					ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
						TaskID:  taskID,
						Forward: true,
					},
				}))

				responseVerified = true
				cancel()
			}
		}
	}

	require.True(t, requestVerified, "request should enter manual hijack and be auto-unzipped for view")
	require.True(t, responseVerified, "response should enter manual hijack and be auto-unzipped for view")

	select {
	case seen := <-seenCh:
		require.True(t, strings.Contains(strings.ToLower(seen.contentEncoding), "gzip"), "server should receive re-zipped request, got Content-Encoding=%q", seen.contentEncoding)
		require.Equal(t, []byte(reqPlainModified), seen.decodedBody, "server should receive gzip body that decodes to the user-edited plain request")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for mock server to receive forwarded request")
	}
}
