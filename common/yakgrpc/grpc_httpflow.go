package yakgrpc

import (
	"context"
	"strings"
	"time"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
	"yaklang.io/yaklang/common/utils/lowhttp"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) DeleteHTTPFlows(ctx context.Context, r *ypb.DeleteHTTPFlowRequest) (*ypb.Empty, error) {
	var websocketHash []string
	db := yakit.QueryWebsocketFlowsByHTTPFlowHash(s.GetProjectDatabase(), r)
	res := yakit.YieldHTTPFlows(db, ctx)
	for v := range res {
		if v.WebsocketHash != "" {
			websocketHash = append(websocketHash, v.WebsocketHash)
		}
	}
	for _, v := range funk.ChunkStrings(websocketHash, 100) {
		err := yakit.DeleteWebsocketFlowsByHTTPFlowHash(s.GetProjectDatabase(), v)
		log.Error(err)
	}

	err := yakit.DeleteHTTPFlow(s.GetProjectDatabase(), r)
	if err != nil {
		log.Error(err)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetHTTPFlowByHash(_ context.Context, r *ypb.GetHTTPFlowByHashRequest) (*ypb.HTTPFlow, error) {
	flow, err := yakit.GetHTTPFlowByHash(s.GetProjectDatabase(), r.GetHash())
	if err != nil {
		return nil, err
	}
	return flow.ToGRPCModelFull()
}

func (s *Server) GetHTTPFlowById(_ context.Context, r *ypb.GetHTTPFlowByIdRequest) (*ypb.HTTPFlow, error) {
	flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), r.GetId())
	if err != nil {
		return nil, err
	}
	return flow.ToGRPCModelFull()
}

func (s *Server) QueryHTTPFlowByIds(_ context.Context, r *ypb.GetHTTPFlowByIdsRequest) (*ypb.HTTPFlows, error) {
	db := s.GetProjectDatabase()
	var full []*ypb.HTTPFlow
	for _, group := range funk.ChunkInt64s(r.Ids, 10) {
		var g []*yakit.HTTPFlow
		if resultHandler := bizhelper.QueryIntegerInArrayInt64(db, "id", group).Find(&g); resultHandler.Error != nil {
			continue
		}
		for _, flow := range g {
			r, _ := flow.ToGRPCModel()
			if r != nil {
				full = append(full, r)
			}
		}
	}
	return &ypb.HTTPFlows{Data: full}, nil
}

func (s *Server) QueryHTTPFlows(ctx context.Context, req *ypb.QueryHTTPFlowRequest) (*ypb.QueryHTTPFlowResponse, error) {
	paging, data, err := yakit.QueryHTTPFlow(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	utils.Debug(func() {
		log.Infof("start to convert httpflow: %s", time.Now())
	})
	var res []*ypb.HTTPFlow
	for _, r := range data {
		m, err := r.ToGRPCModel()
		if err != nil {
			return nil, utils.Errorf("cannot convert httpflow failed: %s", err)
		}
		res = append(res, m)
	}
	utils.Debug(func() {
		log.Infof("finished converting httpflow: %s", time.Now())
	})

	return &ypb.QueryHTTPFlowResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) ConvertFuzzerResponseToHTTPFlow(ctx context.Context, in *ypb.FuzzerResponse) (*ypb.HTTPFlow, error) {
	flow, err := yakit.FuzzerResponseToHTTPFlow(s.GetProjectDatabase(), in)
	if err != nil {
		return nil, err
	}
	return flow.ToGRPCModelFull()
}

func (s *Server) SetTagForHTTPFlow(ctx context.Context, req *ypb.SetTagForHTTPFlowRequest) (*ypb.Empty, error) {
	if len(req.GetCheckTags()) > 0 {
		for _, i := range req.GetCheckTags() {
			err := s.SaveSetTagForHTTPFlow(i.GetId(), i.GetHash(), i.GetTags())
			if err != nil {
				return nil, err
			}
		}
	} else {
		err := s.SaveSetTagForHTTPFlow(req.GetId(), req.GetHash(), req.GetTags())
		if err != nil {
			return nil, err
		}
	}
	return &ypb.Empty{}, nil
}

func (s *Server) SaveSetTagForHTTPFlow(id int64, hash string, tags []string) error {
	flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), id)
	if flow == nil {
		flow, err = yakit.GetHTTPFlowByHash(s.GetProjectDatabase(), hash)
	}
	if err != nil {
		return err
	}
	//flow.AddTag(tags...)
	extLen := len(tags)
	tagsData := make([]string, extLen)
	if extLen > 0 {
		for i := 0; i < extLen; i++ {
			tagsData[i] = tags[i]
		}
	}
	flow.Tags = strings.Join(utils.RemoveRepeatStringSlice(tagsData), "|")
	if db := s.GetProjectDatabase().Save(flow); db.Error != nil {
		return db.Error
	}
	return nil
}

func (s *Server) QueryHTTPFlowsIds(ctx context.Context, req *ypb.QueryHTTPFlowsIdsRequest) (*ypb.QueryHTTPFlowsIdsResponse, error) {
	if len(req.GetIncludeInWhere()) == 0 {
		return nil, utils.Errorf("IncludeInWhere is empty")
	}
	db := s.GetProjectDatabase()
	db = bizhelper.ExactQueryString(db, "source_type", req.SourceType)
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"url", "id",
	}, req.GetIncludeInWhere(), false)

	data := yakit.YieldHTTPFlows(db, context.Background())

	var res []*ypb.HTTPFlow
	for k := range data {
		res = append(res, &ypb.HTTPFlow{
			Id: uint64(k.ID),
		})

	}
	return &ypb.QueryHTTPFlowsIdsResponse{
		Data: res,
	}, nil
}

func (s *Server) HTTPFlowsFieldGroup(ctx context.Context, req *ypb.HTTPFlowsFieldGroupRequest) (*ypb.HTTPFlowsFieldGroupResponse, error) {
	tags, err := yakit.HTTPFlowTags(req.RefreshRequest)
	statusCode, err := yakit.HTTPFlowStatusCode(req.RefreshRequest)
	var tagsCode ypb.HTTPFlowsFieldGroupResponse

	if tags == nil && statusCode == nil {
		return nil, err
	}
	for _, v := range tags {
		tagsCode.Tags = append(tagsCode.Tags, &ypb.TagsCode{
			Value: v.Value,
			Total: int32(v.Count),
		})
	}

	for _, v := range statusCode {
		tagsCode.StatusCode = append(tagsCode.StatusCode, &ypb.TagsCode{
			Value: v.Value,
			Total: int32(v.Count),
		})
	}

	return &tagsCode, nil
}

func (s *Server) GetHTTPPacketBody(ctx context.Context, req *ypb.GetHTTPPacketBodyRequest) (*ypb.Bytes, error) {
	if req.GetPacketRaw() != nil {
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req.GetPacketRaw())
		if body == nil {
			return nil, utils.Error("empty body from packet raw")
		}
		return &ypb.Bytes{Raw: body}, nil
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(req.GetPacket()))
	if body == nil {
		return nil, utils.Error("empty body")
	}

	return &ypb.Bytes{Raw: body}, nil
}

func (s *Server) GetRequestBodyByHTTPFlowID(ctx context.Context, req *ypb.DownloadBodyByHTTPFlowIDRequest) (*ypb.Bytes, error) {
	flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), req.GetId())
	if err != nil {
		return nil, err
	}
	flowIns, err := flow.ToGRPCModelFull()
	if err != nil {
		return nil, err
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(flowIns.GetRequest())
	if body == nil {
		return nil, utils.Error("empty body")
	}
	return &ypb.Bytes{Raw: body}, nil
}

func (s *Server) GetResponseBodyByHTTPFlowID(ctx context.Context, req *ypb.DownloadBodyByHTTPFlowIDRequest) (*ypb.Bytes, error) {
	flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), req.GetId())
	if err != nil {
		return nil, err
	}
	flowIns, err := flow.ToGRPCModelFull()
	if err != nil {
		return nil, err
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(flowIns.GetResponse())
	if body == nil {
		return nil, utils.Error("empty body")
	}
	return &ypb.Bytes{Raw: body}, nil
}
