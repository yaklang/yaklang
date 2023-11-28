package yakgrpc

import (
	"context"
	_ "embed"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	SPACE_ENGINE_ZOOMEYE = "zoomeye"

	SPACE_ENGINE_STATUS_NORMAL = "normal"
)

func (s *Server) GetSpaceEngineStatus(ctx context.Context, req *ypb.GetSpaceEngineStatusRequest) (*ypb.SpaceEngineStatus, error) {
	switch req.GetType() {
	case SPACE_ENGINE_ZOOMEYE:
		result, err := zoomeye.ZoomeyeUserProfile(consts.GetThirdPartyApplicationConfig("zoomeye").APIKey)
		if err != nil {
			return nil, err
		}
		// res := result.Get(`resources`)
		quota := result.Get("quota_info")
		remain := quota.Get("remain_free_quota").Int() + quota.Get("remain_pay_quota").Int()
		status := &ypb.SpaceEngineStatus{
			Type:   SPACE_ENGINE_ZOOMEYE,
			Status: SPACE_ENGINE_STATUS_NORMAL,
			Info:   "ZoomEye额度按月刷新",
			Raw:    []byte(result.Raw),
			Remain: remain,
		}
		return status, nil
	default:
		return nil, utils.Errorf("invalid type: %v", req.GetType())
	}
}

//go:embed grpc_space_engine.yak
var spaceEngineExecCode string

func (s *Server) FetchPortAssetFromSpaceEngine(req *ypb.FetchPortAssetFromSpaceEngineRequest, stream ypb.Yak_FetchPortAssetFromSpaceEngineServer) error {
	engine := yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(stream.Send))
	runtimeId := uuid.NewV4().String()
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVar("FILTER", req.GetFilter())
		engine.SetVar("SCAN_VERIFY", req.GetScanBeforeSave())
		engine.SetVar("TOTAL_PAGE", req.GetMaxPage())
		engine.SetVar("ENGINE_TYPE", req.GetType())
		engine.SetVar("CONCURRENT", req.GetConcurrent())
		yak.BindYakitPluginContextToEngine(engine, &yak.YakitPluginContext{
			PluginName: "space-engine",
			RuntimeId:  runtimeId,
			Proxy:      req.GetProxy(),
		})
		return nil
	})
	return engine.ExecuteWithContext(stream.Context(), spaceEngineExecCode)
}
