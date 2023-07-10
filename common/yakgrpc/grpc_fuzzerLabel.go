package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) SaveFuzzerLabel(ctx context.Context, req *ypb.SaveFuzzerLabelRequest) (*ypb.Empty, error) {
	if req.Data == nil {
		return nil, utils.Error("empty params")
	}
	var errLabel []string
	for _, v := range req.Data{
		item := &yakit.WebFuzzerLabel{
			Label:       v.Label,
			Description: v.Description,
			DefaultDescription: v.DefaultDescription,
		}
		item.Hash = item.CalcHash()
		count := yakit.QueryWebFuzzerLabelCount(s.GetProfileDatabase())
		if count >= 20 {
			return nil, utils.Errorf("已超出常用标签限制数量，请删除后再进行添加")
		}
		err := yakit.CreateOrUpdateWebFuzzerLabel(s.GetProfileDatabase(), item.Hash, item)
		if err != nil {
			errLabel = append(errLabel, item.Label)
		}
	}
	if len(errLabel) > 0 {
		return nil, utils.Errorf(strings.Join(errLabel, ",") + "添加失败")
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryFuzzerLabel(ctx context.Context, req *ypb.Empty) (*ypb.QueryFuzzerLabelResponse, error)  {
	var res []*ypb.FuzzerLabel
	fuzzerLabel, err := yakit.QueryWebFuzzerLabel(s.GetProfileDatabase())
	if err != nil {
		return nil, utils.Errorf("empty result")
	}
	for _, v := range fuzzerLabel{
		res = append(res, &ypb.FuzzerLabel{
			Id:          int64(v.ID),
			Label:       v.Label,
			Description: v.Description,
			DefaultDescription: v.DefaultDescription,
			Hash: v.Hash,
		})
	}
	return &ypb.QueryFuzzerLabelResponse{Data: res}, nil
}

func (s *Server) DeleteFuzzerLabel(ctx context.Context, req *ypb.DeleteFuzzerLabelRequest) (*ypb.Empty, error)  {
	var hash string
	if req != nil {
		hash = req.Hash
	}
	err := yakit.DeleteWebFuzzerLabel(s.GetProfileDatabase(), hash)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
