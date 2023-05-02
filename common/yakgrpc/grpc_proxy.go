package yakgrpc

import (
	"context"
	"fmt"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

const (
	YAK_ENGINE_DEFAULT_SCAN_PROXY = "YAK_ENGINE_DEFAULT_SCAN_PROXY"
)

func (s *Server) GetEngineDefaultProxy(ctx context.Context, e *ypb.Empty) (*ypb.DefaultProxyResult, error) {
	return &ypb.DefaultProxyResult{Proxy: yakit.GetKey(s.GetProfileDatabase(), YAK_ENGINE_DEFAULT_SCAN_PROXY)}, nil
}

func (s *Server) SetEngineDefaultProxy(ctx context.Context, d *ypb.DefaultProxyResult) (*ypb.Empty, error) {
	var err = yakit.SetKey(s.GetProfileDatabase(), YAK_ENGINE_DEFAULT_SCAN_PROXY, d.GetProxy())
	if err != nil {
		return nil, utils.Errorf("设置引擎默认扫描代理失败")
	}
	return &ypb.Empty{}, nil
}

func GetScanProxyEnviron() string {
	return fmt.Sprintf("YAK_PROXY=%v", yakit.GetKey(consts.GetGormProfileDatabase(), YAK_ENGINE_DEFAULT_SCAN_PROXY))
}
