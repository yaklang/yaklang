package yakgrpc

import (
	"context"
	"net/url"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReverseServerLazyStart(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}

	if s.reverseServer != nil {
		t.Fatal("reverse server should not start by default")
	}

	rsp, err := s.GetGlobalReverseServer(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if rsp.GetLocalReversePort() <= 0 {
		t.Fatalf("local reverse port was not assigned: %d", rsp.GetLocalReversePort())
	}
	if s.reverseServer == nil {
		t.Fatal("GetGlobalReverseServer should start reverse server on demand")
	}

	registerRsp, err := s.RegisterFacadesHTTP(context.Background(), &ypb.RegisterFacadesHTTPRequest{
		HTTPResponse: []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if registerRsp.GetFacadesUrl() == "" {
		t.Fatal("empty facade url")
	}
	parsed, err := url.Parse(registerRsp.GetFacadesUrl())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Port() == "" {
		t.Fatal("facade url does not contain a port")
	}
	if s.reverseServer == nil {
		t.Fatal("reverse server should start after RegisterFacadesHTTP")
	}
}
