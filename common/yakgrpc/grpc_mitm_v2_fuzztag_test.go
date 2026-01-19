package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_MITMV2_HijackResponse_BinaryFuzztagRendering tests the complete flow:
// 1. Server returns binary data (PNG)
// 2. MITM converts it to {{unquote}} fuzztag for display
// 3. User intercepts and sends it back
// 4. MITM renders the fuzztag back to binary
// 5. Client receives the correct binary data
func TestGRPCMUSTPASS_MITMV2_HijackResponse_BinaryFuzztagRendering(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// Use a shorter timeout context since we'll cancel immediately on success
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a simple PNG-like binary data (PNG magic number + some data)
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		// Don't set Content-Type to avoid MIME filtering
		writer.Write(pngData)
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	// Initialize MITM server
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	// Reset filters to prevent .png from being filtered
	stream.Send(&ypb.MITMV2Request{
		ResetFilter: true,
	})

	// Clear all filters including MIME filters
	stream.Send(&ypb.MITMV2Request{
		UpdateFilter: true,
		FilterData: &ypb.MITMFilterData{
			ExcludeMIME: []*ypb.FilterDataItem{}, // Empty MIME filter
		},
	})

	// Disable auto-forward to enable manual hijacking
	stream.Send(&ypb.MITMV2Request{
		SetAutoForward:   true,
		AutoForwardValue: false,
	})

	fuzztagConverted := false
	responseSent := false
	clientReceivedCorrectData := false

	resultChan := make(chan []byte, 1)

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		rspMsg := string(rsp.GetMessage().GetMessage())

		// Send HTTP request after MITM server starts
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				// Wait for MITM server to fully initialize
				time.Sleep(time.Second)
				// Send HTTP request and extract response body
				_, err := yak.Execute(`
rsp, req, err = poc.Get(target, poc.proxy(mitmProxy), poc.save(false))
if err != nil {
    die(err)
}
# rsp is a response object, get the raw bytes
rspBytes = rsp.RawPacket
_, body = poc.Split(rspBytes)
resultChan <- body
`, map[string]any{
					"mitmProxy":  `http://` + utils.HostPort("127.0.0.1", mitmPort),
					"target":     fmt.Sprintf("http://%s/api/getimage", utils.HostPort(mockHost, mockPort)),
					"resultChan": resultChan,
				})
				if err != nil {
					t.Logf("HTTP request error: %v", err)
				}
				// Don't cancel here, let the main test loop handle it after verifying the data
			}()
		}

		// Handle manual hijack list
		if len(rsp.GetManualHijackList()) > 0 {
			if rsp.ManualHijackListAction == Hijack_List_Add {
				// Request is hijacked, enable response hijacking
				for _, message := range rsp.ManualHijackList {
					stream.Send(&ypb.MITMV2Request{
						ManualHijackControl: true,
						ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
							TaskID:         message.GetTaskID(),
							Forward:        true,
							HijackResponse: true,
						},
					})
				}
			}

			if rsp.ManualHijackListAction == Hijack_List_Update {
				// Response is ready
				for _, message := range rsp.ManualHijackList {
					if message.Status == Hijack_Status_Response && message.GetResponse() != nil {
						originalResponse := message.GetResponse()

						// Extract response body
						_, responseBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(originalResponse)

						// Verify the response body was converted to fuzztag for display
						if bytes.Contains(responseBody, []byte("{{unquote")) {
							fuzztagConverted = true
						}

						// Send the response back (with fuzztag), MITM should render it back to binary
						err := stream.Send(&ypb.MITMV2Request{
							ManualHijackControl: true,
							ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
								TaskID:     message.GetTaskID(),
								SendPacket: true,
								Response:   originalResponse, // Send back with fuzztag
							},
						})
						if err == nil {
							responseSent = true
						}
						// Give time for client to receive the response before we exit the loop
						go func() {
							time.Sleep(2 * time.Second)
							cancel()
						}()
					}
				}
			}
		}
	}

	// Check if client received the correct data
	t.Log("Waiting for client response...")
	select {
	case receivedBody := <-resultChan:
		t.Logf("Received body from channel: %d bytes", len(receivedBody))
		if bytes.Equal(receivedBody, pngData) {
			clientReceivedCorrectData = true
			t.Log("✓ Client received correct PNG data!")
		} else {
			t.Logf("✗ Client received incorrect data: got %d bytes, expected %d bytes", len(receivedBody), len(pngData))
			t.Logf("Received first 16 bytes: %v", receivedBody[:utils.Min(16, len(receivedBody))])
			t.Logf("Expected first 16 bytes: %v", pngData)
		}
		// Stop the test immediately after verification
		cancel()
	case <-time.After(5 * time.Second):
		t.Log("✗ Timeout waiting for client response")
		cancel()
	}

	require.True(t, fuzztagConverted, "Binary response should be converted to fuzztag for display")
	require.True(t, responseSent, "Response with fuzztag should be successfully sent back to MITM")
	require.True(t, clientReceivedCorrectData, "Client should receive correct PNG binary data (fuzztag rendered back by MITM)")
}

// TestGRPCMUSTPASS_MITMV2_HijackResponse_TextWithBracesNotRendered tests that when a server
// response contains fuzztag-like text (e.g., "{{int(1-5)}}") mixed with binary data:
// 1. The entire response is converted to {{unquote}} (because of binary data)
// 2. The fuzztag-like text inside is escaped during conversion
// 3. After rendering, it becomes plain text again (not re-rendered as fuzztag)
func TestGRPCMUSTPASS_MITMV2_HijackResponse_TextWithBracesNotRendered(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Server returns fuzztag-like text + binary data
	fuzztagLikeText := "{{int(1-5)}}"
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	mixedData := append([]byte(fuzztagLikeText), pngData...)

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write(mixedData)
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	// Initialize MITM server
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	stream.Send(&ypb.MITMV2Request{
		ResetFilter: true,
	})

	stream.Send(&ypb.MITMV2Request{
		UpdateFilter: true,
		FilterData: &ypb.MITMFilterData{
			ExcludeMIME: []*ypb.FilterDataItem{},
		},
	})

	stream.Send(&ypb.MITMV2Request{
		SetAutoForward:   true,
		AutoForwardValue: false,
	})

	fuzztagConverted := false
	clientReceivedCorrectData := false
	resultChan := make(chan []byte, 1)

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		rspMsg := string(rsp.GetMessage().GetMessage())

		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(time.Second)
				_, err := yak.Execute(`
rsp, req, err = poc.Get(target, poc.proxy(mitmProxy), poc.save(false))
if err != nil {
    die(err)
}
rspBytes = rsp.RawPacket
_, body = poc.Split(rspBytes)
resultChan <- body
`, map[string]any{
					"mitmProxy":  `http://` + utils.HostPort("127.0.0.1", mitmPort),
					"target":     fmt.Sprintf("http://%s/api/data", utils.HostPort(mockHost, mockPort)),
					"resultChan": resultChan,
				})
				if err != nil {
					t.Logf("HTTP request error: %v", err)
				}
			}()
		}

		if len(rsp.GetManualHijackList()) > 0 {
			if rsp.ManualHijackListAction == Hijack_List_Add {
				for _, message := range rsp.ManualHijackList {
					stream.Send(&ypb.MITMV2Request{
						ManualHijackControl: true,
						ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
							TaskID:         message.GetTaskID(),
							Forward:        true,
							HijackResponse: true,
						},
					})
				}
			}

			if rsp.ManualHijackListAction == Hijack_List_Update {
				for _, message := range rsp.ManualHijackList {
					if message.Status == Hijack_Status_Response && message.GetResponse() != nil {
						originalResponse := message.GetResponse()
						_, responseBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(originalResponse)

						// Verify mixed content was converted to {{unquote}}
						if bytes.Contains(responseBody, []byte("{{unquote")) {
							fuzztagConverted = true
							// Verify the text inside was escaped (should see \x7b not {{)
							if bytes.Contains(responseBody, []byte(`\x7b\x7b`)) {
								t.Log("✓ Fuzztag-like text was properly escaped in {{unquote}}")
							}
						}

						// Send back the response
						stream.Send(&ypb.MITMV2Request{
							ManualHijackControl: true,
							ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
								TaskID:     message.GetTaskID(),
								SendPacket: true,
								Response:   originalResponse,
							},
						})

						go func() {
							time.Sleep(2 * time.Second)
							cancel()
						}()
					}
				}
			}
		}
	}

	// Verify client received the original mixed data (not re-rendered)
	select {
	case receivedBody := <-resultChan:
		if bytes.Equal(receivedBody, mixedData) {
			clientReceivedCorrectData = true
			t.Log("✓ Client received correct mixed data ({{int}} as text, not re-rendered)")
		} else {
			t.Logf("✗ Data mismatch: got %d bytes, expected %d bytes", len(receivedBody), len(mixedData))
			t.Logf("Expected start: %s", string(mixedData[:utils.Min(20, len(mixedData))]))
			if len(receivedBody) > 0 {
				t.Logf("Received start: %s", string(receivedBody[:utils.Min(20, len(receivedBody))]))
			}
		}
		cancel()
	case <-time.After(5 * time.Second):
		t.Log("✗ Timeout waiting for client response")
		cancel()
	}

	require.True(t, fuzztagConverted, "Mixed content should be converted to {{unquote}}")
	require.True(t, clientReceivedCorrectData, "Client should receive original mixed data ({{int}} not re-rendered)")
}
