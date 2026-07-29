package crep

import (
	"bytes"
	"io"
	"testing"
)

func TestCloneAndParseHijackedResponseKeepsOneOwnedPacket(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nyak")
	bodyOffset := bytes.Index(packet, []byte("\r\n\r\n")) + 4
	ownedPacket, rsp, err := cloneAndParseHijackedResponse(packet)
	if err != nil {
		t.Fatal(err)
	}
	if &packet[0] == &ownedPacket[0] {
		t.Fatal("hijack result was not snapshotted")
	}

	packet[bodyOffset] = 'X'
	ownedPacket[bodyOffset] = 'Y'
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Yak" {
		t.Fatalf("response Body is not backed by the owned packet: %q", body)
	}
}
