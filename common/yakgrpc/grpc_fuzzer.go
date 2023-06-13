package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/saintfish/chardet"
	uuid "github.com/satori/go.uuid"
)

func Chardet(raw []byte) string {
	res, err := chardet.NewTextDetector().DetectBest(raw)
	if err != nil {
		return "utf-8"
	}
	return res.Charset
}

func (s *Server) ExtractUrl(ctx context.Context, req *ypb.FuzzerRequest) (*ypb.ExtractedUrl, error) {
	u, err := lowhttp.ExtractURLFromHTTPRequestRaw([]byte(req.Request), req.GetIsHTTPS())
	if err != nil {
		return nil, err
	}
	return &ypb.ExtractedUrl{Url: u.String()}, nil
}

func (s *Server) StringFuzzer(rootCtx context.Context, req *ypb.StringFuzzerRequest) (*ypb.StringFuzzerResponse, error) {
	max := req.GetLimit()
	timeoutSeconds := req.GetTimeoutSeconds()
	var ctx = rootCtx
	var cancel = func() {}
	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(rootCtx, time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	var res [][]byte
	var counter int64
	_, _ = mutate.QuickMutateWithCallbackEx2(
		req.GetTemplate(), s.GetProfileDatabase(), []func(*mutate.MutateResult) bool{
			func(mutateResult *mutate.MutateResult) bool {
				select {
				case <-ctx.Done():
					return false
				default:
					if max > 0 && counter >= max {
						return false
					}
				}
				counter++

				res = append(res, []byte(mutateResult.Result))
				return true
			},
		},
		yak.MutateWithYaklang(req.GetHotPatchCode()),
		yak.MutateWithParamsGetter(req.GetHotPatchCodeWithParamGetter())(),
	)
	//mutate.MutateWithConditions(req.GetTemplate(), func(finalResult *mutate.MutateResult) bool {
	//	select {
	//	case <-ctx.Done():
	//		return false
	//	default:
	//		if max > 0 && counter >= max {
	//			return false
	//		}
	//	}
	//	_, _ = mutate.QuickMutateWithCallbackEx2(
	//		finalResult.Result, s.db, []func(*mutate.MutateResult) bool{
	//			func(mutateResult *mutate.MutateResult) bool {
	//				select {
	//				case <-ctx.Done():
	//					return false
	//				default:
	//					if max > 0 && counter >= max {
	//						return false
	//					}
	//				}
	//				counter++
	//
	//				res = append(res, []byte(mutateResult.Result))
	//				return true
	//			},
	//		},
	//		yak.MutateWithYaklang(req.GetHotPatchCode()),
	//	)
	//	return true
	//}, yak.MutateWithParamsGetter(req.GetHotPatchCodeWithParamGetter())())

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
	_ = isHttps
	newUrl := lowhttp.MergeUrlFromHTTPRequest([]byte(req.GetRequest()), result, isHttps)
	resultRequest := lowhttp.UrlToGetRequestPacket(newUrl, []byte(req.GetRequest()), isHttps, lowhttp.ExtractCookieJarFromHTTPResponse([]byte(req.GetResponse()))...)
	if resultRequest == nil {
		return nil, utils.Errorf("cannot merge request packet. redirect url: %s", newUrl)
	}

	start := time.Now()
	host, port, _ := utils.ParseStringToHostPort(newUrl)
	rspRaw, _, err := lowhttp.SendHTTPRequestWithRawPacketWithRedirect(
		isHttps, host, port, resultRequest,
		utils.FloatSecondDuration(req.GetPerRequestTimeoutSeconds()), 0,
		utils.PrettifyListFromStringSplited(req.GetProxy(), ",")...)
	if err != nil {
		return nil, err
	}

	var rsp = &ypb.FuzzerResponse{
		Method:                "GET",
		ResponseRaw:           rspRaw,
		GuessResponseEncoding: Chardet(rspRaw),
		RequestRaw:            resultRequest,
	}
	rsp.UUID = uuid.NewV4().String()
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

func (s *Server) HTTPFuzzer(req *ypb.FuzzerRequest, stream ypb.Yak_HTTPFuzzerServer) (finalError error) {
	defer func() {
		if err := recover(); err != nil {
			finalError = utils.Errorf("panic from httpfuzzer: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	var mergedParams = make(map[string]interface{})
	renderedParams, err := s.RenderVariables(stream.Context(), &ypb.RenderVariablesRequest{
		Params: funk.Map(req.GetParams(), func(i *ypb.FuzzerParamItem) *ypb.KVPair {
			return &ypb.KVPair{Key: i.GetKey(), Value: i.GetValue()}
		}).([]*ypb.KVPair),
		IsHTTPS: req.GetIsHTTPS(),
		IsGmTLS: req.GetIsGmTLS(),
	})
	if err != nil {
		return utils.Errorf("render variables failed: %v", err)
	}
	for _, kv := range renderedParams.GetResults() {
		mergedParams[kv.GetKey()] = kv.GetValue()
	}

	if req.GetHistoryWebFuzzerId() > 0 {
		for resp := range yakit.YieldWebFuzzerResponses(s.GetProjectDatabase(), stream.Context(), int(req.GetHistoryWebFuzzerId())) {
			rsp, err := resp.ToGRPCModel()
			if err != nil {
				continue
			}
			err = stream.Send(rsp)
			if err != nil {
				log.Errorf("stream send failed: %s", err)
				continue
			}
		}
		return nil
	}
	if req.GetRequest() == "" && len(req.GetRequestRaw()) <= 0 {
		return utils.Errorf("empty request is not allowed")
	}

	var proxies = utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(req.GetProxy(), ","))
	var concurrent = req.GetConcurrent()
	if concurrent <= 0 {
		concurrent = 20
	}
	var timeoutSeconds = req.GetPerRequestTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}

	task, err := yakit.SaveWebFuzzerTask(s.GetProjectDatabase(), req, 0, false, "executing...")
	if err != nil {
		return utils.Errorf("save to web fuzzer to database failed: %s", err)
	}
	var taskId = task.ID
	_ = task
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

	var rawRequest []byte
	if len(req.GetRequestRaw()) > 0 {
		rawRequest = req.GetRequestRaw()
	} else {
		rawRequest = []byte(req.GetRequest())
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

	var inStatusCode = utils.ParseStringToPorts(req.GetRetryInStatusCode())
	var notInStatusCode = utils.ParseStringToPorts(req.GetRetryNotInStatusCode())

	var httpTplMatcher = make([]*httptpl.YakMatcher, len(req.GetMatchers()))
	var httpTplExtractor = make([]*httptpl.YakExtractor, len(req.GetExtractors()))
	var haveHTTPTplMatcher = len(httpTplMatcher) > 0
	var haveHTTPTplExtractor = len(httpTplExtractor) > 0
	if haveHTTPTplExtractor {
		for i, e := range req.GetExtractors() {
			httpTplExtractor[i] = httptpl.NewExtractorFromGRPCModel(e)
		}
	}

	if haveHTTPTplMatcher {
		for i, m := range req.GetMatchers() {
			httpTplMatcher[i] = httptpl.NewMatcherFromGRPCModel(m)
		}
	}

	if req.GetRepeatTimes() > 0 {
		var buf bytes.Buffer
		buf.WriteString("{{repeat(" + fmt.Sprint(req.GetRepeatTimes()) + ")}}")
		buf.Write(rawRequest)
		rawRequest = buf.Bytes()
	}
	res, err := mutate.ExecPool(
		rawRequest,
		mutate.WithPoolOpt_ForceFuzz(req.GetForceFuzz()),
		mutate.WithPoolOpt_Timeout(timeoutSeconds),
		mutate.WithPoolOpt_Proxy(proxies...),
		mutate.WithPoolOpt_Concurrent(int(concurrent)),
		mutate.WithPoolOpt_Addr(req.GetActualAddr(), req.GetIsHTTPS()),
		mutate.WithPoolOpt_RawMode(true),
		mutate.WithPoolOpt_Https(req.GetIsHTTPS()),
		mutate.WithPoolOpt_GmTLS(req.GetIsGmTLS()),
		mutate.WithPoolOpt_Context(stream.Context()),
		mutate.WithPoolOpt_NoFollowRedirect(req.GetNoFollowRedirect()),
		mutate.WithPoolOpt_FollowJSRedirect(req.GetFollowJSRedirect()),
		mutate.WithPoolOpt_RedirectTimes(int(req.GetRedirectTimes())),
		mutate.WithPoolOpt_noFixContentLength(req.GetNoFixContentLength()),
		mutate.WithPoolOpt_ExtraMutateConditionGetter(yak.MutateWithParamsGetter(
			req.GetHotPatchCodeWithParamGetter()),
		),
		mutate.WithPoolOpt_ExtraMutateCondition(yak.MutateWithYaklang(req.GetHotPatchCode())),
		mutate.WithPoolOpt_DelayMinSeconds(req.GetDelayMinSeconds()),
		mutate.WithPoolOPt_DelayMaxSeconds(req.GetDelayMaxSeconds()),
		mutate.WithPoolOpt_HookCodeCaller(yak.MutateHookCaller(req.GetHotPatchCode())),
		mutate.WithPoolOpt_Source("webfuzzer"),
		mutate.WithPoolOpt_RetryTimes(int(req.GetMaxRetryTimes())),
		mutate.WithPoolOpt_RetryInStatusCode(inStatusCode),
		mutate.WithPoolOpt_RetryNotInStatusCode(notInStatusCode),
		mutate.WithPoolOpt_RetryWaitTime(req.GetRetryWaitSeconds()),
		mutate.WithPoolOpt_RetryMaxWaitTime(req.GetRetryMaxWaitSeconds()),
		mutate.WithPoolOpt_DNSServers(req.GetDNSServers()),
		mutate.WithPoolOpt_EtcHosts(req.GetEtcHosts()),
		mutate.WithPoolOpt_NoSystemProxy(req.GetNoSystemProxy()),
		mutate.WithPoolOpt_FuzzParams(mergedParams),
	)
	if err != nil {
		task.Ok = false
		task.Reason = utils.Errorf("exec http pool failed: %s", err).Error()
		return err
	}

	// 可以用于计算相似度
	var firstHeader, firstBody []byte
	for result := range res {
		task.HTTPFlowTotal++
		var payloads = make([]string, len(result.Payloads))
		for i, payload := range result.Payloads {
			if len(payload) > 100 {
				payload = payload[:100] + "..."
			}
			payloads[i] = utils.ParseStringToVisible(payload)
		}

		if result.Error != nil {
			rsp := &ypb.FuzzerResponse{}
			rsp.UUID = uuid.NewV4().String()
			rsp.Url = utils.EscapeInvalidUTF8Byte([]byte(result.Url))
			rsp.Ok = false
			rsp.Reason = result.Error.Error()
			rsp.TaskId = int64(taskId)
			rsp.Payloads = payloads
			if result.LowhttpResponse != nil && result.LowhttpResponse.TraceInfo != nil {
				rsp.TotalDurationMs = result.LowhttpResponse.TraceInfo.TotalTime.Milliseconds()
				rsp.DurationMs = result.LowhttpResponse.TraceInfo.ServerTime.Milliseconds()
				rsp.FirstByteDurationMs = result.LowhttpResponse.TraceInfo.ServerTime.Milliseconds()
				rsp.DNSDurationMs = result.LowhttpResponse.TraceInfo.DNSTime.Milliseconds()
				rsp.Proxy = result.LowhttpResponse.Proxy
				rsp.RemoteAddr = result.LowhttpResponse.RemoteAddr
			}

			task.HTTPFlowFailedCount++
			yakit.SaveWebFuzzerResponse(s.GetProjectDatabase(), int(task.ID), rsp)
			_ = stream.Send(rsp)
			continue
		}

		var extractorResults []*ypb.KVPair
		if haveHTTPTplExtractor {
			for _, extractor := range httpTplExtractor {
				vars, err := extractor.Execute(result.ResponseRaw)
				if err != nil {
					log.Errorf("extractor execute failed: %s", err)
					continue
				}
				for k, v := range vars {
					extractorResults = append(extractorResults, &ypb.KVPair{Key: k, Value: utils.InterfaceToString(v)})
				}
			}
		}

		var httpTPLmatchersResult bool
		if haveHTTPTplMatcher && result.LowhttpResponse != nil {
			cond := "and"
			switch ret := strings.ToLower(req.GetMatchersCondition()); ret {
			case "or", "and":
				cond = ret
			default:
			}
			ins := &httptpl.YakMatcher{
				SubMatcherCondition: cond,
				SubMatchers:         httpTplMatcher,
			}
			matcherParams := utils.CopyMapInterface(mergedParams)
			for _, kv := range extractorResults {
				matcherParams[kv.GetKey()] = kv.GetValue()
			}
			httpTPLmatchersResult, err = ins.Execute(result.LowhttpResponse, matcherParams)
			if finalError != nil {
				log.Errorf("httptpl.YakMatcher execute failed: %s", err)
			}
		}

		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(result.ResponseRaw)
		if len(body) > 2*1024*1024 {
			body = body[:2*1024*1024]
			body = append(body, []byte("...chunked by yakit web fuzzer")...)
		}

		if !req.GetNoFixContentLength() && (result.Request != nil && result.Request.ProtoMajor != 2) { // no fix for h2 rsp
			result.ResponseRaw = lowhttp.ReplaceHTTPPacketBody(result.ResponseRaw, body, false)
			result.Response, _ = lowhttp.ParseStringToHTTPResponse(string(result.ResponseRaw))
		}

		if len(result.RequestRaw) > 1*1024*1024 {
			result.RequestRaw = result.RequestRaw[:1*1024*1024]
			result.RequestRaw = append(result.RequestRaw, []byte("...chunked by yakit web fuzzer")...)
		}

		task.HTTPFlowSuccessCount++
		var rsp = &ypb.FuzzerResponse{
			Url:                   utils.EscapeInvalidUTF8Byte([]byte(result.Url)),
			Method:                utils.EscapeInvalidUTF8Byte([]byte(result.Request.Method)),
			ResponseRaw:           result.ResponseRaw,
			GuessResponseEncoding: Chardet(result.ResponseRaw),
			RequestRaw:            result.RequestRaw,
			Payloads:              payloads,
			IsHTTPS:               strings.HasPrefix(strings.ToLower(result.Url), "https://"),
			ExtractedResults:      extractorResults,
			MatchedByMatcher:      httpTPLmatchersResult,
		}
		// 处理额外时间
		if result.LowhttpResponse != nil && result.LowhttpResponse.TraceInfo != nil {
			rsp.TotalDurationMs = result.LowhttpResponse.TraceInfo.TotalTime.Milliseconds()
			rsp.DurationMs = result.LowhttpResponse.TraceInfo.ServerTime.Milliseconds()
			rsp.FirstByteDurationMs = result.LowhttpResponse.TraceInfo.ServerTime.Milliseconds()
			rsp.DNSDurationMs = result.LowhttpResponse.TraceInfo.DNSTime.Milliseconds()
			rsp.Proxy = result.LowhttpResponse.Proxy
			rsp.RemoteAddr = result.LowhttpResponse.RemoteAddr
		}
		if rsp.ResponseRaw != nil {
			// 处理结果，相似度
			header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp.ResponseRaw)
			if firstHeader == nil {
				log.Infof("start to set first header[%v]...", result.Url)
				firstHeader = []byte(header)
				rsp.HeaderSimilarity = 1.0
			} else {
				rsp.HeaderSimilarity = utils.CalcSimilarity(firstHeader, []byte(header))
			}

			if firstBody == nil {
				log.Infof("start to set first body[%v]...", result.Url)
				firstBody = body
				rsp.BodySimilarity = 1.0
			} else {
				rsp.BodySimilarity = utils.CalcSimilarity(firstBody, body)
			}
		}

		rsp.UUID = uuid.NewV4().String()
		rsp.Timestamp = result.Timestamp
		rsp.DurationMs = result.DurationMs
		rsp.Host = utils.EscapeInvalidUTF8Byte([]byte(result.Request.Header.Get("Host")))
		if rsp.Host == "" {
			rsp.Host = result.Request.Host
		}
		rsp.Host = utils.EscapeInvalidUTF8Byte([]byte(utils.ParseStringToVisible(result.Request.Host)))

		if result.Response != nil {
			rsp.Ok = true
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
		yakit.SaveWebFuzzerResponse(s.GetProjectDatabase(), int(task.ID), rsp)
		rsp.TaskId = int64(taskId)
		err := stream.Send(rsp)
		if err != nil {
			log.Errorf("send to client failed: %s", err)
			continue
		}
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
			reqRaw, err := httputil.DumpRequest(r, true)
			if err != nil {
				log.Errorf("dump with transfer encoding failed: %s", err)
			}
			if len(reqRaw) > 0 {
				raws = append(raws, lowhttp.FixHTTPRequestOut(reqRaw))
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

func (s *Server) HTTPRequestMutate(ctx context.Context, req *ypb.HTTPRequestMutateParams) (*ypb.MutateResult, error) {
	freq, err := mutate.NewFuzzHTTPRequest(lowhttp.TrimLeftHTTPPacket(req.Request))
	if err != nil {
		return nil, utils.Errorf("build fuzzer request failed: %s", err)
	}

	switch strings.Join(req.FuzzMethods, "") {
	case "POST":
		u, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req.GetRequest(), true)
		if u != nil {
			reqs, _ := freq.FuzzMethod(
				"POST",
			).FuzzGetParamsRaw(
				"",
			).FuzzHTTPHeader(
				"Content-Type", "application/x-www-form-urlencoded",
			).FuzzHTTPHeader(
				"Transfer-Encoding", "",
			).FuzzHTTPHeader(
				"User-Agent", consts.DefaultUserAgent,
			).FuzzPostRaw(
				u.RawQuery,
			).Results()
			if len(reqs) > 0 {
				return requestToMutateResult(reqs, false)
			}
		}
	case "HEAD":
		fallthrough
	case "GET":
		u, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req.GetRequest(), true)
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req.GetRequest())
		if u != nil {
			var params = make(url.Values)
			values, _ := url.ParseQuery(u.RawQuery)
			if values != nil {
				for k, v := range values {
					params[k] = v
				}
			}
			postValue, _ := url.ParseQuery(strings.TrimSpace(string(body)))
			if postValue != nil {
				for k, v := range postValue {
					params[k] = v
				}
			}

			reqs, _ := freq.FuzzMethod(
				strings.ToUpper(strings.Join(req.GetFuzzMethods(), "")),
			).FuzzPath(
				u.Path,
			).FuzzHTTPHeader(
				"Content-Type", "",
			).FuzzHTTPHeader(
				"Transfer-Encoding", "",
			).FuzzGetParamsRaw(params.Encode()).FuzzHTTPHeader(
				"User-Agent", consts.DefaultUserAgent,
			).FuzzPostRaw("").Results()
			if len(reqs) > 0 {
				return requestToMutateResult(reqs, false)
			}
		}
	}

	if len(req.FuzzMethods) > 0 {
		reqs, err := freq.FuzzMethod(req.FuzzMethods...).Results()
		if err != nil {
			return nil, err
		}
		return requestToMutateResult(reqs, false)
	}

	// 获取 body
	reqInstance, err := freq.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	bodyRaw, err := ioutil.ReadAll(reqInstance.Body)
	if err != nil {
		return nil, err
	}
	if bodyRaw == nil {
		return nil, utils.Errorf("empty body")
	}

	// 获取 chunk encode
	if req.ChunkEncode {
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req.GetRequest())
		reqRaw := lowhttp.ReplaceHTTPPacketBody(req.GetRequest(), body, true)
		return &ypb.MutateResult{Result: reqRaw}, nil
	}

	if req.UploadEncode {
		freq, err := mutate.NewFuzzHTTPRequest(lowhttp.TrimLeftHTTPPacket(req.Request))
		if err != nil {
			return nil, utils.Errorf("build fuzz.HTTPRequest failed: %s", err)
		}
		kv := make(map[string]interface{})
		for _, getParam := range freq.GetGetQueryParams() {
			kv[getParam.Name()] = getParam.Value()
		}

		for _, postRawParam := range freq.GetPostParams() {
			kv[postRawParam.Name()] = postRawParam.Value()
		}

		currentPair := freq.FuzzMethod("POST")
		for k, v := range kv {
			currentPair = currentPair.FuzzUploadKVPair(k, v)
		}
		reqs, err := currentPair.Results()
		if err != nil {
			return nil, err
		}
		return requestToMutateResult(reqs, false)
	}

	return &ypb.MutateResult{
		Result:       []byte(req.Request),
		ExtraResults: nil,
	}, nil
}

func (s *Server) HTTPResponseMutate(ctx context.Context, req *ypb.HTTPResponseMutateParams) (*ypb.MutateResult, error) {
	return nil, nil
}

func (s *Server) QueryHistoryHTTPFuzzerTask(ctx context.Context, req *ypb.Empty) (*ypb.HistoryHTTPFuzzerTasks, error) {
	return &ypb.HistoryHTTPFuzzerTasks{Tasks: yakit.QueryFirst50WebFuzzerTask(s.GetProjectDatabase())}, nil
}

func (s *Server) QueryHistoryHTTPFuzzerTaskEx(ctx context.Context, req *ypb.QueryHistoryHTTPFuzzerTaskExParams) (*ypb.HistoryHTTPFuzzerTasksResponse, error) {
	paging, tasks, err := yakit.QueryFuzzerHistoryTasks(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	newTasks := funk.Map(tasks, func(i *yakit.WebFuzzerTask) *ypb.HistoryHTTPFuzzerTaskDetail {
		return i.ToSwaggerModelDetail()
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
		BasicInfo:     task.ToSwaggerModel(),
		OriginRequest: &reqRaw,
	}, nil
}

func (s *Server) QueryHTTPFuzzerResponseByTaskIdRequest(ctx context.Context, req *ypb.QueryHTTPFuzzerResponseByTaskIdRequest) (*ypb.QueryHTTPFuzzerResponseByTaskIdResponse, error) {
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
		return &httptpl.YakExtractor{
			Name:   i.GetName(),
			Type:   i.GetType(),
			Scope:  i.GetScope(),
			Groups: i.GetGroups(),
			RegexpMatchGroup: funk.Map(i.GetRegexpMatchGroup(), func(i int64) int {
				return int(i)
			}).([]int),
			XPathAttribute: i.GetXPathAttribute(),
		}
	}).([]*httptpl.YakExtractor)

	var params = make(map[string]interface{})
	for _, i := range extractors {
		p, err := i.Execute([]byte(req.GetHTTPResponse()))
		if err != nil {
			log.Errorf("extractor %s execute failed: %s", i.Name, err)
			continue
		}
		for k, v := range p {
			params[k] = v
		}
	}

	var results []*ypb.FuzzerParamItem
	for k, v := range params {
		results = append(results, &ypb.FuzzerParamItem{
			Key:   k,
			Value: utils.InterfaceToString(v),
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
	matchers := funk.Map(req.GetMatchers(), func(i *ypb.HTTPResponseMatcher) *httptpl.YakMatcher {
		return &httptpl.YakMatcher{
			MatcherType:   i.GetMatcherType(),
			ExprType:      i.GetExprType(),
			Scope:         i.GetScope(),
			Condition:     i.GetCondition(),
			Group:         i.GetGroup(),
			GroupEncoding: i.GetGroupEncoding(),
			Negative:      i.GetNegative(),
		}
	}).([]*httptpl.YakMatcher)

	matcher := &httptpl.YakMatcher{
		MatcherType: req.GetMatcherCondition(),
		SubMatchers: matchers,
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
	var results = vars.ToMap()
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
