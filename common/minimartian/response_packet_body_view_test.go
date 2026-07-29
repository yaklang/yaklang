package minimartian

import (
	"bytes"
	"io"
	"testing"
)

func TestParseLowHTTPResponsePacketUsesImmutableBodyView(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nyak")
	bodyOffset := bytes.Index(packet, []byte("\r\n\r\n")) + 4
	rsp, err := parseLowHTTPResponsePacket(packet)
	if err != nil {
		t.Fatal(err)
	}

	packet[bodyOffset] = 'Y'
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Yak" {
		t.Fatalf("response Body lost its packet view: %q", body)
	}
}
