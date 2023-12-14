package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	case "website":
		switch ret := strings.ToUpper(req.GetMethod()); ret {
		case "GET":
			return yakurl.GetWebsiteViewerAction().Get(req)
		default:
			return nil, utils.Errorf("not implemented method: %v", ret)
		}
	default:
		return nil, utils.Errorf("unsupported schema: %s", req.GetUrl().GetSchema())
	}
}

func (s *Server) RequestYakURLs(ctx context.Context, req *ypb.RequestYakURLsParams) (*ypb.RequestYakURLResponse, error) {
	method := req.GetMethod()
	page, pageSize := req.GetPage(), req.GetPageSize()

	total := int64(0)
	resources := make([]*ypb.YakURLResource, 0)
	for _, url := range req.GetUrls() {
		res, err := s.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Url:      url,
			Method:   method,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			return nil, utils.Error(err)
		}
		resources = append(resources, res.GetResources()...)
		total += res.GetTotal()
	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  1000,
		Resources: resources,
		Total:     total,
	}, nil
}
