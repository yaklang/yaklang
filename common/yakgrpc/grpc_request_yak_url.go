package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) RequestYakURL(ctx context.Context, req *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var err error
	if req.GetUrl().GetFromRaw() != "" {
		req.Url, err = yakurl.LoadFromRaw(req.GetUrl().GetFromRaw())
		if err != nil {
			return nil, utils.Error(err)
		}
	}

	action := yakurl.DefaultActionFactory{}.CreateAction(strings.ToLower(req.GetUrl().GetSchema()))
	if action == nil {
		return nil, utils.Errorf("unsupported schema: %s", req.GetUrl().GetSchema())
	}
	switch ret := strings.ToUpper(req.GetMethod()); ret {
	case "GET":
		return action.Get(req)
	case "POST":
		return action.Post(req)
	default:
		return nil, utils.Errorf("not implemented method: %v", ret)
	}

}
