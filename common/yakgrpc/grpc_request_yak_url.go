package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
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
	schema := strings.ToLower(req.GetUrl().GetSchema())
	actionService := yakurl.GetActionService()
	action := actionService.GetAction(schema)
	if action == nil {
		return nil, utils.Errorf("unsupported schema: %s", req.GetUrl().GetSchema())
	}
	switch ret := strings.ToUpper(req.GetMethod()); ret {
	case http.MethodGet:
		return action.Get(req)
	case http.MethodPost:
		return action.Post(req)
	case http.MethodPut:
		return action.Put(req)
	case http.MethodDelete:
		return action.Delete(req)
	case http.MethodHead:
		return action.Head(req)
	default:
		return nil, utils.Errorf("not implemented method: %v", ret)
	}

}
