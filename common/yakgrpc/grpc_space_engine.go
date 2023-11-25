package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
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
		res := result.Get(`resources`)
		quota := res.Get("quota_info")
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
