package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) SaveFuzzerConfig(ctx context.Context, req *ypb.SaveFuzzerConfigRequest) (*ypb.DbOperateMessage, error) {
	if req.Data == nil {
		return nil, utils.Error("empty params")
	}
	var errs []string
	msg := &ypb.DbOperateMessage{
		TableName:    "WebFuzzerConfig",
		Operation:    "CreateOrUpdate",
		EffectRows:   int64(len(req.Data)),
		ExtraMessage: fmt.Sprintf("CreateOrUpdate web fuzzer config with pageId"),
	}
	for _, v := range req.Data {
		item := &schema.WebFuzzerConfig{
			PageId: v.PageId,
			Type:   v.Type,
			Config: v.Config,
		}
		err := yakit.CreateOrUpdateWebFuzzerConfig(s.GetProjectDatabase(), item)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return msg, utils.Errorf(strings.Join(errs, ",") + "添加失败")
	}
	return msg, nil
}

func (s *Server) QueryFuzzerConfig(ctx context.Context, params *ypb.QueryFuzzerConfigRequest) (*ypb.QueryFuzzerConfigResponse, error) {
	var res []*ypb.FuzzerConfig
	fuzzerConfig, err := yakit.QueryWebFuzzerConfig(s.GetProjectDatabase(), params.GetLimit())
	if err != nil {
		return nil, utils.Errorf("empty result")
	}
	for _, v := range fuzzerConfig {
		res = append(res, &ypb.FuzzerConfig{
			PageId: v.PageId,
			Type:   v.Type,
			Config: v.Config,
		})
	}
	return &ypb.QueryFuzzerConfigResponse{Data: res}, nil
}

func (s *Server) DeleteFuzzerConfig(ctx context.Context, req *ypb.DeleteFuzzerConfigRequest) (*ypb.DbOperateMessage, error) {
	msg, err := yakit.DeleteWebFuzzerConfig(s.GetProjectDatabase(), req.GetPageId(), req.GetDeleteAll())
	if err != nil {
		return msg, err
	}
	return msg, nil
}
