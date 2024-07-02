package yakgrpc

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) DeleteHTTPFlows(ctx context.Context, r *ypb.DeleteHTTPFlowRequest) (*ypb.Empty, error) {
	db := s.GetProjectDatabase()
	if !r.GetDeleteAll() {
		var (
			websocketHash []string
			httpFlowsHash []string
		)

		db = yakit.QueryWebsocketFlowsByHTTPFlowHash(db, r)
		db = db.Select([]string{"websocket_hash", "hash"}) //  just select websocket_hash and hash
		res := yakit.YieldHTTPFlows(db, ctx)
		for v := range res {
			if v.WebsocketHash != "" {
				websocketHash = append(websocketHash, v.WebsocketHash)
			}
			httpFlowsHash = append(httpFlowsHash, v.Hash)
		}
		err := utils.GormTransaction(s.GetProjectDatabase(), func(tx *gorm.DB) error {
			for _, hash := range httpFlowsHash {
				err := yakit.DeleteWebsocketFlowsByHTTPFlowHash(tx, hash)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Error(err)
		}
		err = utils.GormTransaction(s.GetProjectDatabase(), func(tx *gorm.DB) error {
			for _, hash := range httpFlowsHash {
				err := yakit.DeleteExtractedDataByTraceId(tx, hash)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Error(err)
		}
	} else {
		yakit.DropWebsocketFlowTable(db)
		yakit.DropExtractedDataTable(db)
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
	return model.ToHTTPFlowGRPCModelFull(flow)
}

func (s *Server) GetHTTPFlowById(_ context.Context, r *ypb.GetHTTPFlowByIdRequest) (*ypb.HTTPFlow, error) {
	flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), r.GetId())
	if err != nil {
		return nil, err
	}
	return model.ToHTTPFlowGRPCModelFull(flow)
}

func (s *Server) GetHTTPFlowByIds(_ context.Context, r *ypb.GetHTTPFlowByIdsRequest) (*ypb.HTTPFlows, error) {
	db := s.GetProjectDatabase().Model(&schema.HTTPFlow{})
	var full []*ypb.HTTPFlow
	var g []*schema.HTTPFlow
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", r.Ids)
	db.Find(&g)
	for _, flow := range g {
		r, _ := model.ToHTTPFlowGRPCModelFull(flow)
		if r != nil {
			full = append(full, r)
		}
	}

	// for _, group := range funk.ChunkInt64s(r.Ids, 10) {
	// 	if db = db.Where("id in (?)", group).Find(&g); db.Error != nil {
	// 		continue
	// 	}
	// 	for _, flow := range g {
	// 		r, _ := flow.ToGRPCModel(true)
	// 		if r != nil {
	// 			full = append(full, r)
	// 		}
	// 	}
	// }
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

	start := time.Now()
	var res []*ypb.HTTPFlow
	for _, r := range data {
		m, err := model.ToHTTPFlowGRPCModel(r, req.Full)
		if err != nil {
			return nil, utils.Errorf("cannot convert httpflow failed: %s", err)
		}
		res = append(res, m)
	}
	cost := time.Now().Sub(start)
	if cost.Milliseconds() > 200 {
		log.Infof("finished converting httpflow(%v) cost: %s", len(res), cost)
	}

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
	return model.ToHTTPFlowGRPCModelFull(flow)
}

func (s *Server) SetTagForHTTPFlow(ctx context.Context, req *ypb.SetTagForHTTPFlowRequest) (*ypb.Empty, error) {
	if len(req.GetCheckTags()) > 0 {
		err := utils.GormTransaction(s.GetProjectDatabase(), func(tx *gorm.DB) error {
			for _, i := range req.GetCheckTags() {
				err := s.SaveSetTagForHTTPFlow(i.GetId(), i.GetHash(), i.GetTags())
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
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
	flow, err := yakit.GetHTTPFlowByIDOrHash(s.GetProjectDatabase(), id, hash)
	if err != nil {
		return err
	}
	// flow.AddTag(tags...)
	extLen := len(tags)
	tagsData := make([]string, extLen)
	if extLen > 0 {
		for i := 0; i < extLen; i++ {
			tagsData[i] = tags[i]
		}
	}
	flow.Tags = strings.Join(utils.RemoveRepeatStringSlice(tagsData), "|")
	err = yakit.UpdateHTTPFlowTags(s.GetProjectDatabase(), flow)
	m, _ := model.ToHTTPFlowGRPCModel(flow, true)
	model.SetHTTPFlowCacheGRPCModel(flow, false, m)
	if err != nil {
		return err
	}
	return nil
}

// 似乎已弃用？没有调用
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
	// statusCode, err := yakit.HTTPFlowStatusCode(req.RefreshRequest)
	var tagsCode ypb.HTTPFlowsFieldGroupResponse

	if tags == nil {
		return nil, err
	}
	for _, v := range tags {
		tagsCode.Tags = append(tagsCode.Tags, &ypb.TagsCode{
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
	flowIns, err := model.ToHTTPFlowGRPCModelFull(flow)
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
	flowIns, err := model.ToHTTPFlowGRPCModelFull(flow)
	if err != nil {
		return nil, err
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(flowIns.GetResponse())
	if body == nil {
		return nil, utils.Error("empty body")
	}
	return &ypb.Bytes{Raw: body}, nil
}

type HTTPFlowShare struct {
	*schema.HTTPFlow
	ExtractedList         []*schema.ExtractedData
	WebsocketFlowsList    []*schema.WebsocketFlow
	ProjectGeneralStorage []*schema.ProjectGeneralStorage
}

func (s *Server) HTTPFlowsShare(ctx context.Context, req *ypb.HTTPFlowsShareRequest) (*ypb.HTTPFlowsShareResponse, error) {
	if req.GetIds() == nil || req.Module == "" || req.ExpiredTime == 0 {
		return nil, utils.Error("params empty")
	}
	var data []HTTPFlowShare

	if len(req.GetIds()) > 50 {
		return nil, utils.Error("exceed the limit")
	}
	db := s.GetProjectDatabase()
	ret := yakit.YieldHTTPFlows(bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetIds()), ctx)

	for httpFlow := range ret {
		httpFlowShareData, extractedData, websocketFlowsData, projectGeneralStorage := s.HTTPFlowsData(ctx, httpFlow)
		data = append(data, HTTPFlowShare{
			HTTPFlow:              httpFlowShareData,
			ExtractedList:         extractedData,
			WebsocketFlowsList:    websocketFlowsData,
			ProjectGeneralStorage: projectGeneralStorage,
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
	var shareData []*HTTPFlowShare
	err := json.Unmarshal([]byte(req.ShareExtractContent), &shareData)
	if err != nil {
		return nil, utils.Errorf("HTTPFlowsExtract failed: %s", err)
	}
	err = utils.GormTransaction(s.GetProjectDatabase(), func(tx *gorm.DB) error {
		for _, data := range shareData {
			shareHttpFlow := &schema.HTTPFlow{
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
				RuntimeId:          data.RuntimeId,
				FromPlugin:         data.FromPlugin,
				// IsRequestOversize:          data.IsRequestOversize,
				// IsResponseOversize:         data.IsResponseOversize,
				IsTooLargeResponse:         data.IsTooLargeResponse,
				TooLargeResponseHeaderFile: data.TooLargeResponseHeaderFile,
				TooLargeResponseBodyFile:   data.TooLargeResponseBodyFile,
			}
			var httpFlowId int64
			var httpFlow schema.HTTPFlow

			if db := tx.Where("hash = ?", shareHttpFlow.Hash).Assign(shareHttpFlow).FirstOrCreate(&httpFlow); db.Error != nil {
				return utils.Errorf("HTTPFlowsExtract CreateOrUpdateHTTPFlow failed: %s", err)
			}
			httpFlowId = int64(httpFlow.ID)

			for _, v := range data.ExtractedList {
				err = yakit.CreateOrUpdateExtractedData(tx, -1, &schema.ExtractedData{
					SourceType:  v.SourceType,
					TraceId:     v.TraceId,
					Regexp:      v.Regexp,
					RuleVerbose: v.RuleVerbose,
					Data:        v.Data,
				})
				if err != nil {
					return utils.Errorf("HTTPFlowsExtract CreateOrUpdateExtractedData failed: %s", err.Error())
				}
			}

			for _, v := range data.WebsocketFlowsList {
				if db1 := tx.Create(&schema.WebsocketFlow{
					WebsocketRequestHash: v.WebsocketRequestHash,
					FrameIndex:           v.FrameIndex,
					FromServer:           v.FromServer,
					QuotedData:           string(v.QuotedData),
					MessageType:          v.MessageType,
					Hash:                 v.Hash,
				}); db1.Error != nil {
					return utils.Errorf("WebsocketFlow failed: %s", db1.Error)
				}
			}

			for _, v := range data.ProjectGeneralStorage {
				if strings.Contains(v.Key, "_request") {
					v.Key = "_request"
				} else if strings.Contains(v.Key, "_response") {
					v.Key = "_response"
				}
				shareProjectGeneralStorage := &schema.ProjectGeneralStorage{
					Key:        strconv.Quote(strconv.FormatInt(httpFlowId, 10) + v.Key),
					Value:      v.Value,
					ExpiredAt:  v.ExpiredAt,
					ProcessEnv: v.ProcessEnv,
					Verbose:    v.Verbose,
					Group:      v.Group,
				}
				if httpFlowId > 0 {
					if db2 := tx.Where("key = ?", strconv.FormatInt(httpFlowId, 10)+v.Key).Assign(shareProjectGeneralStorage).FirstOrCreate(&schema.ProjectGeneralStorage{}); db2.Error != nil {
						return utils.Errorf("SetProjectKey failed: %s", db2.Error)
					}
				}
			}
		}
		return nil
	})

	return &ypb.Empty{}, nil
}

func (s *Server) GetHTTPFlowBare(ctx context.Context, req *ypb.HTTPFlowBareRequest) (*ypb.HTTPFlowBareResponse, error) {
	db := s.GetProjectDatabase()
	id, typ := req.GetId(), req.GetBareType()
	suffix := "_request"
	if typ == "response" {
		suffix = "_response"
	}

	if data, err := yakit.GetProjectKeyWithError(db, strconv.FormatInt(id, 10)+suffix); err != nil {
		return nil, utils.Errorf("get bare from kv failed: %s", err)
	} else {
		return &ypb.HTTPFlowBareResponse{
			Id:   id,
			Data: []byte(data),
		}, nil
	}
}

func (s *Server) ExportHTTPFlows(ctx context.Context, req *ypb.ExportHTTPFlowsRequest) (*ypb.QueryHTTPFlowResponse, error) {
	if req.FieldName == nil {
		return nil, utils.Errorf("params is empty")
	}
	paging, data, err := yakit.ExportHTTPFlow(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var res []*ypb.HTTPFlow
	for _, r := range data {
		m, err := model.ToHTTPFlowGRPCModel(r, req.ExportWhere.Full)
		if err != nil {
			return nil, utils.Errorf("cannot convert httpflow failed: %s", err)
		}
		res = append(res, m)
	}
	cost := time.Now().Sub(start)
	if cost.Milliseconds() > 200 {
		log.Infof("finished converting httpflow(%v) cost: %s", len(res), cost)
	}

	return &ypb.QueryHTTPFlowResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.ExportWhere.GetPagination().GetOrderBy(),
			Order:   req.ExportWhere.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) HTTPFlowsData(ctx context.Context, httpFlow *schema.HTTPFlow) (*schema.HTTPFlow, []*schema.ExtractedData, []*schema.WebsocketFlow, []*schema.ProjectGeneralStorage) {
	var (
		httpFlowShare         *schema.HTTPFlow
		extractedData         []*schema.ExtractedData
		websocketFlowsData    []*schema.WebsocketFlow
		projectGeneralStorage []*schema.ProjectGeneralStorage
	)
	if httpFlow.Hash != "" {
		db1 := s.GetProjectDatabase().Where("source_type == 'httpflow' and trace_id = ? ", httpFlow.Hash)
		extracted := yakit.BatchExtractedData(db1, ctx)
		for v := range extracted {
			extractedData = append(extractedData, &schema.ExtractedData{
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
			raw, _ := strconv.Unquote(string(v.QuotedData))
			if len(raw) <= 0 {
				raw = string(v.QuotedData)
			}
			websocketFlowsData = append(websocketFlowsData, &schema.WebsocketFlow{
				WebsocketRequestHash: v.WebsocketRequestHash,
				FrameIndex:           v.FrameIndex,
				FromServer:           v.FromServer,
				QuotedData:           raw,
				MessageType:          v.MessageType,
				Hash:                 v.Hash,
			})
		}
	}

	httpFlowShare = &schema.HTTPFlow{
		Model: gorm.Model{
			CreatedAt: httpFlow.CreatedAt,
			UpdatedAt: httpFlow.UpdatedAt,
		},
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
		Request:            httpFlow.Request,
		Response:           httpFlow.Response,
		GetParamsTotal:     httpFlow.GetParamsTotal,
		PostParamsTotal:    httpFlow.PostParamsTotal,
		CookieParamsTotal:  httpFlow.CookieParamsTotal,
		IPAddress:          httpFlow.IPAddress,
		RemoteAddr:         httpFlow.RemoteAddr,
		IPInteger:          httpFlow.IPInteger,
		Tags:               httpFlow.Tags,
		IsWebsocket:        httpFlow.IsWebsocket,
		WebsocketHash:      httpFlow.WebsocketHash,
		// 新加字段
		RuntimeId:  httpFlow.RuntimeId,
		FromPlugin: httpFlow.FromPlugin,
		// IsRequestOversize:          httpFlow.IsRequestOversize,
		// IsResponseOversize:         httpFlow.IsResponseOversize,
		IsTooLargeResponse:         httpFlow.IsTooLargeResponse,
		TooLargeResponseHeaderFile: httpFlow.TooLargeResponseHeaderFile,
		TooLargeResponseBodyFile:   httpFlow.TooLargeResponseBodyFile,
	}
	projectStoragesWhere := []string{strconv.Quote(strconv.FormatInt(int64(httpFlow.ID), 10) + "_response"), strconv.Quote(strconv.FormatInt(int64(httpFlow.ID), 10) + "_request")}
	projectStorages, _ := yakit.GetProjectKeyByWhere(s.GetProjectDatabase(), projectStoragesWhere)
	for _, v := range projectStorages {
		projectGeneralStorage = append(projectGeneralStorage, &schema.ProjectGeneralStorage{
			Key:        v.Key,
			Value:      v.Value,
			Group:      v.Group,
			Verbose:    v.Verbose,
			ExpiredAt:  v.ExpiredAt,
			ProcessEnv: v.ProcessEnv,
		})
	}

	return httpFlowShare, extractedData, websocketFlowsData, projectGeneralStorage
}

func (s *Server) HTTPFlowsToOnline(ctx context.Context, req *ypb.HTTPFlowsToOnlineRequest) (*ypb.Empty, error) {
	if req.Token == "" || req.ProjectName == "" {
		return nil, utils.Errorf("params empty")
	}
	limit := make(chan struct{}, 20)
	var (
		successHash []string
		wg          sync.WaitGroup
		count       = 0
		mu          sync.Mutex
	)

	db := s.GetProjectDatabase()
	db = db.Where("upload_online <> '1' ")
	ret := yakit.YieldHTTPFlows(db, context.Background())
	for httpFlow := range ret {
		wg.Add(1)

		limit <- struct{}{}

		go func(httpFlow *schema.HTTPFlow) {
			defer func() {
				<-limit
			}()
			defer wg.Done()
			httpFlowShareData, extractedData, websocketFlowsData, projectGeneralStorage := s.HTTPFlowsData(ctx, httpFlow)
			content := HTTPFlowShare{
				HTTPFlow:              httpFlowShareData,
				ExtractedList:         extractedData,
				WebsocketFlowsList:    websocketFlowsData,
				ProjectGeneralStorage: projectGeneralStorage,
			}
			data, err := json.Marshal(content)
			if err != nil {
				log.Errorf("JSON marshal error: %s", err)
				return
			}
			client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
			err = client.UploadHTTPFlowToOnline(ctx, req.Token, req.ProjectName, data)
			if err != nil {
				if strings.Contains(err.Error(), "token过期") {
					log.Errorf("httpflow to online failed: %s", err.Error())
					return
				}
				log.Errorf("httpflow to online failed: %s", err.Error())
			} else {
				mu.Lock()
				defer mu.Unlock()
				successHash = append(successHash, httpFlow.Hash)
			}
		}(httpFlow)

		count++
		if count == 20 {
			time.Sleep(1 * time.Second)
			count = 0
		}
	}
	// 等待所有协程执行完毕
	wg.Wait()
	for _, v := range funk.ChunkStrings(successHash, 100) {
		err := yakit.HTTPFlowToOnline(s.GetProjectDatabase(), v)
		if err != nil {
			log.Errorf("HTTPFlowsToOnline failed: %s", err)
		}
	}

	return &ypb.Empty{}, nil
}
