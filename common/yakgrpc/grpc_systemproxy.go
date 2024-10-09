package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetSystemProxy(ctx context.Context, req *ypb.Empty) (*ypb.GetSystemProxyResult, error) {
	p, err := netx.GetSystemProxy()
	if err != nil {
		return nil, err
	}
	return &ypb.GetSystemProxyResult{
		CurrentProxy: p.DefaultServer,
		Enable:       p.Enabled,
	}, nil
}

func (s *Server) SetSystemProxy(ctx context.Context, req *ypb.SetSystemProxyRequest) (*ypb.Empty, error) {
	if !req.GetEnable() {
		err := netx.SetSystemProxy(netx.SystemProxySetting{
			Enabled:       false,
			DefaultServer: "",
		})
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	err := netx.SetSystemProxy(netx.SystemProxySetting{
		Enabled:       true,
		DefaultServer: req.GetHttpProxy(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
