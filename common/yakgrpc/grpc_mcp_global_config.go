package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetMCPGlobalConfig(ctx context.Context, _ *ypb.Empty) (*ypb.MCPGlobalConfig, error) {
	cfg, err := yakit.GetMCPGlobalConfig(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return yakit.CatalogMCPGlobalConfig(), nil
	}
	return cfg, nil
}

func (s *Server) SetMCPGlobalConfig(ctx context.Context, cfg *ypb.MCPGlobalConfig) (*ypb.Empty, error) {
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	if err := mcp.ValidateToolSetNames(cfg.GetDefaultToolSets()); err != nil {
		return nil, err
	}
	if err := validateMCPResourceSetNames(cfg.GetDefaultResourceSets()); err != nil {
		return nil, err
	}
	if _, err := yakit.SetMCPGlobalConfig(s.GetProfileDatabase(), cfg); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) ResetMCPGlobalConfig(ctx context.Context, _ *ypb.Empty) (*ypb.MCPGlobalConfig, error) {
	return yakit.ResetMCPGlobalConfig(s.GetProfileDatabase())
}

func validateMCPResourceSetNames(names []string) error {
	registered := mcp.GlobalResourceSetList()
	for _, name := range names {
		if name == "" {
			continue
		}
		found := false
		for _, item := range registered {
			if item == name {
				found = true
				break
			}
		}
		if !found {
			return utils.Errorf("undefined resource set: %s", name)
		}
	}
	return nil
}
