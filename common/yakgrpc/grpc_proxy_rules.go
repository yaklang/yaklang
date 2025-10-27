package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetGlobalProxyRulesConfig(ctx context.Context, _ *ypb.Empty) (*ypb.GlobalProxyRulesConfig, error) {
	config, err := yakit.GetGlobalProxyRulesConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return &ypb.GlobalProxyRulesConfig{}, nil
	}
	return config, nil
}

func (s *Server) SetGlobalProxyRulesConfig(ctx context.Context, req *ypb.SetGlobalProxyRulesConfigRequest) (*ypb.Empty, error) {
	_, err := yakit.SetGlobalProxyRulesConfig(req.GetConfig())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
