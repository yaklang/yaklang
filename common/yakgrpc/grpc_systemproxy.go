package yakgrpc

import (
	"context"
	"yaklang.io/yaklang/common/utils/systemproxy"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetSystemProxy(ctx context.Context, req *ypb.Empty) (*ypb.GetSystemProxyResult, error) {
	p, err := systemproxy.Get()
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
		err := systemproxy.Set(systemproxy.Settings{
			Enabled:       false,
			DefaultServer: "",
		})
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	err := systemproxy.Set(systemproxy.Settings{
		Enabled:       true,
		DefaultServer: req.GetHttpProxy(),
	})
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
