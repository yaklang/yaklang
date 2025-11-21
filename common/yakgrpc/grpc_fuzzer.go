package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/davecgh/go-spew/spew"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	filter2 "github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/google/uuid"
	"github.com/saintfish/chardet"
)

var (
	_FuzzerTaskSwitchMap = new(sync.Map)
	fuzzerSessionPreFix  = "__FUZZER_SESSION__"
)

func Chardet(raw []byte) string {
	res, err := chardet.NewTextDetector().DetectBest(raw)
	if err != nil {
		return "utf-8"
	}
	return res.Charset
}

func (s *Server) ExtractUrl(ctx context.Context, req *ypb.FuzzerRequest) (*ypb.ExtractedUrl, error) {
	res, err := mutate.FuzzTagExec(req.GetRequest(), mutate.Fuzz_WithEnableDangerousTag(), mutate.Fuzz_WithResultLimit(1))
	if err != nil {
		return nil, err
	}
	var u *url.URL
	if err != nil {
		u, err = lowhttp.ExtractURLFromHTTPRequestRaw([]byte(req.Request), req.GetIsHTTPS())
		if err != nil {
			return nil, err
		}
	} else {
		render, err := lowhttp.ParseStringToHttpRequest(res[0])
		if err != nil {
			return nil, err
		}
		u, err = lowhttp.ExtractURLFromHTTPRequest(render, req.GetIsHTTPS())
		if err != nil {
			return nil, err
		}
	}

	return &ypb.ExtractedUrl{Url: u.String()}, nil
}

func (s *Server) StringFuzzer(rootCtx context.Context, req *ypb.StringFuzzerRequest) (*ypb.StringFuzzerResponse, error) {
	max := req.GetLimit()
	timeoutSeconds := req.GetTimeoutSeconds()
	ctx := rootCtx
	cancel := func() {}
	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(rootCtx, time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	var res [][]byte
	var counter int64
	opts := yak.Fuzz_WithAllHotPatch(rootCtx, req.GetHotPatchCode())
	opts = append(opts, mutate.Fuzz_WithResultHandler(func(origin string, payloads []string) bool {
		select {
		case <-ctx.Done():
			return false
		default:
			if max > 0 && counter >= max {
				return false
			}
		}
		counter++
		res = append(res, []byte(origin))
		return true
	}), mutate.Fuzz_WithEnableDangerousTag())
	mutate.FuzzTagExec(
		req.GetTemplate(),
		opts...,
	)
	return &ypb.StringFuzzerResponse{Results: res}, nil
}

func (s *Server) RedirectRequest(ctx context.Context, req *ypb.RedirectRequestParams) (*ypb.FuzzerResponse, error) {
	result := lowhttp.GetRedirectFromHTTPResponse([]byte(req.GetResponse()), false)
	if result == "" {
		return nil, utils.Error("cannot find redirect url")
	}

	isHttps := req.GetIsHttps()
	if strings.HasPrefix(result, "https://") {
		isHttps = true
	}
	if strings.HasPrefix(result, "http://") {
		isHttps = false
	}
	_ = isHttps
	newUrl := lowhttp.MergeUrlFromHTTPRequest([]byte(req.GetRequest()), result, isHttps)
	resultRequest := lowhttp.UrlToGetRequestPacket(newUrl, []byte(req.GetRequest()), isHttps, lowhttp.ExtractCookieJarFromHTTPResponse([]byte(req.GetResponse()))...)
	if resultRequest == nil {
		return nil, utils.Errorf("cannot merge request packet. redirect url: %s", newUrl)
	}
	start := time.Now()
	host, port, _ := utils.ParseStringToHostPort(newUrl)
	rspIns, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithHttps(isHttps),
		lowhttp.WithHost(host),
		lowhttp.WithPort(port),
		lowhttp.WithRequest(resultRequest),
		lowhttp.WithTimeoutFloat(req.GetPerRequestTimeoutSeconds()),
		lowhttp.WithGmTLS(req.GetIsGmTLS()),
		lowhttp.WithProxy(utils.PrettifyListFromStringSplited(req.GetProxy(), ",")...))
	if err != nil {
		return nil, err
	}
	rspRaw := rspIns.RawPacket
	// 提取响应
	extractHTTPResponseResult, err := s.ExtractHTTPResponse(ctx, &ypb.ExtractHTTPResponseParams{
		HTTPResponse: string(rspRaw),
		HTTPRequest:  string(rspIns.RawRequest),
		IsHTTPS:      isHttps,
		Extractors:   req.GetExtractors(),
	})
	var extractResults []*ypb.KVPair
	if err == nil && extractHTTPResponseResult != nil && extractHTTPResponseResult.GetValues() != nil {
		for _, value := range extractHTTPResponseResult.GetValues() {
			extractResults = append(extractResults, &ypb.KVPair{
				Key:   value.GetKey(),
				Value: utils.EscapeInvalidUTF8Byte([]byte(value.GetValue())),
			})
		}
	}
	// 匹配响应
	var httpTPLmatchersResult bool
	var hitColor []string
	if len(req.GetMatchers()) != 0 {
		httpTplMatcher := make([]*YakFuzzerMatcher, 0)
		for _, matcher := range req.GetMatchers() {
			httpTplMatcher = append(httpTplMatcher, NewHttpFlowMatcherFromGRPCModel(matcher))
		}
		mergedParams := make(map[string]interface{})
		renderedParams, err := s.RenderVariables(ctx, &ypb.RenderVariablesRequest{
			Params: funk.Map(req.GetParams(), func(i *ypb.FuzzerParamItem) *ypb.KVPair {
				return &ypb.KVPair{Key: i.GetKey(), Value: i.GetValue()}
			}).([]*ypb.KVPair),
			IsHTTPS: req.GetIsHttps(),
			IsGmTLS: req.GetIsGmTLS(),
		})
		if err != nil {
			return nil, utils.Errorf("render variables failed: %v", err)
		}
		for _, kv := range renderedParams.GetResults() {
			mergedParams[kv.GetKey()] = kv.GetValue()
		}

		matcherParams := utils.CopyMapInterface(mergedParams)
		httpTPLmatchersResult, hitColor, _ = MatchColor(httpTplMatcher, &httptpl.RespForMatch{
			RawPacket:     rspRaw,
			RequestPacket: rspIns.RawRequest,
		}, matcherParams)
		if httpTPLmatchersResult {
			err := yakit.AppendHTTPFlowTagsByHiddenIndexEx(rspIns.HiddenIndex, hitColor...)
			if err != nil {
				log.Errorf("append http flow tags failed: %s", err)
			}
		}
	}

	rsp := &ypb.FuzzerResponse{
		Method:                "GET",
		ResponseRaw:           rspRaw,
		GuessResponseEncoding: Chardet(rspRaw),
		RequestRaw:            resultRequest,
		ExtractedResults:      extractResults,
		MatchedByMatcher:      httpTPLmatchersResult,
		HitColor:              strings.Join(hitColor, "|"),
	}
	rsp.UUID = uuid.New().String()
	rsp.Timestamp = start.Unix()
	rsp.DurationMs = time.Now().Sub(start).Milliseconds()

	requestIns, err := lowhttp.ParseBytesToHttpRequest(resultRequest)
	if err != nil {
		return nil, err
	}
	rsp.Host = requestIns.Header.Get("Host")
	if rsp.Host == "" {
		rsp.Host = requestIns.Host
	}

	responseIns, err := lowhttp.ParseBytesToHTTPResponse(rspRaw)
	if responseIns != nil {
		rsp.Ok = true
		rsp.StatusCode = int32(responseIns.StatusCode)
		rsp.ContentType = responseIns.Header.Get("Content-Type")
		var bodyLen int64 = 0
		if responseIns.Body != nil {
			raw, _ := ioutil.ReadAll(responseIns.Body)
			bodyLen = int64(len(raw))
		}
		rsp.BodyLength = bodyLen

		// 解析 Headers
		for k, vs := range responseIns.Header {
			for _, v := range vs {
				rsp.Headers = append(rsp.Headers, &ypb.HTTPHeader{
					Header: k,
					Value:  v,
				})
			}
		}
	}
	return rsp, nil
}

func (s *Server) PreloadHTTPFuzzerParams(ctx context.Context, req *ypb.PreloadHTTPFuzzerParamsRequest) (*ypb.PreloadHTTPFuzzerParamsResponse, error) {
	vars := httptpl.NewVars()
	for _, k := range req.GetParams() {
		if k.GetType() == "raw" {
			vars.Set(k.GetKey(), k.GetValue())
			continue
		}
		vars.AutoSet(k.GetKey(), k.GetValue())
	}
	var results []*ypb.FuzzerParamItem
	for k, v := range vars.ToMap() {
		results = append(results, &ypb.FuzzerParamItem{
			Key:   k,
			Value: utils.InterfaceToString(v),
			Type:  "raw",
		})
	}
	return &ypb.PreloadHTTPFuzzerParamsResponse{Values: results}, nil
}

type fuzzerServerPush struct {
	FuzzerTabIndex      string `json:"fuzzer_tab_index"`
	FuzzerIndex         string `json:"fuzzer_index"`
	FuzzerSequenceIndex string `json:"fuzzer_sequence_index"`
	DiscardCount        int    `json:"discard_count"`
}

func (s *Server) HTTPFuzzer(req *ypb.FuzzerRequest, stream ypb.Yak_HTTPFuzzerServer) (finalError error) {
	defer func() {
		if err := recover(); err != nil {
			finalError = utils.Errorf("panic from httpfuzzer: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// runtimeID
	var runtimeID string
	if wrapperStream, ok := stream.(*WrapperHTTPFuzzerStream); ok {
		// runtimeID from webfuzzer sequence
		runtimeID = wrapperStream.fallback.runtimeID
	} else {
		runtimeID = uuid.NewString()
	}

	// server push info
	engineDropPacket := req.GetEngineDropPacket()

	discardCount := new(atomic.Int64)
	fuzzerTabIndex := req.GetFuzzerTabIndex()
	fuzzerIndex := req.GetFuzzerIndex()
	fuzzerSequenceIndex := req.GetFuzzerSequenceIndex()
	doFuzzerServerPush := func() {
		yakit.BroadcastData(yakit.ServerPushType_Fuzzer, &fuzzerServerPush{
			FuzzerTabIndex:      fuzzerTabIndex,
			FuzzerIndex:         fuzzerIndex,
			FuzzerSequenceIndex: fuzzerSequenceIndex,
			DiscardCount:        int(discardCount.Load()),
		})
	}

	throttle := utils.NewThrottle(1)
	doFuzzerServerPushThrottle := func() {
		throttle(doFuzzerServerPush)
	}
	defer doFuzzerServerPush()

	// retry
	isRetry := req.GetRetryTaskID() > 0
	// pause
	pauseTaskID := req.GetPauseTaskID()
	isPause := pauseTaskID > 0
	// 暂停任务
	var sw *utils.Switch
	if !isPause {
		sw = utils.NewSwitch(true)
		go func() {
			select {
			case <-stream.Context().Done():
				sw.SwitchTo(true)
			}
		}()
	} else if req.GetSetPauseStatus() {
		i, ok := _FuzzerTaskSwitchMap.Load(uint(pauseTaskID))
		if ok {
			sw = i.(*utils.Switch)
			sw.SwitchTo(!req.GetIsPause())
			return nil
		} else {
			return utils.Errorf("pause task[%d] not found", pauseTaskID)
		}
	}
	// rawRequest
	var rawRequest []byte
	if !isRetry {
		if len(req.GetRequestRaw()) > 0 {
			rawRequest = req.GetRequestRaw()
		} else {
			rawRequest = []byte(req.GetRequest())
		}
	}

	// hot code
	var extraOpt []mutate.FuzzConfigOpt
	if strings.TrimSpace(req.GetHotPatchCode()) != "" {
		extraOpt = append(extraOpt, yak.Fuzz_WithAllHotPatch(stream.Context(), req.GetHotPatchCode())...)
		extraOpt = append(extraOpt, mutate.Fuzz_WithExtraFuzzTagHandler("request", func(s string) []string {
			return []string{utils.UnsafeBytesToString(rawRequest)}
		}))
	}

	/*
		Plugins
	*/
	var pocs []*schema.YakScript
	for _, i := range req.GetYamlPoCNames() {
		poc, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), i)
		if err != nil {
			log.Errorf("get yaml poc[%v] failed: %s", i, err)
			continue
		}
		if poc.Type != "nuclei" {
			log.Errorf("poc[%s] is not yaml poc: %s", i, poc.Type)
			continue
		}
		pocs = append(pocs, poc)
	}

	var batchTarget string
	if req.GetBatchTargetFile() {
		if ret := utils.GetFirstExistedFile(string(req.BatchTarget)); ret != "" {
			fp, err := os.Open(ret)
			if err != nil {
				return utils.Errorf("open batch target file failed: %s", err)
			}
			raw, _ := io.ReadAll(fp)
			fp.Close()
			batchTarget = strings.TrimSpace(string(raw))
		} else {
			return utils.Errorf("batch target file not found: %s", req.GetBatchTarget())
		}
	} else {
		batchTarget = string(req.GetBatchTarget())
	}

	// feedback
	swg := utils.NewSizedWaitGroup(int(req.GetConcurrent()))
	defer swg.Wait()
	feedbackWg := new(sync.WaitGroup)
	defer func() {
		feedbackWg.Wait()
	}()
	feedbackLock := new(sync.Mutex)
	feedbackResponse := func(rsp *ypb.FuzzerResponse, skipPoC bool) error {
		feedbackLock.Lock()
		defer feedbackLock.Unlock()
		startTime := time.Now()
		defer func() {
			duration := time.Now().Sub(startTime)
			if duration > time.Second {
				log.Infof("http fuzzer response feedback cost too much for %v", duration)
			}
		}()

		if !req.GetReMatch() {
			sw.WaitUntilOpen()
		}

		err := stream.Send(rsp)
		if err != nil {
			return err
		}

		if skipPoC {
			return nil
		}

		feedbackWg.Add(1)
		go func() {
			defer feedbackWg.Done()
			for _, p := range pocs {
				poc := p
				err := swg.AddWithContext(stream.Context())
				if err != nil {
					break
				}
				go func() {
					defer swg.Done()
					defer func() {
						if err := recover(); err != nil {
							spew.Dump(err)
							utils.PrintCurrentGoroutineRuntimeStack()
						}
					}()
					httptpl.ScanPacket(
						rsp.RequestRaw, lowhttp.WithHttps(rsp.IsHTTPS),
						httptpl.WithTemplateRaw(poc.Content),
						lowhttp.WithSaveHTTPFlowHandler(func(i *lowhttp.LowhttpResponse) {
							err := stream.Send(ConvertLowhttpResponseToFuzzerResponseBase(i))
							if err != nil {
								log.Errorf("yaml poc send failed")
							}
						}),
						httptpl.WithOnRisk(rsp.Url, func(i *schema.Risk) {
							log.Infof("found risk: %s", i.Title)
						}),
					)
				}()

			}
		}()
		return nil
	}

	historyID := req.GetHistoryWebFuzzerId()
	reMatch := req.GetReMatch()

	httpTplMatcher := make([]*YakFuzzerMatcher, len(req.GetMatchers()))
	httpTplExtractor := make([]*httptpl.YakExtractor, len(req.GetExtractors()))
	haveHTTPTplMatcher := len(httpTplMatcher) > 0
	haveHTTPTplExtractor := len(httpTplExtractor) > 0
	if haveHTTPTplExtractor {
		for i, e := range req.GetExtractors() {
			httpTplExtractor[i] = httptpl.NewExtractorFromGRPCModel(e)
		}
	}

	if haveHTTPTplMatcher {
		for i, m := range req.GetMatchers() {
			httpTplMatcher[i] = NewHttpFlowMatcherFromGRPCModel(m)
		}
	}

	if historyID > 0 {
		// 回溯找到所有之前的包，进行整合
		oldIDs, err := yakit.GetWebFuzzerTasksIDByRetryRootID(s.GetProjectDatabase(), uint(historyID))
		// 找到最新的任务并排除
		latestID := lo.Max(oldIDs)
		if !reMatch {
			oldIDs = lo.Filter(oldIDs, func(item uint, _ int) bool {
				return item != latestID
			})
		}

		if err != nil {
			log.Errorf("get old web fuzzer success response failed: %s", err)
		} else {
			// 重匹配的分支
			if reMatch {
				if len(oldIDs) == 0 { // 尝试修复
					oldIDs = []uint{uint(historyID)}
				}
				_, _, getMirrorHTTPFlowParams, _, _ := yak.MutateHookCaller(stream.Context(), req.GetHotPatchCode(), nil)
				for resp := range yakit.YieldWebFuzzerResponseByTaskIDs(s.GetProjectDatabase(), stream.Context(), oldIDs, true) {
					var extractorResults []*ypb.KVPair
					respModel, err := resp.ToGRPCModel()
					if err != nil || respModel == nil {
						log.Errorf("convert web fuzzer response to grpc model failed: %s", err)
						continue
					}

					if haveHTTPTplExtractor { // 提取器提取参数
						params := make(map[string]any)
						for _, extractor := range httpTplExtractor {
							vars, err := extractor.ExecuteWithRequest(respModel.ResponseRaw, respModel.RequestRaw, respModel.IsHTTPS, params)
							if err != nil {
								log.Errorf("extractor execute failed: %s", err)
								continue
							}
							for k, v := range vars {
								params[k] = v
								extractorResults = append(extractorResults, &ypb.KVPair{Key: k, Value: httptpl.ExtractResultToString(v), MarshalValue: marshalValue(v)}) // 提取器 参数
							}
						}
					}
					var httpTPLmatchersResult bool
					var hitColor []string
					var discard bool
					for mergedParams := range s.PreRenderVariables(stream.Context(), req.GetParams(), req.GetIsHTTPS(), req.GetIsGmTLS(), false) {
						existedParams := make(map[string]string) // 传入的参数
						if mergedParams != nil {
							for k, v := range utils.InterfaceToMap(mergedParams) {
								existedParams[k] = strings.Join(v, ",")
							}
						}

						if getMirrorHTTPFlowParams != nil {
							for k, v := range getMirrorHTTPFlowParams(respModel.RequestRaw, respModel.ResponseRaw, existedParams) { // 热加载的参数
								extractorResults = append(extractorResults, &ypb.KVPair{Key: utils.EscapeInvalidUTF8Byte([]byte(k)), Value: utils.EscapeInvalidUTF8Byte([]byte(v)), MarshalValue: marshalValue(v)})
							}
						}

						matcherParams := utils.CopyMapInterface(mergedParams)
						for _, kv := range extractorResults { // 合并
							matcherParams[kv.GetKey()] = kv.GetValue()
						}
						httpTPLmatchersResult, hitColor, discard = MatchColor(httpTplMatcher,
							&httptpl.RespForMatch{
								RawPacket:     respModel.ResponseRaw,
								Duration:      float64(respModel.DurationMs),
								RequestPacket: respModel.RequestRaw,
							},
							matcherParams)
						if httpTPLmatchersResult {
							respModel.MatchedByMatcher = true
							respModel.HitColor = strings.Join(hitColor, "|")
							respModel.Discard = discard
							break
						}
					}
					if discard && engineDropPacket {
						discardCount.Add(1)
						doFuzzerServerPushThrottle()
						continue
					}
					respModel.TaskId = int64(historyID)
					respModel.ExtractedResults = extractorResults
					feedbackResponse(respModel, true)
				}

			} else {
				// 只展示之前成功的包
				if len(oldIDs) > 0 {
					for resp := range yakit.YieldWebFuzzerResponseByTaskIDs(s.GetProjectDatabase(), stream.Context(), oldIDs, true) {
						respModel, err := resp.ToGRPCModel()
						if err != nil {
							log.Errorf("convert web fuzzer response to grpc model failed: %s", err)
							continue
						}
						feedbackResponse(respModel, true)
					}
				}

				// 展示最新任务的所有包
				for resp := range yakit.YieldWebFuzzerResponses(s.GetProjectDatabase(), stream.Context(), int(latestID)) {
					respModel, err := resp.ToGRPCModel()
					if err != nil {
						log.Errorf("convert web fuzzer response to grpc model failed: %s", err)
						continue
					}
					feedbackResponse(respModel, true)
				}
			}
		}
		return nil
	}
	if !isRetry && len(rawRequest) <= 0 {
		return utils.Errorf("empty request is not allowed")
	}

	proxies := utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(req.GetProxy(), ","))
	concurrent := req.GetConcurrent()
	if concurrent <= 0 {
		concurrent = 20
	}
	timeoutSeconds := req.GetPerRequestTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}

	dialTimeoutSeconds := req.GetDialTimeoutSeconds()
	if dialTimeoutSeconds <= 0 {
		dialTimeoutSeconds = 5
	}

	task, err := yakit.SaveWebFuzzerTask(s.GetProjectDatabase(), req, 0, false, "executing...")
	if err != nil {
		return utils.Errorf("save to web fuzzer to database failed: %s", err)
	}
	// 重试任务
	var retryRootID uint
	taskID := task.ID
	task.FuzzerIndex = req.GetFuzzerIndex()
	task.FuzzerTabIndex = req.GetFuzzerTabIndex()
	if !isRetry {
		task.RetryRootID = task.ID
	} else {
		retryRootID, err = yakit.GetWebFuzzerRetryRootID(s.GetProjectDatabase(), uint(req.RetryTaskID))
		if err != nil {
			return err
		}
		task.RetryRootID = retryRootID
	}
	// 存储重试任务的开关
	_FuzzerTaskSwitchMap.Store(task.ID, sw)

	defer func() {
		if db := s.GetProjectDatabase().Save(task); db.Error != nil {
			log.Errorf("update web fuzzer task failed: %s", db.Error)
		}
	}()

	/* 丢包过滤器 */
	includeStatusCodeFilter := utils.NewPortsFilter()
	var maxBody, minBody int64
	var regexps, keywords []string
	filter := req.GetFilter()
	if filter != nil {
		includeStatusCodeFilter.Add(filter.GetStatusCode()...)
		regexps = filter.GetRegexps()
		keywords = filter.GetKeywords()
		minBody = filter.GetMinBodySize()
		maxBody = filter.GetMaxBodySize()
	}

	// 保存 request 中 host/port
	defer func() {
		if req.GetActualAddr() != "" {
			task.Host = req.GetActualAddr()
		} else {
			results := extractHostRegexp.FindStringSubmatch(string(rawRequest))
			if len(results) > 1 {
				task.Host = results[1]
				if len(task.Host) > 40 {
					task.Host = task.Host[:40] + "..."
				}
			}
		}
		_, task.Port, _ = utils.ParseStringToHostPort(task.Host)
	}()

	inStatusCode := utils.ParseStringToPorts(req.GetRetryInStatusCode())
	notInStatusCode := utils.ParseStringToPorts(req.GetRetryNotInStatusCode())

	var iInput any
	retryPayloadsMap := make(map[string][]string) // key 是原始请求报文，value 是重试的payload，我们需要将重试的payload绑定回去
	// 这里可能会出现原始请求报文一样的情况，但是这样也是因为payload没有而导致的，例如{{repeat(10)}}

	if !isRetry {
		// 插入 {{repeat(n)}}的fuzz标签
		if req.GetRepeatTimes() > 0 {
			var buf bytes.Buffer
			buf.WriteString("{{repeat(" + fmt.Sprint(req.GetRepeatTimes()) + ")}}")
			buf.Write(rawRequest)
			rawRequest = buf.Bytes()
		}
		iInput = rawRequest
	} else {
		// 找到上次任务的包
		failedResponses := make([]*schema.WebFuzzerResponse, 0)
		for resp := range yakit.YieldWebFuzzerResponses(s.GetProjectDatabase(), stream.Context(), int(req.RetryTaskID)) {
			if !resp.OK {
				failedResponses = append(failedResponses, resp)
				retryPayloadsMap[resp.Request] = strings.Split(resp.Payload, ",")
			}
		}

		if len(failedResponses) == 0 {
			return utils.Errorf("no failed web fuzzer request found")
		}

		// 回溯找到所有之前重试成功的包
		oldIDs, err := yakit.GetWebFuzzerTasksIDByRetryRootID(s.GetProjectDatabase(), retryRootID)
		if err != nil {
			log.Errorf("get old web fuzzer success response failed: %s", err)
		} else {
			for resp := range yakit.YieldWebFuzzerResponseByTaskIDs(s.GetProjectDatabase(), stream.Context(), oldIDs, true) {
				respModel, err := resp.ToGRPCModel()
				if err != nil {
					log.Errorf("convert web fuzzer response to grpc model failed: %s", err)
					continue
				}
				feedbackResponse(respModel, true)
			}
		}

		iInput = lo.Map(failedResponses, func(i *schema.WebFuzzerResponse, _ int) []byte {
			return []byte(i.Request)
		})
	}

	requestCount := 0
	if req.GetForceOnlyOneResponse() {
		requestCount = 1
	}

	//maxBodySize := 5 * 1024 * 1024
	maxBodySize := consts.GetGlobalMaxContentLength()
	if req.GetMaxBodySize() > 1024 && req.GetMaxBodySize() < 10*1024*1024 {
		maxBodySize = uint64(req.MaxBodySize)
	}

	fuzzerRequestSwg := utils.NewSizedWaitGroup(int(concurrent))
	executeBatchRequestsWithParams := func(mergedParams map[string]any) (retErr error) {
		defer func() {
			if err := recover(); err != nil {
				retErr = utils.Errorf("panic from grpc.httpfuzzer executeBatchRequestsWithParams: %v", err)
				utils.Debug(func() {
					utils.PrintCurrentGoroutineRuntimeStack()
				})
			}
		}()

		httpPoolOpts := []mutate.HttpPoolConfigOption{
			mutate.WithPoolOpt_FuzzParams(mergedParams),
			mutate.WithPoolOpt_ExtraFuzzOptions(extraOpt...),
			mutate.WithPoolOpt_Timeout(timeoutSeconds),
			mutate.WithPoolOpt_DialTimeout(dialTimeoutSeconds),
			mutate.WithPoolOpt_Proxy(proxies...),
			mutate.WithPoolOpt_BatchTarget(batchTarget),
			mutate.WithPoolOpt_SizedWaitGroup(fuzzerRequestSwg),
			mutate.WithPoolOpt_Addr(req.GetActualAddr(), req.GetIsHTTPS()),
			mutate.WithPoolOpt_RawMode(true),
			mutate.WithPoolOpt_Https(req.GetIsHTTPS()),
			mutate.WithPoolOpt_GmTLS(req.GetIsGmTLS()),
			mutate.WithPoolOpt_RandomJA3(req.GetRandomJA3()),
			mutate.WithPoolOpt_Context(stream.Context()),
			mutate.WithPoolOpt_FollowJSRedirect(req.GetFollowJSRedirect()),
			mutate.WithPoolOpt_RedirectTimes(int(req.GetRedirectTimes())),
			mutate.WithPoolOpt_NoFollowRedirect(req.GetNoFollowRedirect()),
			mutate.WithPoolOpt_noFixContentLength(req.GetNoFixContentLength()),
			// mutate.WithPoolOpt_ExtraMutateConditionGetter(yak.MutateWithParamsGetter(req.GetHotPatchCodeWithParamGetter())),
			// mutate.WithPoolOpt_ExtraMutateCondition(yak.MutateWithYaklang(req.GetHotPatchCode())),
			mutate.WithPoolOpt_DelayMinSeconds(req.GetDelayMinSeconds()),
			mutate.WithPoolOPt_DelayMaxSeconds(req.GetDelayMaxSeconds()),
			mutate.WithPoolOpt_Source("webfuzzer"),
			mutate.WithPoolOpt_RetryTimes(int(req.GetMaxRetryTimes())),
			mutate.WithPoolOpt_MaxContentLength(int(maxBodySize)),
			mutate.WithPoolOpt_RetryInStatusCode(inStatusCode),
			mutate.WithPoolOpt_RetryNotInStatusCode(notInStatusCode),
			mutate.WithPoolOpt_RetryWaitTime(req.GetRetryWaitSeconds()),
			mutate.WithPoolOpt_RetryMaxWaitTime(req.GetRetryMaxWaitSeconds()),
			mutate.WithPoolOpt_DNSServers(req.GetDNSServers()),
			mutate.WithPoolOpt_EtcHosts(req.GetEtcHosts()),
			mutate.WithPoolOpt_NoSystemProxy(req.GetNoSystemProxy()),
			mutate.WithPoolOpt_RequestCountLimiter(requestCount),
			mutate.WithPoolOpt_MutateWithMethods(req.GetMutateMethods()),
			mutate.WithPoolOpt_RuntimeId(runtimeID),
			mutate.WithPoolOpt_WithPayloads(true),
			mutate.WithPoolOpt_RandomSession(true),
			mutate.WithPoolOpt_UseConnPool(!req.GetDisableUseConnPool()),
			mutate.WithPoolOpt_SaveHTTPFlow(false),
			mutate.WithPoolOpt_NoReadMultiResponse(req.GetNoReadMultiResponse()),
			//mutate.WithPoolOpt_ConnPool(true),
		}

		if !req.GetDisableUseConnPool() {
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ConnPool(lowhttp.NewHttpConnPool(stream.Context(), int(concurrent*50), int(concurrent))))
		}

		if !req.GetDisableHotPatch() {
			beforeRequest, afterRequest, mirrorHTTPFlow, retryHandler, customFailureChecker := yak.MutateHookCaller(stream.Context(), req.GetHotPatchCode(), nil)
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_HookCodeCaller(beforeRequest, afterRequest, mirrorHTTPFlow, retryHandler, customFailureChecker))
		}

		if req.GetOverwriteSNI() {
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_SNI(req.GetSNI()))
		}

		if req.GetEnableRandomChunked() {
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_EnableRandomChunked(req.GetEnableRandomChunked()))
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_RandomChunkedLength(int(req.GetRandomChunkedMinLength()), int(req.GetRandomChunkedMaxLength())))
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_RandomChunkDelayTime(
				time.Duration(req.GetRandomChunkedMinDelay())*time.Millisecond,
				time.Duration(req.GetRandomChunkedMaxDelay())*time.Millisecond,
			))
		}
		fuzzMode := req.GetFuzzTagMode() // ""/"close"/"standard"/"legacy"
		forceFuzz := req.GetForceFuzz()  // true/false
		if fuzzMode == "" {              // 以forceFuzz为准
			if forceFuzz {
				fuzzMode = "standard"
			} else {
				fuzzMode = "close"
			}
		}
		if isRetry {
			// 重试的时候，不需要渲染fuzztag
			fuzzMode = "close"
		}
		switch fuzzMode {
		case "close":
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ForceFuzz(false))
		case "standard":
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ForceFuzz(true))
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ForceFuzzDangerous(true))
		case "simple", "legacy":
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ForceFuzz(true))
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ForceFuzzDangerous(true))
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ExtraFuzzOptions(mutate.Fuzz_WithSimple(true)))
		}
		if req.GetFuzzTagSyncIndex() {
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ExtraFuzzOptions(mutate.Fuzz_SyncTag(true)))
		}
		if !isPause {
			httpPoolOpts = append(httpPoolOpts, mutate.WithPoolOpt_ExternSwitch(sw))
		}
		res, err := mutate.ExecPool(
			iInput,
			httpPoolOpts...,
		)
		if err != nil {
			task.Ok = false
			task.Reason = utils.Errorf("exec http pool failed: %s", err).Error()
			return err
		}
		// 可以用于计算相似度
		var firstHeader, firstBody []byte

		nowTime := time.Now()
		count := 0
		for result := range res {
			count++
			if count > 2 && time.Now().Sub(nowTime).Seconds() > 1 {
				log.Error("HELP! handle result cost too much time, can someone investigate it?")
			}
			nowTime = time.Now()

			// 2M
			if len(result.RequestRaw) > 2*1024*1024 {
				result.RequestRaw = result.RequestRaw[:2*1024*1024]
				result.RequestRaw = append(result.RequestRaw, []byte("...(request > 2M) show chunked by yakit web fuzzer")...)
			}

			var payloads []string
			task.HTTPFlowTotal++

			if !isRetry {
				payloads = make([]string, len(result.Payloads))
				for i, payload := range result.Payloads {
					if len(payload) > 100 {
						payload = payload[:100] + "..."
					}
					payloads[i] = utils.ParseStringToVisible(payload)
				}
			} else {
				payloads, _ = retryPayloadsMap[string(result.RequestRaw)]
			}

			var extractorResults []*ypb.KVPair

			if result != nil && result.ExtraInfo != nil {
				for k, v := range result.ExtraInfo {
					extractorResults = append(extractorResults, &ypb.KVPair{Key: utils.EscapeInvalidUTF8Byte([]byte(k)), Value: utils.EscapeInvalidUTF8Byte([]byte(v)), MarshalValue: marshalValue(v)})
				}
			}

			if result.Error != nil {
				log.Errorf("http pool error: %s", result.Error)
				hiddenIndex := ""
				rsp := &ypb.FuzzerResponse{}
				rsp.RequestRaw = result.RequestRaw
				rsp.UUID = uuid.New().String()
				rsp.Url = utils.EscapeInvalidUTF8Byte([]byte(result.Url))
				rsp.Ok = false
				rsp.Reason = result.Error.Error()
				rsp.TaskId = int64(taskID)
				rsp.Payloads = payloads
				rsp.RuntimeID = runtimeID
				rsp.ResponseRaw = result.ResponseRaw
				if result.LowhttpResponse != nil && result.LowhttpResponse.TraceInfo != nil {
					SetFuzzerRespTraceInfo(rsp, result.LowhttpResponse.TraceInfo)
					rsp.RemoteAddr = result.LowhttpResponse.RemoteAddr
					hiddenIndex = result.LowhttpResponse.HiddenIndex
				}
				if hiddenIndex == "" {
					hiddenIndex = uuid.NewString()
				}

				task.HTTPFlowFailedCount++
				// yakit.SaveWebFuzzerResponse(s.GetProjectDatabase(), int(task.ID), hiddenIndex, rsp)
				yakit.SaveWebFuzzerResponseEx(int(task.ID), hiddenIndex, rsp)
				_ = feedbackResponse(rsp, false)
				continue
			}

			if haveHTTPTplExtractor {
				params := make(map[string]any)
				for _, extractor := range httpTplExtractor {
					vars, err := extractor.ExecuteWithRequest(result.ResponseRaw, result.RequestRaw, req.GetIsHTTPS(), params)
					if err != nil {
						log.Errorf("extractor execute failed: %s", err)
						continue
					}
					for k, v := range vars {
						params[k] = v
						extractorResults = append(extractorResults, &ypb.KVPair{Key: k, Value: httptpl.ExtractResultToString(v), MarshalValue: marshalValue(v)})
					}
				}
			}
			extractorResultsOrigin := extractorResults
			for k, v := range mergedParams {
				extractorResults = append(extractorResults, &ypb.KVPair{
					Key: k, Value: utils.EscapeInvalidUTF8Byte(codec.AnyToBytes(v)),
					MarshalValue: marshalValue(v),
				},
				)
			}

			var httpTPLmatchersResult, discard bool
			var hitColor []string
			lowhttpResponse := result.LowhttpResponse

			if haveHTTPTplMatcher && lowhttpResponse != nil {
				//cond := "and"
				//switch ret := strings.ToLower(req.GetMatchersCondition()); ret {
				//case "or", "and":
				//	cond = ret
				//default:
				//}
				//ins := &httptpl.YakMatcher{
				//	SubMatcherCondition: cond,
				//	SubMatchers:         httpTplMatcher,
				//}

				matcherParams := utils.CopyMapInterface(mergedParams)
				for _, kv := range extractorResultsOrigin {
					matcherParams[kv.GetKey()] = kv.GetValue()
				}
				matchColorStart := time.Now()
				httpTPLmatchersResult, hitColor, discard = MatchColor(httpTplMatcher, &httptpl.RespForMatch{
					RawPacket:     result.ResponseRaw,
					Duration:      lowhttpResponse.GetDurationFloat(),
					RequestPacket: result.RequestRaw,
				}, matcherParams)
				if du := time.Now().Sub(matchColorStart); du > time.Second {
					log.Warnf("match color and append httpflow tags cost too much time, can someone investigate it? cost: %v", du)
				}

				if discard && engineDropPacket {
					discardCount.Add(1)
					doFuzzerServerPushThrottle()
					continue
				} else {
					if httpTPLmatchersResult {
						result.LowhttpResponse.Tags = append(result.LowhttpResponse.Tags, hitColor...)
					}
				}
			}

			if consts.GLOBAL_HTTP_FLOW_SAVE.IsSet() {
				yakit.SaveLowHTTPFlow(result.LowhttpResponse, false)
			}

			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(result.ResponseRaw)

			if !req.GetNoFixContentLength() && (result.Request != nil && result.Request.ProtoMajor != 2) { // no fix for h2 rsp
				result.ResponseRaw = lowhttp.ReplaceHTTPPacketBody(result.ResponseRaw, body, false)
				result.Response, _ = lowhttp.ParseStringToHTTPResponse(string(result.ResponseRaw))
			}

			tooLarge := false
			tooLargeHeaderFile, tooLargeBodyFile := "", ""
			if lowhttpResponse != nil {
				tooLarge = lowhttpResponse.TooLarge
				if tooLarge {
					reqIns := lowhttpResponse.RequestInstance
					tooLargeHeaderFile = httpctx.GetResponseTooLargeHeaderFile(reqIns)
					tooLargeBodyFile = httpctx.GetResponseTooLargeBodyFile(reqIns)
				}
			}

			feedbackNormalResponseStart := time.Now()
			task.HTTPFlowSuccessCount++
			rsp := &ypb.FuzzerResponse{
				Ok:                         true,
				Url:                        utils.EscapeInvalidUTF8Byte([]byte(result.Url)),
				Method:                     utils.EscapeInvalidUTF8Byte([]byte(result.Request.Method)),
				ResponseRaw:                result.ResponseRaw,
				GuessResponseEncoding:      Chardet(result.ResponseRaw),
				RequestRaw:                 result.RequestRaw,
				Payloads:                   payloads,
				IsHTTPS:                    strings.HasPrefix(strings.ToLower(result.Url), "https://"),
				ExtractedResults:           extractorResults,
				MatchedByMatcher:           httpTPLmatchersResult,
				HitColor:                   strings.Join(hitColor, "|"),
				IsTooLargeResponse:         tooLarge,
				TooLargeResponseBodyFile:   tooLargeBodyFile,
				TooLargeResponseHeaderFile: tooLargeHeaderFile,
				DisableRenderStyles:        len(body) > 1024*1024*2,
				RuntimeID:                  runtimeID,
				IsAutoFixContentType:       lowhttpResponse.IsFixContentType,
				OriginalContentType:        lowhttpResponse.OriginContentType,
				FixContentType:             lowhttpResponse.FixContentType,
				IsSetContentTypeOptions:    lowhttpResponse.IsSetContentTypeOptions,
			}

			if req.GetEnableRandomChunked() && result.RandomChunkedData != nil {
				for _, chunkInfo := range result.RandomChunkedData {
					rsp.RandomChunkedData = append(rsp.RandomChunkedData, chunkInfo.ToGRPCModel())
				}
			}

			redirectPacket := result.LowhttpResponse.RedirectRawPackets
			if result.LowhttpResponse != nil {
				// redirect
				for _, f := range redirectPacket {
					rsp.RedirectFlows = append(rsp.RedirectFlows, &ypb.RedirectHTTPFlow{
						IsHttps:  f.IsHttps,
						Request:  f.Request,
						Response: f.Response,
					})
				}
			}

			// 处理额外时间
			if result.LowhttpResponse != nil && result.LowhttpResponse.TraceInfo != nil {
				SetFuzzerRespTraceInfo(rsp, result.LowhttpResponse.TraceInfo)
				rsp.Proxy = result.LowhttpResponse.Proxy
				rsp.RemoteAddr = result.LowhttpResponse.RemoteAddr
			}
			if len(rsp.ResponseRaw) == 0 { // 只有在http pool请求、解析未出错，但响应为空时才会进入此分支
				rsp.Ok = false
				rsp.Reason = "empty response"
			}
			if rsp.ResponseRaw != nil {
				// 处理结果，相似度
				header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp.ResponseRaw)
				if firstHeader == nil {
					log.Debugf("start to set first header[%v]...", result.Url)
					firstHeader = []byte(header)
					rsp.HeaderSimilarity = 1.0
				} else {
					rsp.HeaderSimilarity = utils.CalcSimilarity(firstHeader, []byte(header))
				}

				if firstBody == nil {
					log.Debugf("start to set first body[%v]...", result.Url)
					firstBody = body
					rsp.BodySimilarity = 1.0
				} else {
					rsp.BodySimilarity = utils.CalcSimilarity(firstBody, body)
				}
			}

			rsp.UUID = uuid.New().String()
			rsp.Timestamp = result.Timestamp
			rsp.DurationMs = result.DurationMs
			rsp.Host = utils.EscapeInvalidUTF8Byte([]byte(result.Request.Header.Get("Host")))
			if rsp.Host == "" {
				rsp.Host = result.Request.Host
			}
			rsp.Host = utils.EscapeInvalidUTF8Byte([]byte(utils.ParseStringToVisible(result.Request.Host)))

			if result.Response != nil {
				rsp.StatusCode = int32(result.Response.StatusCode)
				rsp.ContentType = utils.ParseStringToVisible(result.Response.Header.Get("Content-Type"))
				var bodyLen int64 = 0
				if result.Response.Body != nil {
					raw, _ := ioutil.ReadAll(result.Response.Body)
					bodyLen = int64(len(raw))
				}
				rsp.BodyLength = bodyLen

				// 解析 Headers
				for k, vs := range result.Response.Header {
					for _, v := range vs {
						rsp.Headers = append(rsp.Headers, &ypb.HTTPHeader{
							Header: utils.ParseStringToVisible(k),
							Value:  utils.ParseStringToVisible(v),
						})
					}
				}
			}

			if rsp.StatusCode > 0 {
				// 通过长度过滤
				if minBody <= maxBody && (minBody > 0 || maxBody > 0) {
					if maxBody >= rsp.BodyLength && minBody <= rsp.BodyLength {
						rsp.MatchedByFilter = true
					}
				}

				// 通过 StatusCode 过滤
				if !rsp.MatchedByFilter {
					rsp.MatchedByFilter = includeStatusCodeFilter.Contains(int(rsp.StatusCode))
				}

				// rule
				if !rsp.MatchedByFilter && (len(regexps) > 0 || len(keywords) > 0) {
					if utils.MatchAnyOfRegexp(rsp.ResponseRaw, regexps...) {
						rsp.MatchedByFilter = true
					}
					if rsp.MatchedByFilter || utils.MatchAllOfRegexp(rsp.ResponseRaw, keywords...) {
						rsp.MatchedByFilter = true
					}
				}
			}
			// 自动重定向
			if !req.GetNoFollowRedirect() {

				for i := 0; i < len(redirectPacket)-1; i++ {
					redirectRes := redirectPacket[i].RespRecord
					method, _, _ := lowhttp.GetHTTPPacketFirstLine(redirectRes.RawRequest)
					var redirectMatchersResult, redirectDiscard bool
					var redirectHitColor []string
					if haveHTTPTplMatcher {
						matcherParams := utils.CopyMapInterface(mergedParams)
						for _, kv := range extractorResultsOrigin {
							matcherParams[kv.GetKey()] = kv.GetValue()
						}
						redirectMatchersResult, redirectHitColor, redirectDiscard = MatchColor(httpTplMatcher, &httptpl.RespForMatch{
							RawPacket:     redirectRes.RawPacket,
							Duration:      redirectRes.GetDurationFloat(),
							RequestPacket: redirectRes.RawRequest,
						}, matcherParams)

						if redirectDiscard && engineDropPacket {
							discardCount.Add(1)
							doFuzzerServerPushThrottle()
							continue
						} else {
							if redirectMatchersResult {
								redirectRes.Tags = append(redirectRes.Tags, hitColor...)
							}
						}

					}

					if consts.GLOBAL_HTTP_FLOW_SAVE.IsSet() {
						yakit.SaveLowHTTPFlow(redirectRes, false)
					}

					redirectRsp := &ypb.FuzzerResponse{
						Url:                   utils.EscapeInvalidUTF8Byte([]byte(redirectRes.Url)),
						Method:                utils.EscapeInvalidUTF8Byte([]byte(method)),
						ResponseRaw:           redirectRes.RawPacket,
						GuessResponseEncoding: Chardet(redirectRes.RawPacket),
						RequestRaw:            redirectRes.RawRequest,
						Payloads:              payloads,
						IsHTTPS:               redirectRes.Https,
						MatchedByMatcher:      redirectMatchersResult,
						HitColor:              strings.Join(redirectHitColor, "|"),
						RuntimeID:             runtimeID,
						Discard:               redirectDiscard,
					}
					if redirectRes != nil && redirectRes.TraceInfo != nil {
						SetFuzzerRespTraceInfo(redirectRsp, redirectRes.TraceInfo)
						redirectRsp.Proxy = redirectRes.Proxy
						redirectRsp.RemoteAddr = redirectRes.RemoteAddr
					}
					redirectRsp.UUID = uuid.New().String()
					redirectRsp.Timestamp = result.Timestamp
					redirectRsp.DurationMs = result.DurationMs
					redirectRsp.Host = utils.EscapeInvalidUTF8Byte([]byte(lowhttp.GetHTTPPacketHeader(redirectRes.RawRequest, "Host")))

					if redirectRes.RawPacket != nil {
						redirectRsp.Ok = true
						redirectRsp.StatusCode = int32(lowhttp.GetStatusCodeFromResponse(redirectRes.RawPacket))
						redirectRsp.ContentType = utils.ParseStringToVisible(lowhttp.GetHTTPPacketHeader(redirectRes.RawPacket, "Content-Type"))
						var bodyLen int64 = 0
						if lowhttp.GetHTTPPacketBody(redirectRes.RawPacket) != nil {
							bodyLen = int64(len(lowhttp.GetHTTPPacketBody(redirectRes.RawPacket)))
						}
						redirectRsp.BodyLength = bodyLen

						// 解析 Headers
						for k, vs := range lowhttp.GetHTTPPacketHeaders(redirectRes.RawPacket) {
							for _, v := range vs {
								redirectRsp.Headers = append(redirectRsp.Headers, &ypb.HTTPHeader{
									Header: utils.ParseStringToVisible(k),
									Value:  utils.ParseStringToVisible(v),
								})
							}
						}
					}

					if redirectRsp.StatusCode > 0 {
						// 通过长度过滤
						if minBody <= maxBody && (minBody > 0 || maxBody > 0) {
							if maxBody >= redirectRsp.BodyLength && minBody <= redirectRsp.BodyLength {
								redirectRsp.MatchedByFilter = true
							}
						}

						// 通过 StatusCode 过滤
						if !redirectRsp.MatchedByFilter {
							redirectRsp.MatchedByFilter = includeStatusCodeFilter.Contains(int(redirectRsp.StatusCode))
						}

						// rule
						if !redirectRsp.MatchedByFilter && (len(regexps) > 0 || len(keywords) > 0) {
							if utils.MatchAnyOfRegexp(redirectRsp.ResponseRaw, regexps...) {
								redirectRsp.MatchedByFilter = true
							}
							if redirectRsp.MatchedByFilter || utils.MatchAllOfRegexp(redirectRsp.ResponseRaw, keywords...) {
								redirectRsp.MatchedByFilter = true
							}
						}
					}
					// yakit.SaveWebFuzzerResponse(s.GetProjectDatabase(), int(task.ID), redirectRes.Uuid, redirectRsp)
					yakit.SaveWebFuzzerResponseEx(int(task.ID), redirectRes.HiddenIndex, redirectRsp)
					redirectRsp.TaskId = int64(taskID)
					err := feedbackResponse(redirectRsp, false)
					if err != nil {
						log.Errorf("send to client failed: %s", err)
						continue
					}
				}
				// 如果重定向了,修正最后一个req
				if len(redirectPacket) > 0 {
					rsp.RequestRaw = redirectPacket[len(redirectPacket)-1].Request
				}
			}
			// yakit.SaveWebFuzzerResponse(s.GetProjectDatabase(), int(task.ID), result.LowhttpResponse.Uuid, rsp)
			yakit.SaveWebFuzzerResponseEx(int(task.ID), result.LowhttpResponse.HiddenIndex, rsp)
			rsp.TaskId = int64(taskID)
			rsp.Discard = discard
			err := feedbackResponse(rsp, false)
			if du := time.Now().Sub(feedbackNormalResponseStart); du > time.Second {
				log.Warnf("feedbackNormalResponse cost too much time, try investigate it, cost: %v", du)
			}
			if err != nil {
				log.Errorf("send to client failed: %s", err)
			}
		}
		return nil
	}

	/*
		handle vars
	*/
	wg := new(sync.WaitGroup)

	errReader, errWriter := utils.NewBufPipe(nil)
	errFilter := filter2.NoCacheNewFilter()

	mtx := new(sync.Mutex)
	for _param := range s.PreRenderVariables(stream.Context(), req.GetParams(), req.GetIsHTTPS(), req.GetIsGmTLS(), req.GetFuzzTagSyncIndex()) {
		mergedParams := _param
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := executeBatchRequestsWithParams(mergedParams)
			if err != nil {

				mtx.Lock()
				defer func() {
					mtx.Unlock()
				}()
				msgs := err.Error()
				if errFilter.Exist(msgs) {
					return
				}
				errFilter.Insert(msgs)
				if errReader.Count() > 0 {
					errWriter.Write([]byte("\n"))
				}
				_, _ = errWriter.Write([]byte(msgs))
			}
		}()
	}
	wg.Wait()
	errWriter.Close()

	var errBuf bytes.Buffer
	io.Copy(&errBuf, errReader)

	if errBuf.Len() > 0 {
		task.Ok = false
		task.Reason = errBuf.String()
		return utils.Errorf("execute batch requests failed: %s", errBuf.String())
	}
	task.Ok = true
	task.Reason = "normal exit / user canceled"
	return nil
}

var requestToMutateResult = func(reqs []*http.Request, chunked bool) (*ypb.MutateResult, error) {
	var raws [][]byte
	for _, r := range reqs {
		if chunked {
			r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36")
			r.Header.Set("Accept-Encoding", "*/*")
			r.Close = true
			urlIns, err := lowhttp.ExtractURLFromHTTPRequest(r, false)
			if err != nil {
				log.Errorf("extract url from httprequest: %v", err)
			}

			if urlIns != nil {
				r.URL = urlIns
			} else {
				r.URL, err = url.Parse(fmt.Sprintf("http://%v", r.Header.Get("Host")))
				if err != nil {
					log.Errorf("fallback generate url failed: %s", err)
				}
			}
			reqRaw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				log.Errorf("dump with transfer encoding failed: %s", err)
			}
			if len(reqRaw) > 0 {
				raws = append(raws, lowhttp.FixHTTPRequest(reqRaw))
			}
			continue
		}
		reqRaw, _ := utils.HttpDumpWithBody(r, true)
		if len(reqRaw) > 0 {
			raws = append(raws, reqRaw)
		}
	}

	if raws != nil && len(raws) > 1 {
		return &ypb.MutateResult{
			Result:       raws[0],
			ExtraResults: raws[1:],
		}, nil
	}

	if raws != nil && len(raws) == 1 {
		return &ypb.MutateResult{
			Result: raws[0],
		}, nil
	}

	return nil, utils.Errorf("empty result")
}

// 已弃用，使用 common\yak\yaklib\codec\codegrpc\codec_grpc_methods.go:HTTPRequestMutate
func (s *Server) HTTPRequestMutate(ctx context.Context, req *ypb.HTTPRequestMutateParams) (*ypb.MutateResult, error) {
	rawRequest := req.GetRequest()
	result := rawRequest
	method := strings.ToUpper(strings.Join(req.FuzzMethods, ""))
	// get params
	totalParams := lowhttp.GetFullHTTPRequestQueryParams(rawRequest)
	contentType := lowhttp.GetHTTPPacketHeader(rawRequest, "Content-Type")
	transferEncoding := lowhttp.GetHTTPPacketHeader(rawRequest, "Transfer-Encoding")
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rawRequest)
	// chunked 转 Content-Length
	if !req.ChunkEncode && utils.IContains(transferEncoding, "chunked") {
		result = lowhttp.ReplaceHTTPPacketBody(result, body, false)
		_, body = lowhttp.SplitHTTPHeadersAndBodyFromPacket(result)
	}
	// post params
	postParams, _, _ := lowhttp.GetParamsFromBody(contentType, body)
	if totalParams == nil {
		totalParams = make(map[string][]string)
	}
	for _, param := range postParams.Items {
		totalParams[param.Key] = append(totalParams[param.Key], param.Values...)
	}

	switch method {
	case "POST":
		result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result),
			poc.WithReplaceHttpPacketMethod("POST"),
			poc.WithReplaceHttpPacketQueryParamRaw(""),
			poc.WithReplaceHttpPacketHeader("Content-Type", "application/x-www-form-urlencoded"),
			poc.WithDeleteHeader("Transfer-Encoding"),
			poc.WithAppendHeaderIfNotExist("User-Agent", consts.DefaultUserAgent),
			poc.WithReplaceFullHttpPacketPostParamsWithoutEscape(totalParams),
		)

	default:
		if len(method) > 0 {
			result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result),
				poc.WithReplaceHttpPacketMethod(method),
				poc.WithReplaceFullHttpPacketQueryParamsWithoutEscape(totalParams),
				poc.WithDeleteHeader("Transfer-Encoding"),
				poc.WithDeleteHeader("Content-Type"),
				poc.WithAppendHeaderIfNotExist("User-Agent", consts.DefaultUserAgent),
				poc.WithReplaceHttpPacketBody(nil, false),
			)
		}
	}

	if req.ChunkEncode {
		// chunk编码
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(result)
		result = lowhttp.ReplaceHTTPPacketBody(result, body, true)
	}
	if req.UploadEncode {
		opts := make([]poc.PocConfigOption, 0)
		opts = append(opts, poc.WithReplaceHttpPacketBody(nil, false))
		opts = append(opts, poc.WithReplaceHttpPacketQueryParamRaw(""))
		if len(totalParams) > 0 {
			for k, values := range totalParams {
				for _, v := range values {
					opts = append(opts, poc.WithAppendHttpPacketUploadFile(k, "", v, ""))
				}
			}
		} else {
			opts = append(opts, poc.WithAppendHttpPacketUploadFile("key", "", "[value]", ""))
		}
		result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result), opts...)
	}

	return &ypb.MutateResult{
		Result:       result,
		ExtraResults: nil,
	}, nil
}

func (s *Server) HTTPResponseMutate(ctx context.Context, req *ypb.HTTPResponseMutateParams) (*ypb.MutateResult, error) {
	return nil, nil
}

// Deprecated
func (s *Server) QueryHistoryHTTPFuzzerTask(ctx context.Context, req *ypb.Empty) (*ypb.HistoryHTTPFuzzerTasks, error) {
	return &ypb.HistoryHTTPFuzzerTasks{Tasks: yakit.QueryFirst50WebFuzzerTask(s.GetProjectDatabase())}, nil
}

func (s *Server) QueryHistoryHTTPFuzzerTaskEx(ctx context.Context, req *ypb.QueryHistoryHTTPFuzzerTaskExParams) (*ypb.HistoryHTTPFuzzerTasksResponse, error) {
	paging, tasks, err := yakit.QueryFuzzerHistoryTasks(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	newTasks := funk.Map(tasks, func(i *schema.WebFuzzerTask) *ypb.HistoryHTTPFuzzerTaskDetail {
		return i.ToGRPCModelDetail()
	}).([]*ypb.HistoryHTTPFuzzerTaskDetail)
	return &ypb.HistoryHTTPFuzzerTasksResponse{
		Data:       newTasks,
		Total:      int64(paging.TotalRecord),
		TotalPage:  int64(paging.TotalPage),
		Pagination: req.GetPagination(),
	}, nil
}

func (s *Server) GetHistoryHTTPFuzzerTask(ctx context.Context, req *ypb.GetHistoryHTTPFuzzerTaskRequest) (*ypb.HistoryHTTPFuzzerTaskDetail, error) {
	task, err := yakit.GetWebFuzzerTaskById(s.GetProjectDatabase(), int(req.GetId()))
	if err != nil {
		return nil, err
	}
	var reqRaw ypb.FuzzerRequest
	err = json.Unmarshal([]byte(task.RawFuzzTaskRequest), &reqRaw)
	if err != nil {
		return nil, err
	}
	return &ypb.HistoryHTTPFuzzerTaskDetail{
		BasicInfo:     task.ToGRPCModel(),
		OriginRequest: &reqRaw,
	}, nil
}

func (s *Server) QueryHTTPFuzzerResponseByTaskId(ctx context.Context, req *ypb.QueryHTTPFuzzerResponseByTaskIdRequest) (*ypb.QueryHTTPFuzzerResponseByTaskIdResponse, error) {
	p, rets, err := yakit.QueryWebFuzzerResponse(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	var results []*ypb.FuzzerResponse
	for _, i := range rets {
		r, err := i.ToGRPCModel()
		if err != nil {
			continue
		}
		results = append(results, r)
	}

	return &ypb.QueryHTTPFuzzerResponseByTaskIdResponse{
		Pagination: req.Pagination,
		Data:       results,
		Total:      int64(p.TotalRecord),
		TotalPage:  int64(p.TotalPage),
	}, nil
}

func (s *Server) ExtractHTTPResponse(ctx context.Context, req *ypb.ExtractHTTPResponseParams) (*ypb.ExtractHTTPResponseResult, error) {
	if req.GetHTTPResponse() == "" {
		return nil, utils.Error("empty http response")
	}

	if len(req.GetExtractors()) == 0 {
		return nil, utils.Error("empty extractors")
	}

	/*
		type YakExtractor struct {
			Name             string // name or index
			Type             string
			Scope            string // header body all
			Groups           []string
			RegexpMatchGroup []int
			XPathAttribute   string
		}
	*/
	extractors := funk.Map(req.GetExtractors(), func(i *ypb.HTTPResponseExtractor) *httptpl.YakExtractor {
		return httptpl.NewExtractorFromGRPCModel(i)
	}).([]*httptpl.YakExtractor)

	params := make(map[string]interface{})
	for _, i := range extractors {
		p, err := i.ExecuteWithRequest([]byte(req.GetHTTPResponse()), []byte(req.GetHTTPRequest()), req.GetIsHTTPS(), params)
		if err != nil {
			log.Errorf("extractor %s execute failed: %s", i.Name, err)
			continue
		}
		for k, v := range p {
			params[k] = httptpl.ExtractResultToString(v)
		}
	}

	var results []*ypb.FuzzerParamItem
	for k, v := range params {
		results = append(results, &ypb.FuzzerParamItem{
			Key:   k,
			Value: httptpl.ExtractResultToString(v),
		})
	}
	return &ypb.ExtractHTTPResponseResult{Values: results}, nil
}

func (s *Server) MatchHTTPResponse(ctx context.Context, req *ypb.MatchHTTPResponseParams) (*ypb.MatchHTTPResponseResult, error) {
	if req.GetHTTPResponse() == "" {
		return nil, utils.Error("empty http response")
	}

	if len(req.GetMatchers()) == 0 {
		return nil, utils.Error("empty matchers")
	}

	/*
		type YakMatcher struct {
			MatcherType string
			// just for expr
			ExprType string

			// groups
			Scope         string
			Condition     string
			Group         []string
			GroupEncoding string

			Negative bool

			// or / and
			SubMatcherCondition string
			SubMatchers         []*YakMatcher
		}
	*/

	//matchers := funk.Map(req.GetMatchers(), func(i *ypb.HTTPResponseMatcher) *httptpl.YakMatcher {
	//	res := &httptpl.YakMatcher{
	//		MatcherType:   i.GetMatcherType(),
	//		ExprType:      i.GetExprType(),
	//		Scope:         i.GetScope(),
	//		Condition:     i.GetCondition(),
	//		Group:         i.GetGroup(),
	//		GroupEncoding: i.GetGroupEncoding(),
	//		Negative:      i.GetNegative(),
	//	}
	//	res.Format()
	//	return res
	//}).([]*httptpl.YakMatcher)
	matchers := httptpl.NewMatcherSliceFromGRPCModel(req.GetMatchers())

	matcher := &httptpl.YakMatcher{
		SubMatcherCondition: req.GetMatcherCondition(),
		SubMatchers:         matchers,
	}
	if matcher.SubMatcherCondition == "" {
		matcher.SubMatcherCondition = "and"
	}

	result, err := matcher.ExecuteRawResponse([]byte(req.GetHTTPResponse()), nil)
	if err != nil {
		return nil, err
	}
	return &ypb.MatchHTTPResponseResult{Matched: result}, nil
}

func (s *Server) RenderVariables(ctx context.Context, req *ypb.RenderVariablesRequest) (*ypb.RenderVariablesResponse, error) {
	vars := httptpl.NewVars()
	for _, kv := range req.GetParams() {
		vars.AutoSet(kv.GetKey(), kv.GetValue())
	}
	results := vars.ToMap()
	var finalResults []*ypb.KVPair
	for _, kv := range req.GetParams() {
		value, ok := results[kv.GetKey()]
		if !ok {
			continue
		}
		finalResults = append(finalResults, &ypb.KVPair{
			Key:   kv.GetKey(),
			Value: utils.EscapeInvalidUTF8Byte(utils.InterfaceToBytes(value)),
		})
	}

	var responseVars []*ypb.KVPair
	for k, v := range httptpl.LoadVarFromRawResponse(req.GetHTTPResponse(), 0) {
		responseVars = append(responseVars, &ypb.KVPair{
			Key:   k,
			Value: utils.EscapeInvalidUTF8Byte(utils.InterfaceToBytes(v)),
		})
	}
	sort.SliceStable(responseVars, func(i, j int) bool {
		return responseVars[i].Key < responseVars[j].Key
	})
	finalResults = append(finalResults, responseVars...)
	return &ypb.RenderVariablesResponse{Results: finalResults}, nil
}

func (s *Server) RenderVariablesWithTypedKV(ctx context.Context, kvs []*ypb.FuzzerParamItem) map[string]any {
	vars := httptpl.NewVars()
	for _, kv := range kvs {
		key, _ := kv.GetKey(), kv.GetValue()
		value := unmarshalValue(kv.GetMarshalValue())
		if kv.GetType() == "nuclei-dsl" {
			vars.SetAsNucleiTags(key, value)
		} else {
			vars.Set(key, value)
		}
	}
	return vars.ToMap()
}

func (s *Server) PreRenderVariables(ctx context.Context, params []*ypb.FuzzerParamItem, https, gmtls, syncTagIndex bool) chan map[string]any {
	resultsChan := make(chan map[string]any, 100)
	if len(params) <= 0 {
		resultsChan <- make(map[string]any)
		close(resultsChan)
		return resultsChan
	}

	l := make([][]any, len(params))
	idToParam := make(map[int]*ypb.FuzzerParamItem)
	hasNucleiTag := false
	paramsMap := lo.SliceToMap(params, func(item *ypb.FuzzerParamItem) (string, any) {
		if item.GetMarshalValue() == "" {
			return item.GetKey(), item.GetValue()
		}
		return item.GetKey(), unmarshalValue(item.GetMarshalValue())
	})

	for index, p := range params {
		_, valueStr := p.GetKey(), p.GetValue()
		var value any
		if p.GetMarshalValue() == "" {
			value = valueStr
		} else {
			value = unmarshalValue(p.GetMarshalValue())
		}

		typ := strings.TrimSpace(strings.ToLower(p.GetType()))
		idToParam[index] = p

		if typ == "fuzztag" {
			opts := mutate.FuzzFileOptions()
			opts = append(opts, mutate.Fuzz_WithParams(paramsMap))
			//if syncTagIndex {
			//	opts = append(opts, mutate.Fuzz_SyncTag(true))
			//}
			rets, _ := mutate.FuzzTagExec(valueStr, opts...)
			if len(rets) > 0 {
				anyRets := lo.Map(rets, func(item string, _ int) any {
					return item
				})
				l[index] = anyRets
				continue
			}
		} else if typ == "nuclei-dsl" {
			hasNucleiTag = true
		}

		l[index] = []any{value}
	}

	var count int64 = 0
	handlePayload := func(payloads []any) error {
		params := make([]*ypb.FuzzerParamItem, 0)
		resultMap := make(map[string]any)
		if hasNucleiTag {
			for index, v := range payloads {
				p := idToParam[index]
				key := p.GetKey()
				params = append(params, &ypb.FuzzerParamItem{Key: key, Value: codec.AnyToString(v), Type: p.GetType(), MarshalValue: marshalValue(v)})
			}
			resultMap = s.RenderVariablesWithTypedKV(ctx, params)
		} else {
			for index, v := range payloads {
				p := idToParam[index]
				key := p.GetKey()
				resultMap[key] = v
			}
		}

		atomic.AddInt64(&count, 1)
		resultsChan <- resultMap
		return nil
	}
	go func() {
		defer func() {
			if count <= 0 {
				resultsChan <- make(map[string]any)
				close(resultsChan)
				return
			}
			close(resultsChan)

			if err := recover(); err != nil {
				log.Errorf("cartesian to fuzztag vars failed: %v", err)
			}
		}()
		if syncTagIndex {
			for i := 0; ; i++ {
				ok := false
				payload := []any{}
				for _, group := range l {
					if i >= len(group) {
						payload = append(payload, "")
						continue
					}
					ok = true
					payload = append(payload, group[i])
				}
				if !ok {
					break
				}
				err := handlePayload(payload)
				if err != nil {
					log.Errorf("handle payload failed: %s", err)
				}
			}
		} else {
			err := cartesian.ProductExContext(ctx, l, handlePayload)
			if err != nil {
				log.Errorf("cartesian product failed: %s", err)
			}
		}
	}()
	return resultsChan
}

func (s *Server) GetSystemDefaultDnsServers(ctx context.Context, req *ypb.Empty) (*ypb.DefaultDnsServerResponse, error) {
	servers, err := utils.GetSystemDnsServers()
	return &ypb.DefaultDnsServerResponse{DefaultDnsServer: servers}, err
}

var (
	Action_Retain  = "retain"
	Action_Discard = "discard"
)

type YakFuzzerMatcher struct { // Added some display fields
	Matcher *httptpl.YakMatcher
	Color   string
	Action  string
}

func NewHttpFlowMatcherFromGRPCModel(m *ypb.HTTPResponseMatcher) *YakFuzzerMatcher {
	res := &YakFuzzerMatcher{
		Matcher: &httptpl.YakMatcher{
			MatcherType:         m.GetMatcherType(),
			ExprType:            m.GetExprType(),
			Scope:               m.GetScope(),
			Condition:           m.GetCondition(),
			Group:               m.GetGroup(),
			GroupEncoding:       m.GetGroupEncoding(),
			Negative:            m.GetNegative(),
			SubMatcherCondition: m.GetSubMatcherCondition(),
			SubMatchers:         funk.Map(m.GetSubMatchers(), httptpl.NewMatcherFromGRPCModel).([]*httptpl.YakMatcher),
		},
		Color:  m.GetHitColor(),
		Action: m.GetAction(),
	}
	return res
}

func MatchColor(m []*YakFuzzerMatcher, rsp *httptpl.RespForMatch, vars map[string]interface{}, suf ...string) (matched bool, hitColor []string, discard bool) {
	for _, flowMatcher := range m {
		startTime := time.Now()
		res, err := flowMatcher.Matcher.Execute(rsp, vars, suf...)
		elapsed := time.Since(startTime)
		if elapsed > time.Second {
			log.Infof("matcher execution took %v, cost is too heavy", elapsed)
		}
		if err != nil {
			log.Errorf("yak match err :%s", err)
		}

		if CheckShouldDiscard(flowMatcher.Action, res) { // if should discard, return directly
			matched = res
			if res {
				hitColor = append(hitColor, flowMatcher.Color)
			}
			discard = true
			return
		} else if res { // has not action and match success ,update match info
			matched = true
			hitColor = append(hitColor, flowMatcher.Color)
		}
	}
	return
}

func CheckShouldDiscard(action string, matchRes bool) bool {
	return (action == Action_Retain && !matchRes) || (action == Action_Discard && matchRes)
}

func SetFuzzerRespTraceInfo(resp *ypb.FuzzerResponse, traceInfo *lowhttp.LowhttpTraceInfo) {
	if traceInfo == nil {
		return
	}
	resp.TotalDurationMs = traceInfo.TotalTime.Milliseconds()
	resp.DurationMs = traceInfo.ServerTime.Milliseconds()
	resp.FirstByteDurationMs = traceInfo.ServerTime.Milliseconds()
	resp.DNSDurationMs = traceInfo.DNSTime.Milliseconds()
	resp.TLSHandshakeDurationMs = traceInfo.TLSHandshakeTime.Milliseconds()
	resp.TCPDurationMs = traceInfo.TCPTime.Milliseconds()
	resp.ConnectDurationMs = traceInfo.ConnTime.Milliseconds()
}
