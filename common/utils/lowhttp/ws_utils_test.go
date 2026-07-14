package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWebsocketClientWriteMaskMatchesRole(t *testing.T) {
	tests := []struct {
		name       string
		serverMode bool
		wantMasked bool
	}{
		{name: "client frames are masked", wantMasked: true},
		{name: "server frames are not masked", serverMode: true, wantMasked: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, write := range []struct {
				name string
				fn   func(*WebsocketClient) error
			}{
				{name: "text", fn: func(c *WebsocketClient) error { return c.WriteText([]byte("hello")) }},
				{name: "binary", fn: func(c *WebsocketClient) error { return c.WriteBinary([]byte("hello")) }},
				{name: "close", fn: func(c *WebsocketClient) error { return c.WriteClose() }},
			} {
				t.Run(write.name, func(t *testing.T) {
					var output bytes.Buffer
					client := NewWebsocketClientIns(
						nil,
						NewFrameReader(bytes.NewReader(nil), false),
						NewFrameWriter(&output, false),
						GetWebsocketExtensions(nil),
						WithWebsocketServerMode(test.serverMode),
					)
					defer client.cancel()

					require.NoError(t, write.fn(client))
					require.GreaterOrEqual(t, output.Len(), 2)
					require.Equal(t, test.wantMasked, output.Bytes()[1]&MASKBIT != 0)
				})
			}
		})
	}
}

func TestWebsocketMaskedWriteDoesNotMutatePayload(t *testing.T) {
	payload := []byte("caller-owned websocket payload")
	want := bytes.Clone(payload)
	_, err := NewFrameWriter(io.Discard, false).WriteDirect(true, false, TextMessage, true, payload)
	require.NoError(t, err)
	require.Equal(t, want, payload)
}

func TestIsExpectedWebsocketReadError(t *testing.T) {
	require.True(t, isExpectedWebsocketReadError(io.EOF))
	require.True(t, isExpectedWebsocketReadError(net.ErrClosed))
	require.True(t, isExpectedWebsocketReadError(context.Canceled))
	require.True(t, isExpectedWebsocketReadError(errors.Join(errors.New("read frame"), net.ErrClosed)))
	require.False(t, isExpectedWebsocketReadError(errors.New("invalid websocket frame")))
}

func TestWebsocketClientWriteCloseReason(t *testing.T) {
	var output bytes.Buffer
	client := NewWebsocketClientIns(
		nil,
		NewFrameReader(bytes.NewReader(nil), false),
		NewFrameWriter(&output, false),
		GetWebsocketExtensions(nil),
		WithWebsocketServerMode(true),
	)
	defer client.cancel()

	require.NoError(t, client.WriteCloseEx(ClosePolicyViolation, "yak policy"))
	frame, err := NewFrameReader(bytes.NewReader(output.Bytes()), false).ReadFrame()
	require.NoError(t, err)
	require.Equal(t, CloseMessage, frame.Type())
	require.Equal(t, ClosePolicyViolation, frame.GetCloseCode())
	require.Equal(t, "yak policy", string(frame.GetData()))
	require.False(t, frame.GetMask())

	require.Error(t, client.WriteCloseEx(CloseNormalClosure, string([]byte{0xff})))
	require.Error(t, client.WriteCloseEx(0, "reason without code"))
	require.Error(t, client.WriteCloseEx(CloseNormalClosure, strings.Repeat("x", 124)))
}

func TestValidateWebsocketUpgradeResponse(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	request := []byte("GET /chat HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Protocol: chat, superchat\r\n\r\n")
	valid := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header: http.Header{
				"Upgrade":                []string{"WebSocket"},
				"Connection":             []string{"keep-alive, Upgrade"},
				"Sec-WebSocket-Accept":   []string{ComputeWebsocketAcceptKey(key)},
				"Sec-WebSocket-Protocol": []string{"superchat"},
			},
		}
	}
	require.NoError(t, validateWebsocketUpgradeResponse(request, valid()))

	badAccept := valid()
	badAccept.Header["Sec-WebSocket-Accept"] = []string{"invalid"}
	require.Error(t, validateWebsocketUpgradeResponse(request, badAccept))

	missingConnection := valid()
	delete(missingConnection.Header, "Connection")
	require.Error(t, validateWebsocketUpgradeResponse(request, missingConnection))

	unofferedProtocol := valid()
	unofferedProtocol.Header["Sec-WebSocket-Protocol"] = []string{"other"}
	require.Error(t, validateWebsocketUpgradeResponse(request, unofferedProtocol))
}

func TestWebsocketPermessageDeflateNegotiation(t *testing.T) {
	request := http.Header{}
	request.Set("Sec-WebSocket-Extensions", "permessage-deflate; client_no_context_takeover; client_max_window_bits; server_no_context_takeover; server_max_window_bits=10, permessage-deflate; client_max_window_bits")
	response := http.Header{}
	response.Set("Sec-WebSocket-Extensions", "permessage-deflate; server_no_context_takeover; client_no_context_takeover; server_max_window_bits=9; client_max_window_bits=8")

	ext, err := ValidateWebsocketExtensions(request, response)
	require.NoError(t, err)
	require.True(t, ext.IsDeflate)
	require.False(t, ext.ClientContextTakeover)
	require.False(t, ext.ServerContextTakeover)
	require.Equal(t, 8, ext.ClientMaxWindowBits)
	require.Equal(t, 9, ext.ServerMaxWindowBits)
	require.Equal(t, 8, ext.writeFlateWindowBits(false))
	require.Equal(t, 9, ext.writeFlateWindowBits(true))
	require.Equal(t, 8, ext.readFlateWindowBits(true))
	require.Equal(t, 9, ext.readFlateWindowBits(false))
}

func TestWebsocketPermessageDeflateOfferHintsDoNotBecomeNegotiatedResponseParams(t *testing.T) {
	request := http.Header{"Sec-WebSocket-Extensions": []string{"permessage-deflate; client_no_context_takeover; client_max_window_bits=9"}}
	response := http.Header{"Sec-WebSocket-Extensions": []string{"permessage-deflate"}}
	ext, err := ValidateWebsocketExtensions(request, response)
	require.NoError(t, err)
	// The response did not agree to either optional client parameter. The
	// sender may still honor its own offer hints, but the peer's receive state
	// must follow the actual response and retain the RFC defaults.
	require.True(t, ext.ClientContextTakeover)
	require.Equal(t, websocketDefaultWindowBits, ext.ClientMaxWindowBits)
}

func TestWebsocketPermessageDeflateRejectsInvalidNegotiation(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		response string
	}{
		{name: "unknown parameter", request: "permessage-deflate; x=y", response: "permessage-deflate"},
		{name: "duplicate parameter", request: "permessage-deflate; server_max_window_bits=10; server_max_window_bits=9", response: "permessage-deflate"},
		{name: "invalid window", request: "permessage-deflate; server_max_window_bits=7", response: "permessage-deflate"},
		{name: "response client window without offer", request: "permessage-deflate", response: "permessage-deflate; client_max_window_bits=9"},
		{name: "response bare client window", request: "permessage-deflate; client_max_window_bits", response: "permessage-deflate; client_max_window_bits"},
		{name: "response violates server window", request: "permessage-deflate; server_max_window_bits=9", response: "permessage-deflate; server_max_window_bits=10"},
		{name: "extension not offered", request: "", response: "x-example"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := http.Header{}
			request.Set("Sec-WebSocket-Extensions", test.request)
			response := http.Header{}
			response.Set("Sec-WebSocket-Extensions", test.response)
			_, err := ValidateWebsocketExtensions(request, response)
			require.Error(t, err)
		})
	}
}

func TestWebsocketPermessageDeflateQuotedParameters(t *testing.T) {
	request := http.Header{"Sec-WebSocket-Extensions": []string{`permessage-deflate; client_max_window_bits="9"`}}
	response := http.Header{"Sec-WebSocket-Extensions": []string{`permessage-deflate; client_max_window_bits="9"`}}
	ext, err := ValidateWebsocketExtensions(request, response)
	require.NoError(t, err)
	require.Equal(t, 9, ext.ClientMaxWindowBits)
}

func TestWebsocketPermessageDeflateOfferFormatting(t *testing.T) {
	option := WithWebsocketRFC7692FullCompression()
	config := &WebsocketClientConfig{}
	option(config)
	require.Equal(t, "permessage-deflate; client_no_context_takeover; client_max_window_bits", config.compressionOffer)
}

func TestStrictWebsocketReaderUnmasksTextBeforeValidation(t *testing.T) {
	var packet bytes.Buffer
	payload := []byte("masked text must be validated after decoding")
	require.NoError(t, func() error {
		_, err := NewFrameWriter(&packet, false).WriteDirect(true, false, TextMessage, true, bytes.Clone(payload))
		return err
	}())

	client := NewWebsocketClientIns(
		nil,
		NewFrameReader(bytes.NewReader(packet.Bytes()), false),
		NewFrameWriter(io.Discard, false),
		GetWebsocketExtensions(nil),
		WithWebsocketServerMode(true),
		WithWebsocketStrictMode(true),
	)
	defer client.cancel()

	frame, err := client.fr.ReadFrame()
	require.NoError(t, err)
	require.Equal(t, payload, frame.GetData())
}

func TestStrictWebsocketReaderRejectsInvalidLengthBeforePayloadRead(t *testing.T) {
	for _, test := range []struct {
		name   string
		packet []byte
	}{
		{
			name:   "non-minimal 16-bit length",
			packet: []byte{DEFAULT_TEXT_MESSAGE_FISRT_BYTE, TWO_BYTE_BIT, 0, 125},
		},
		{
			name:   "reserved high bit in 64-bit length",
			packet: []byte{DEFAULT_TEXT_MESSAGE_FISRT_BYTE, EIGHT_BYTE_BIT, 0x80, 0, 0, 0, 0, 0, 0, 0},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			client := NewWebsocketClientIns(
				nil,
				NewFrameReader(bytes.NewReader(test.packet), false),
				NewFrameWriter(&output, false),
				GetWebsocketExtensions(nil),
				WithWebsocketStrictMode(true),
			)
			defer client.cancel()

			_, err := client.fr.ReadFrame()
			require.ErrorIs(t, err, errInvalidWebsocketFrame)
			closeFrame, err := NewFrameReader(bytes.NewReader(output.Bytes()), false).ReadFrame()
			require.NoError(t, err)
			require.Equal(t, CloseProtocolError, closeFrame.GetCloseCode())
		})
	}
}

func TestWebsocketReassemblyPreservesMessageOpcodeAcrossControlFrame(t *testing.T) {
	var packet bytes.Buffer
	writer := NewFrameWriter(&packet, false)
	_, err := writer.WriteDirect(false, false, TextMessage, true, []byte("hello "))
	require.NoError(t, err)
	_, err = writer.WriteDirect(true, false, PingMessage, true, []byte("ping"))
	require.NoError(t, err)
	_, err = writer.WriteDirect(true, false, ContinueMessage, true, []byte("world"))
	require.NoError(t, err)

	type receivedFrame struct {
		opcode int
		data   []byte
	}
	received := make(chan receivedFrame, 2)
	conn, peer := net.Pipe()
	defer peer.Close()
	client := NewWebsocketClientIns(
		conn,
		NewFrameReader(bytes.NewReader(packet.Bytes()), false),
		NewFrameWriter(io.Discard, false),
		GetWebsocketExtensions(nil),
		WithWebsocketServerMode(true),
		WithWebsocketStrictMode(true),
		WithWebsocketDisableReassembly(false),
		WithWebsocketAllFrameHandler(func(_ *WebsocketClient, frame *Frame, data []byte, _ func()) {
			received <- receivedFrame{opcode: frame.Type(), data: bytes.Clone(data)}
		}),
	)
	client.Start()

	select {
	case frame := <-received:
		require.Equal(t, PingMessage, frame.opcode)
		require.Equal(t, []byte("ping"), frame.data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interleaved ping")
	}
	select {
	case frame := <-received:
		require.Equal(t, TextMessage, frame.opcode)
		require.Equal(t, []byte("hello world"), frame.data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reassembled text message")
	}
	client.Wait()
}

func TestCompressedFragmentsAreReassembledWhenRawReassemblyIsDisabled(t *testing.T) {
	header := http.Header{}
	header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
	ext := GetWebsocketExtensions(header)

	var packet bytes.Buffer
	compressingClient := NewWebsocketClientIns(
		nil,
		NewFrameReader(bytes.NewReader(nil), true),
		NewFrameWriter(&packet, true),
		ext,
	)
	defer compressingClient.cancel()
	payload := []byte(strings.Repeat("compressed-fragment-message-", 256))
	require.NoError(t, compressingClient.WriteText(payload))

	type receivedFrame struct {
		opcode int
		data   []byte
	}
	received := make(chan receivedFrame, 2)
	conn, peer := net.Pipe()
	defer peer.Close()
	receivingServer := NewWebsocketClientIns(
		conn,
		NewFrameReader(bytes.NewReader(packet.Bytes()), true),
		NewFrameWriter(io.Discard, true),
		ext,
		WithWebsocketServerMode(true),
		WithWebsocketStrictMode(true),
		WithWebsocketDisableReassembly(true),
		WithWebsocketAllFrameHandler(func(_ *WebsocketClient, frame *Frame, data []byte, _ func()) {
			received <- receivedFrame{opcode: frame.Type(), data: bytes.Clone(data)}
		}),
	)
	receivingServer.Start()

	select {
	case frame := <-received:
		require.Equal(t, TextMessage, frame.opcode)
		require.Equal(t, payload, frame.data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for compressed message")
	}
	select {
	case frame := <-received:
		t.Fatalf("compressed fragments produced an extra callback: opcode=%d size=%d", frame.opcode, len(frame.data))
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWebsocketCompressionUsesNegotiatedMinimumWindow(t *testing.T) {
	header := http.Header{}
	header.Set("Sec-WebSocket-Extensions", "permessage-deflate; client_max_window_bits=8; server_max_window_bits=8")
	ext := GetWebsocketExtensions(header)
	payload := []byte(strings.Repeat("eight-bit-window-", 512))

	for _, test := range []struct {
		name             string
		senderServerMode bool
	}{
		{name: "client compressor"},
		{name: "server compressor", senderServerMode: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var packet bytes.Buffer
			sender := NewWebsocketClientIns(
				nil,
				NewFrameReader(bytes.NewReader(nil), true),
				NewFrameWriter(&packet, true),
				ext,
				WithWebsocketServerMode(test.senderServerMode),
			)
			defer sender.cancel()
			require.Equal(t, 8, sender.writeFlateWindowBits())
			require.NoError(t, sender.WriteText(payload))

			received := make(chan []byte, 1)
			conn, peer := net.Pipe()
			defer peer.Close()
			receiver := NewWebsocketClientIns(
				conn,
				NewFrameReader(bytes.NewReader(packet.Bytes()), true),
				NewFrameWriter(io.Discard, true),
				ext,
				WithWebsocketServerMode(!test.senderServerMode),
				WithWebsocketStrictMode(true),
				WithWebsocketFromServerHandler(func(data []byte) {
					received <- bytes.Clone(data)
				}),
			)
			receiver.Start()

			select {
			case got := <-received:
				require.Equal(t, payload, got)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for minimum-window compressed message")
			}
		})
	}
}

func TestWebsocketCompressionContextTakeoverInBothDirections(t *testing.T) {
	header := http.Header{}
	header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
	ext := GetWebsocketExtensions(header)
	payload := []byte(strings.Repeat("context-takeover-history-", 512))

	for _, senderServerMode := range []bool{false, true} {
		name := "client compressor"
		if senderServerMode {
			name = "server compressor"
		}
		t.Run(name, func(t *testing.T) {
			var packet bytes.Buffer
			sender := NewWebsocketClientIns(
				nil,
				NewFrameReader(bytes.NewReader(nil), true),
				NewFrameWriter(&packet, true),
				ext,
				WithWebsocketServerMode(senderServerMode),
			)
			defer sender.cancel()
			require.True(t, sender.writeFlateContextTakeover())
			require.NoError(t, sender.WriteText(payload))
			require.NoError(t, sender.WriteText(payload))

			received := make(chan []byte, 2)
			conn, peer := net.Pipe()
			defer peer.Close()
			receiver := NewWebsocketClientIns(
				conn,
				NewFrameReader(bytes.NewReader(packet.Bytes()), true),
				NewFrameWriter(io.Discard, true),
				ext,
				WithWebsocketServerMode(!senderServerMode),
				WithWebsocketStrictMode(true),
				WithWebsocketFromServerHandler(func(data []byte) {
					received <- bytes.Clone(data)
				}),
			)
			receiver.Start()

			for i := 0; i < 2; i++ {
				select {
				case got := <-received:
					require.Equal(t, payload, got)
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for context-takeover message %d", i+1)
				}
			}
		})
	}
}

func TestIsValidUTF8(t *testing.T) {
	t.Run("valid utf8", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 0, remindSize)
	})

	t.Run("remind utf8 1", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 1, remindSize)
	})

	t.Run("remind utf8 2", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 2, remindSize)
	})

	t.Run("invalid utf8 2", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x90")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.False(t, valid)
		require.Equal(t, 2, remindSize)
	})

	t.Run("remind utf8 3", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 3, remindSize)
	})

	t.Run("valid utf8 4", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 0, remindSize)
	})

	t.Run("remind utf8 6", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80\x80\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.False(t, valid)
		require.Equal(t, 6, remindSize)
	})
}

func TestReadMaskAndDeflate(t *testing.T) {
	packet := []byte{0xc1, 0xfe, 0x1, 0x2a, 0xf1, 0x6, 0x71, 0xbd, 0xc5, 0x96, 0xcc, 0xf3, 0x32, 0x36, 0x65, 0x38, 0x2e, 0xe3, 0xbd, 0xf3, 0x24, 0xb2, 0x30, 0x19, 0x88, 0x13, 0xf5, 0x91, 0x36, 0xbf, 0x18, 0x11, 0xcb, 0x88, 0xc0, 0xa0, 0x5b, 0x84, 0x29, 0xfd, 0x93, 0x63, 0x37, 0x8d, 0x41, 0xd6, 0x68, 0x79, 0xb1, 0x47, 0x5b, 0xf2, 0x22, 0x52, 0x93, 0x90, 0xa1, 0x4e, 0xca, 0xc0, 0x4b, 0x41, 0xcb, 0x5a, 0x16, 0x1c, 0xe4, 0x4f, 0x50, 0x77, 0x5a, 0xd9, 0x96, 0xe1, 0x7d, 0xaa, 0xdc, 0xf3, 0xf9, 0x60, 0x7, 0xa3, 0xa, 0x60, 0x73, 0xc7, 0x9b, 0xcb, 0x5f, 0xa, 0x56, 0x9e, 0x83, 0x4b, 0xb9, 0xdf, 0x77, 0x40, 0x3b, 0x5c, 0xa4, 0x65, 0xe1, 0xbc, 0xfd, 0xcc, 0x96, 0xba, 0x63, 0x93, 0x41, 0x3b, 0xc5, 0x34, 0x5d, 0x94, 0xf6, 0x85, 0x27, 0x84, 0xc6, 0xa4, 0x57, 0xd9, 0x29, 0x79, 0xcf, 0xca, 0x3e, 0x19, 0xac, 0x5b, 0x47, 0x7d, 0x8b, 0x64, 0x8c, 0xab, 0xe, 0x6a, 0x6, 0x66, 0x59, 0x17, 0x52, 0xa, 0xf7, 0x7b, 0x3c, 0xff, 0xf3, 0xc7, 0x96, 0x59, 0xe0, 0x4, 0x5b, 0xce, 0x35, 0xe7, 0x14, 0x48, 0xc9, 0xe8, 0x15, 0x85, 0x59, 0xf6, 0xc1, 0xb2, 0xef, 0xa1, 0xa6, 0x3, 0x43, 0xb9, 0x5b, 0x6b, 0xe5, 0x31, 0x12, 0x3e, 0x3c, 0x36, 0x62, 0x2e, 0xef, 0x5b, 0xb4, 0x92, 0xe5, 0x9f, 0x78, 0x54, 0x28, 0xf3, 0x52, 0x62, 0x4b, 0xf6, 0x97, 0x95, 0x45, 0x2c, 0x83, 0x50, 0x7f, 0x14, 0xf3, 0xa9, 0xea, 0x5f, 0x11, 0xe5, 0xb9, 0x57, 0x24, 0x7, 0xea, 0x3, 0xe1, 0xbc, 0xbc, 0xe0, 0x6, 0xf0, 0xc1, 0xc2, 0xd, 0x0, 0x32, 0xd5, 0x5d, 0x94, 0xf2, 0x67, 0x6b, 0x5c, 0x75, 0xe4, 0x80, 0x41, 0xcf, 0x68, 0x90, 0xbe, 0x84, 0xd2, 0xb, 0x78, 0x74, 0x2c, 0x4, 0x42, 0x7a, 0x53, 0x93, 0x36, 0x79, 0x5e, 0x66, 0xe2, 0x89, 0xec, 0x1d, 0x9d, 0xa0, 0x18, 0x63, 0x32, 0x15, 0x8, 0x3, 0x79, 0x55, 0xe3, 0xda, 0xa5, 0xc9, 0xd, 0x67, 0x4d, 0xd0, 0x88, 0xc3, 0xd5, 0x1b, 0x70, 0xd1, 0xb3, 0x53, 0xae, 0x49, 0xb4, 0xb9, 0xad, 0xbe, 0x48, 0x36, 0x5e, 0x20, 0x7e, 0x65, 0x5e, 0x17, 0x9}
	isDeflate := true
	payloadRaw := `{"history":[],"query":"你好","plugin_enable":1,"occasion":"","isbn":"","channel":"web","lib_name":"深圳市图书馆","dh_name":"","org_key":"shenzhen-library-staff","user_id":"temp-401188d5-13bd-4fa4-8cf3-43949284cc9f","chat_mode":"","reply":"","role":"布小智","topic":"","unmatch_result":"","model":"deepseek_r1","answer_model":"","device_id":"pc","is_mini_app_call":null,"client_ip":"127.0.0.1"}`
	reader := bufio.NewReader(bytes.NewBuffer(packet))
	writer := bufio.NewWriter(os.Stdout)
	clientFrameReader := NewFrameReaderFromBufio(reader, isDeflate)
	clientFrameWriter := NewFrameWriterFromBufio(writer, isDeflate)
	client := NewWebsocketClientIns(
		nil,
		clientFrameReader,
		clientFrameWriter,
		GetWebsocketExtensions(nil),
		WithWebsocketDisableReassembly(false),
		WithWebsocketCompress(true),
	)

	frame, err := client.fr.ReadFrame()
	if err != nil {
		return
	}
	require.Equal(t, TextMessage, frame.messageType)
	require.Equal(t, payloadRaw, string(frame.payload))
}
