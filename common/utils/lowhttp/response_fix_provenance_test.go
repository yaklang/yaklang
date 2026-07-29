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

func TestLowhttpResponsePacketFixedProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write([]byte("response-body"))
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
	request := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", server.Listener.Addr().String()))

	tests := []struct {
		name      string
		extraOpts []LowhttpOpt
		wantFixed bool
	}{
		{name: "regular", wantFixed: true},
		{name: "no_fix", extraOpts: []LowhttpOpt{WithNoFixContentLength(true), WithNoReadMultiResponse(true)}},
		{name: "no_body_buffer", extraOpts: []LowhttpOpt{WithNoBodyBuffer(true)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []LowhttpOpt{
				WithHost(host),
				WithPort(port),
				WithPacketBytes(request),
				WithTimeout(2 * time.Second),
			}
			opts = append(opts, tt.extraOpts...)
			rsp, err := HTTPWithoutRedirect(opts...)
			if err != nil {
				t.Fatal(err)
			}
			if rsp.ResponsePacketFixed != tt.wantFixed {
				t.Fatalf("ResponsePacketFixed=%v, want %v", rsp.ResponsePacketFixed, tt.wantFixed)
			}
		})
	}
}
