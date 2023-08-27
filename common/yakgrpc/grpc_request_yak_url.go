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

	switch strings.TrimSpace(strings.ToLower(req.GetUrl().GetSchema())) {
	case "file":
		switch ret := strings.ToUpper(req.GetMethod()); ret {
		case "GET":
			return yakurl.GetLocalFileSystemAction().Get(req)
		case "POST":
			return yakurl.GetLocalFileSystemAction().Post(req)
		default:
			return nil, utils.Errorf("not implemented method: %v", ret)
		}
	default:
		return nil, utils.Errorf("unsupported schema: %s", req.GetUrl().GetSchema())
	}

	return nil, utils.Error(`not implemented`)
}
