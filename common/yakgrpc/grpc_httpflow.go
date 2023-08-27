package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"time"
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

func (s *Server) GetHTTPFlowByIds(_ context.Context, r *ypb.GetHTTPFlowByIdsRequest) (*ypb.HTTPFlows, error) {
	db := s.GetProjectDatabase()
	var full []*ypb.HTTPFlow
	for _, group := range funk.ChunkInt64s(r.Ids, 10) {
		var g []*yakit.HTTPFlow
		if db = db.Where("id in (?)", group).Find(&g); db.Error != nil {
			continue
		}
		for _, flow := range g {
			r, _ := flow.ToGRPCModel(true)
			if r != nil {
				full = append(full, r)
			}
		}
	}
	return &ypb.HTTPFlows{Data: full}, nil
}

func (s *Server) QueryHTTPFlows(ctx context.Context, req *ypb.QueryHTTPFlowRequest) (*ypb.QueryHTTPFlowResponse, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	paging, data, err := yakit.QueryHTTPFlow(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	utils.Debug(func() {
		log.Infof("start to convert httpflow: %s", time.Now())
	})
	var res []*ypb.HTTPFlow
	for _, r := range data {
		m, err := r.ToGRPCModel(req.Full)
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

func (s *Server) HTTPFlowsShare(ctx context.Context, req *ypb.HTTPFlowsShareRequest) (*ypb.HTTPFlowsShareResponse, error) {
	if req.GetIds() == nil || req.Module == "" || req.ExpiredTime == 0 {
		return nil, utils.Error("params empty")
	}

	type HTTPFlowShare struct {
		*yakit.HTTPFlow
		ExtractedList      []*yakit.ExtractedData
		WebsocketFlowsList []*yakit.WebsocketFlowShare
	}
	var (
		data               []HTTPFlowShare
		extractedData      []*yakit.ExtractedData
		websocketFlowsData []*yakit.WebsocketFlowShare
	)

	if len(req.GetIds()) > 50 {
		return nil, utils.Error("exceed the limit")
	}
	ret := yakit.YieldHTTPFlows(s.GetProjectDatabase().Where("id in (?)", req.Ids), ctx)

	for httpFlow := range ret {
		if httpFlow.Hash != "" {
			db1 := s.GetProjectDatabase().Where("source_type == 'httpflow' and trace_id = ? ", httpFlow.Hash)
			extracted := yakit.BatchExtractedData(db1, ctx)
			for v := range extracted {
				extractedData = append(extractedData, &yakit.ExtractedData{
					SourceType:  v.SourceType,
					TraceId:     v.TraceId,
					Regexp:      utils.EscapeInvalidUTF8Byte([]byte(v.Regexp)),
					RuleVerbose: utils.EscapeInvalidUTF8Byte([]byte(v.RuleVerbose)),
					Data:        utils.EscapeInvalidUTF8Byte([]byte(v.Data)),
				})
			}
		}
		if httpFlow.WebsocketHash != "" {
			db2 := bizhelper.ExactQueryString(s.GetProjectDatabase(), "websocket_request_hash", httpFlow.WebsocketHash)
			websocketFlows := yakit.BatchWebsocketFlows(db2, ctx)
			for v := range websocketFlows {
				raw, _ := strconv.Unquote(v.QuotedData)
				if len(raw) <= 0 {
					raw = v.QuotedData
				}
				websocketFlowsData = append(websocketFlowsData, &yakit.WebsocketFlowShare{
					WebsocketRequestHash: v.WebsocketRequestHash,
					FrameIndex:           v.FrameIndex,
					FromServer:           v.FromServer,
					QuotedData:           []byte(raw),
					MessageType:          v.MessageType,
					Hash:                 v.Hash,
				})
			}
		}

		httpFlowShare := &yakit.HTTPFlow{
			HiddenIndex:        httpFlow.HiddenIndex,
			NoFixContentLength: httpFlow.NoFixContentLength,
			Hash:               httpFlow.Hash,
			IsHTTPS:            httpFlow.IsHTTPS,
			Url:                httpFlow.Url,
			Path:               httpFlow.Path,
			Method:             httpFlow.Method,
			BodyLength:         httpFlow.BodyLength,
			ContentType:        httpFlow.ContentType,
			StatusCode:         httpFlow.StatusCode,
			SourceType:         httpFlow.SourceType,
			//Request:            request,
			//Response:           response,
			Request:           httpFlow.Request,
			Response:          httpFlow.Response,
			GetParamsTotal:    httpFlow.GetParamsTotal,
			PostParamsTotal:   httpFlow.PostParamsTotal,
			CookieParamsTotal: httpFlow.CookieParamsTotal,
			IPAddress:         httpFlow.IPAddress,
			RemoteAddr:        httpFlow.RemoteAddr,
			IPInteger:         httpFlow.IPInteger,
			Tags:              httpFlow.Tags,
			IsWebsocket:       httpFlow.IsWebsocket,
			WebsocketHash:     httpFlow.WebsocketHash,
		}
		data = append(data, HTTPFlowShare{
			HTTPFlow:           httpFlowShare,
			ExtractedList:      extractedData,
			WebsocketFlowsList: websocketFlowsData,
		})
	}
	shareContent, err := json.Marshal(data)
	if err != nil {
		return nil, utils.Errorf("marshal params failed: %s", err)
	}

	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	shareRes, err := client.HttpFlowShareWithToken(ctx, req.Token, req.ExpiredTime, req.Module, string(shareContent), req.Pwd, req.LimitNum)
	if err != nil {
		return nil, utils.Errorf("HTTPFlowsShare failed: %s", err)
	}
	return &ypb.HTTPFlowsShareResponse{
		ShareId:     shareRes.ShareId,
		ExtractCode: shareRes.ExtractCode,
	}, nil
}

func (s *Server) HTTPFlowsExtract(ctx context.Context, req *ypb.HTTPFlowsExtractRequest) (*ypb.Empty, error) {
	if req.ShareExtractContent == "" {
		return nil, utils.Error("params empty")
	}
	type HTTPFlowShare struct {
		*yakit.HTTPFlow
		ExtractedList      []*yakit.ExtractedData
		WebsocketFlowsList []*yakit.WebsocketFlowShare
	}
	var (
		shareData []*HTTPFlowShare
	)
	err := json.Unmarshal([]byte(req.ShareExtractContent), &shareData)

	if err != nil {
		return nil, utils.Errorf("HTTPFlowsExtract failed: %s", err)
	}
	sw := s.GetProjectDatabase().Begin()
	for _, data := range shareData {
		shareHttpFlow := &yakit.HTTPFlow{
			HiddenIndex:        data.HiddenIndex,
			NoFixContentLength: data.NoFixContentLength,
			Hash:               data.Hash,
			IsHTTPS:            data.IsHTTPS,
			Url:                data.Url,
			Path:               data.Path,
			Method:             data.Method,
			BodyLength:         data.BodyLength,
			ContentType:        data.ContentType,
			StatusCode:         data.StatusCode,
			SourceType:         data.SourceType,
			Request:            data.Request,
			Response:           data.Response,
			GetParamsTotal:     data.GetParamsTotal,
			PostParamsTotal:    data.PostParamsTotal,
			CookieParamsTotal:  data.CookieParamsTotal,
			IPAddress:          data.IPAddress,
			RemoteAddr:         data.RemoteAddr,
			IPInteger:          data.IPInteger,
			Tags:               data.Tags,
			IsWebsocket:        data.IsWebsocket,
			WebsocketHash:      data.WebsocketHash,
		}
		if shareHttpFlow != nil {
			err = yakit.CreateOrUpdateHTTPFlow(s.GetProjectDatabase(), shareHttpFlow.Hash, shareHttpFlow)
			if err != nil {
				sw.Rollback()
				return nil, utils.Errorf("HTTPFlowsExtract CreateOrUpdateHTTPFlow failed: %s", err)
			}
		}

		for _, v := range data.ExtractedList {
			err = yakit.CreateOrUpdateExtractedData(s.GetProjectDatabase(), -1, &yakit.ExtractedData{
				SourceType:  v.SourceType,
				TraceId:     v.TraceId,
				Regexp:      v.Regexp,
				RuleVerbose: v.RuleVerbose,
				Data:        v.Data,
			})
			if err != nil {
				sw.Rollback()
				return nil, utils.Errorf("HTTPFlowsExtract CreateOrUpdateExtractedData failed: %s", err.Error())
			}
		}

		for _, v := range data.WebsocketFlowsList {
			if db1 := s.GetProjectDatabase().Create(&yakit.WebsocketFlow{
				WebsocketRequestHash: v.WebsocketRequestHash,
				FrameIndex:           v.FrameIndex,
				FromServer:           v.FromServer,
				QuotedData:           string(v.QuotedData),
				MessageType:          v.MessageType,
				Hash:                 v.Hash,
			}); db1.Error != nil {
				sw.Rollback()
				return nil, utils.Errorf("HTTPFlowsExtract failed: %s", db1.Error)
			}
		}
	}
	sw.Commit()

	return &ypb.Empty{}, nil
}
