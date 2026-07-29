package minimartian

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestTransferFixedResponsePacket(t *testing.T) {
	t.Run("transfers_owned_packet", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		response := &lowhttp.LowhttpResponse{RawPacket: packet, ResponsePacketFixed: true}

		transferFixedResponsePacket(req, response)

		stored := httpctx.GetFixedResponseBytes(req)
		if len(stored) == 0 || &packet[0] != &stored[0] {
			t.Fatal("fixed response packet was not transferred with owned storage")
		}
		if response.RawPacket != nil || response.ResponsePacketFixed {
			t.Fatal("source response retained transferred packet ownership")
		}
	})

	t.Run("ignores_unproven_packet", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		response := &lowhttp.LowhttpResponse{RawPacket: packet}

		transferFixedResponsePacket(req, response)

		if len(httpctx.GetFixedResponseBytes(req)) != 0 {
			t.Fatal("unproven response packet was transferred")
		}
		if len(response.RawPacket) == 0 {
			t.Fatal("unproven response packet ownership changed")
		}
	})

	t.Run("ignores_packet_after_response_modified", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetResponseModified(req, "test")
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		response := &lowhttp.LowhttpResponse{RawPacket: packet, ResponsePacketFixed: true}

		transferFixedResponsePacket(req, response)

		if len(httpctx.GetFixedResponseBytes(req)) != 0 {
			t.Fatal("fixed response packet was transferred after modification")
		}
		if len(response.RawPacket) == 0 || !response.ResponsePacketFixed {
			t.Fatal("ignored packet ownership changed")
		}
	})
}
