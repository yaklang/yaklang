package lowhttp

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestLowhttpBorrowFixedRequestPacketOwnership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "2")
		_, _ = writer.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	host, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name        string
		borrow      bool
		wantAliases bool
	}{
		{name: "default-owned"},
		{name: "immutable-internal-borrow", borrow: true, wantAliases: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := []byte(fmt.Sprintf(
				"GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n",
				server.Listener.Addr().String(),
			))
			response, err := HTTPWithoutRedirect(
				WithHost(host),
				WithPort(port),
				WithPacketBytes(packet),
				WithBorrowFixedRequestPacket(test.borrow),
				WithTimeout(2*time.Second),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(response.RawRequest) == 0 {
				t.Fatal("missing raw request")
			}
			aliases := &response.RawRequest[0] == &packet[0]
			if aliases != test.wantAliases {
				t.Fatalf("RawRequest aliases input = %v, want %v", aliases, test.wantAliases)
			}
		})
	}
}
