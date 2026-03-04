package yakgrpc

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type autoUnzipCaseConfig struct {
	pathSuffix      string
	requestChunked  bool
	requestGzip     bool
	responseChunked bool
	responseGzip    bool
}

type serverSeen struct {
	contentEncoding  string
	transferEncoding string
	rawBody          []byte
	decodedBody      []byte
}

func TestGRPCMUSTPASS_MITMV2_ManualHijack_AutoUnzip_ViewPlainRequestAndResponse(t *testing.T) {
	runMITMV2ManualHijackAutoUnzipCase(t, autoUnzipCaseConfig{
		pathSuffix:      "/plain-gzip",
		requestChunked:  false,
		requestGzip:     true,
		responseChunked: false,
		responseGzip:    true,
	})
}

func TestGRPCMUSTPASS_MITMV2_ManualHijack_AutoUnzip_ViewPlainRequestAndResponse_ChunkGzip(t *testing.T) {
	runMITMV2ManualHijackAutoUnzipCase(t, autoUnzipCaseConfig{
		pathSuffix:      "/chunk-gzip",
		requestChunked:  true,
		requestGzip:     true,
		responseChunked: true,
		responseGzip:    true,
	})
}

func TestGRPCMUSTPASS_MITMV2_ManualHijack_AutoUnzip_ViewPlainRequestAndResponse_ChunkRaw(t *testing.T) {
	runMITMV2ManualHijackAutoUnzipCase(t, autoUnzipCaseConfig{
		pathSuffix:      "/chunk-raw",
		requestChunked:  true,
		requestGzip:     false,
		responseChunked: true,
		responseGzip:    false,
	})
}

func runMITMV2ManualHijackAutoUnzipCase(t *testing.T, cfg autoUnzipCaseConfig) {
	t.Helper()

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqPlainOriginal := "req-orig-" + uuid.NewString()
	reqPlainModified := "req-mod-" + uuid.NewString()
	respPlain := "resp-" + uuid.NewString()

	seenCh := make(chan serverSeen, 1)

	mockHost, mockPort := utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {
		ce := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(req, "Content-Encoding"))
		te := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding"))
		rawBody := lowhttp.GetHTTPPacketBody(req)

		decoded := rawBody
		if strings.Contains(strings.ToLower(te), "chunked") {
			if out, err := codec.HTTPChunkedDecode(decoded); err == nil {
				decoded = out
			}
		}
		if strings.Contains(strings.ToLower(ce), "gzip") {
			if out, err := utils.GzipDeCompress(decoded); err == nil {
				decoded = out
			}
		}

		select {
		case seenCh <- serverSeen{
			contentEncoding:  ce,
			transferEncoding: te,
			rawBody:          rawBody,
			decodedBody:      decoded,
		}:
		default:
		}

		respBody := []byte(respPlain)
		if cfg.responseGzip {
			respBody, _ = utils.GzipCompress(respBody)
		}

		rsp := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n")
		if cfg.responseGzip {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		rsp = lowhttp.ReplaceHTTPPacketBody(rsp, respBody, cfg.responseChunked)
		return rsp
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	target := "http://" + utils.HostPort(mockHost, mockPort) + "/autounzip" + cfg.pathSuffix

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
					bodyRaw := []byte(reqPlainOriginal)
					if cfg.requestGzip {
						bodyRaw, _ = utils.GzipCompress(bodyRaw)
					}

					opts := []poc.PocConfigOption{
						poc.WithProxy(proxy),
						poc.WithTimeout(10),
						poc.WithNoRedirect(true),
					}
					if cfg.requestChunked {
						opts = append(opts, poc.WithReplaceHttpPacketBody(bodyRaw, true))
					} else {
						opts = append(opts, poc.WithBody(bodyRaw))
					}
					if cfg.requestGzip {
						opts = append(opts, poc.WithReplaceHttpPacketHeader("Content-Encoding", "gzip"))
					}

					_, _, err := poc.DoPOST(target, opts...)
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

				// Auto-unzip should make the "view" packet plain for the user (no Content-Encoding/Transfer-Encoding, no binary fuzztag).
				ce := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(item.GetRequest(), "Content-Encoding"))
				require.Equal(t, "", ce)
				te := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(item.GetRequest(), "Transfer-Encoding"))
				require.Equal(t, "", te)
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
				te := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(item.GetResponse(), "Transfer-Encoding"))
				require.Equal(t, "", te)
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
		if cfg.requestChunked {
			require.True(t, strings.Contains(strings.ToLower(seen.transferEncoding), "chunked"), "server should receive chunked request, got Transfer-Encoding=%q", seen.transferEncoding)
		} else {
			require.False(t, strings.Contains(strings.ToLower(seen.transferEncoding), "chunked"), "server should not receive chunked request, got Transfer-Encoding=%q", seen.transferEncoding)
		}

		if cfg.requestGzip {
			require.True(t, strings.Contains(strings.ToLower(seen.contentEncoding), "gzip"), "server should receive gzip request, got Content-Encoding=%q", seen.contentEncoding)
		} else {
			require.Equal(t, "", strings.TrimSpace(seen.contentEncoding), "server should receive plain request body without Content-Encoding")
		}

		require.Equal(t, []byte(reqPlainModified), seen.decodedBody, "server should receive request body that decodes to the user-edited plain request")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for mock server to receive forwarded request")
	}
}
