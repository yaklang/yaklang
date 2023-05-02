package yakgrpc

import (
	"context"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"net/url"
	"time"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) RegisterFacadesHTTP(ctx context.Context, req *ypb.RegisterFacadesHTTPRequest) (*ypb.RegisterFacadesHTTPResponse, error) {
	if s.reverseServer == nil {
		return nil, utils.Error("reverse server is nil! your yaklang facades cannot be found in system.")
	}

	host := s.reverseServer.Host
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	addr := utils.HostPort(host, s.reverseServer.Port)

	if req.GetHTTPFlowID() > 0 {
		flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), req.GetHTTPFlowID())
		if err != nil {
			return nil, err
		}
		flowGrpc, err := flow.ToGRPCModelFull()
		if err != nil {
			return nil, err
		}
		urlIns, err := url.Parse(flowGrpc.Url)
		if err != nil {
			return nil, err
		}
		if urlIns.Scheme == "ws" {
			urlIns.Scheme = "http"
		}
		if urlIns.Scheme == "wss" {
			urlIns.Scheme = "https"
		}
		urlIns.Scheme = "http"
		pattern := urlIns.RequestURI()
		s.reverseServer.SetRawResourceEx(pattern, flowGrpc.Response, true)
		go func() {
			select {
			case <-time.After(10 * time.Minute):
				s.reverseServer.RemoveHTTPResource(pattern)
			}
		}()
		urlIns.Host = addr
		return &ypb.RegisterFacadesHTTPResponse{
			FacadesUrl: urlIns.String(),
		}, nil
	}

	if req.GetHTTPResponse() == nil {
		return nil, utils.Error("http response empty")
	}

	urlStr := `http://` + addr + "/"
	if req.GetUrl() != "" {
		urlStr = req.GetUrl()
	}
	uid := uuid.NewV4().String()
	path := fmt.Sprintf("/%v", uid)
	urlIns, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if urlIns.Scheme == "ws" {
		urlIns.Scheme = "http"
	}
	if urlIns.Scheme == "wss" {
		urlIns.Scheme = "https"
	}
	if urlIns != nil {
		urlIns.Path = path
	}
	urlIns.Scheme = "http"
	urlIns.Host = addr
	pattern := path
	s.reverseServer.SetRawResourceEx(pattern, req.GetHTTPResponse(), true)
	go func() {
		select {
		case <-time.After(10 * time.Minute):
			s.reverseServer.RemoveHTTPResource(pattern)
		}
	}()
	return &ypb.RegisterFacadesHTTPResponse{
		FacadesUrl: urlIns.String(),
	}, nil
}
