//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SaveFuzzerConfig(ctx context.Context, req *ypb.SaveFuzzerConfigRequest) (*ypb.DbOperateMessage, error) {
	if req.Data == nil {
		return nil, utils.Error("empty params")
	}
	var errs error
	msg := &ypb.DbOperateMessage{
		TableName:    "WebFuzzerConfig",
		Operation:    DbOperationCreateOrUpdate,
		ExtraMessage: "CreateOrUpdate web fuzzer config with pageId",
	}
	for _, v := range req.Data {
		item := &schema.WebFuzzerConfig{
			PageId: v.PageId,
			Type:   v.Type,
			Config: v.Config,
		}
		count,err := yakit.CreateOrUpdateWebFuzzerConfig(s.GetProjectDatabase(), item)
		msg.EffectRows +=count
		errs = utils.JoinErrors(errs, err)
	}
	return msg, errs
}

func (s *Server) QueryFuzzerConfig(ctx context.Context, params *ypb.QueryFuzzerConfigRequest) (*ypb.QueryFuzzerConfigResponse, error) {
	var res []*ypb.FuzzerConfig
	fuzzerConfig, err := yakit.QueryWebFuzzerConfig(s.GetProjectDatabase(), params)
 	if err != nil {
		return nil, err
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
	count, err := yakit.DeleteWebFuzzerConfig(s.GetProjectDatabase(), req.GetPageId(), req.GetDeleteAll())
	if err != nil {
		return nil, err
	}
	msg := &ypb.DbOperateMessage{
		TableName:    "web_fuzzer_config",
		Operation:    DbOperationDelete,
		EffectRows:   count,
		ExtraMessage: "delete web fuzzer config with pageId",
	}
	return msg, nil
}
