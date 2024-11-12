package yakgrpc

import (
	"context"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetAllPluginEnv(ctx context.Context, empty *ypb.Empty) (*ypb.PluginEnvData, error) {
	env, err := yakit.GetAllPluginEnv(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.PluginEnvData{
		Env: lo.Map(env, func(i *schema.PluginEnv, _ int) *ypb.KVPair {
			return &ypb.KVPair{Key: i.Key, Value: i.Value}
		}),
	}, nil
}

func (s *Server) CreatePluginEnv(ctx context.Context, request *ypb.PluginEnvData) (*ypb.Empty, error) {
	for _, env := range request.Env {
		if err := yakit.CreatePluginEnv(s.GetProfileDatabase(), env.Key, env.Value); err != nil {
			return nil, err
		}
	}
	return &ypb.Empty{}, nil
}

func (s *Server) SetPluginEnv(ctx context.Context, request *ypb.PluginEnvData) (*ypb.Empty, error) {
	for _, env := range request.Env {
		if err := yakit.CreateOrUpdatePluginEnv(s.GetProfileDatabase(), env.Key, env.Value); err != nil {
			return nil, err
		}
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePluginEnv(ctx context.Context, request *ypb.DeletePluginEnvRequest) (*ypb.Empty, error) {
	if request.GetAll() {
		if err := yakit.DeleteAllPluginEnv(s.GetProfileDatabase(), &schema.PluginEnv{}); err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}

	if err := yakit.DeletePluginEnvByKey(s.GetProfileDatabase(), request.Key); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryPluginEnv(ctx context.Context, request *ypb.QueryPluginEnvRequest) (*ypb.PluginEnvData, error) {
	env, err := yakit.GetPluginEnvsByKey(s.GetProfileDatabase(), request.Key)
	if err != nil {
		return nil, err
	}
	return &ypb.PluginEnvData{
		Env: lo.Map(env, func(i *schema.PluginEnv, _ int) *ypb.KVPair {
			return &ypb.KVPair{Key: i.Key, Value: i.Value}
		}),
	}, nil
}
