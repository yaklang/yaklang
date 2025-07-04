package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/fuzztagx"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ImportHTTPFuzzerTaskFromYaml yaml -> yakTemplate -> fuzzerRequest
func (s *Server) ImportHTTPFuzzerTaskFromYaml(ctx context.Context, req *ypb.ImportHTTPFuzzerTaskFromYamlRequest) (*ypb.ImportHTTPFuzzerTaskFromYamlResponse, error) {
	var fuzzerRequest []*ypb.FuzzerRequest
	content := req.GetYamlContent()
	if content == "" {
		return nil, errors.New("yaml content is empty")
	}
	// 转Template
	yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(content)
	warningMsgStr := ""
	if err := yakTemplate.CheckTemplateRisks(); err != nil {
		warningMsgStr = err.Error()
	}

	if err != nil {
		return nil, utils.Errorf("cannot create yak template from yaml: %v", err)
	}

	extractorMap := map[int][]*ypb.HTTPResponseExtractor{}
	matcherMap := map[int][]*ypb.HTTPResponseMatcher{}
	// 转FuzzerRequest
	seqs := yakTemplate.GenerateRequestSequences("http://www.example.com", false)
	for _, sequence := range seqs {
		fuzzerReqs := []*ypb.FuzzerRequest{}
		for _, request := range sequence.Requests {
			fuzzerReqs = append(fuzzerReqs, &ypb.FuzzerRequest{
				Request:                  string(request.Raw),
				RequestRaw:               request.Raw,
				PerRequestTimeoutSeconds: request.Timeout.Seconds(),
			})
		}
		config := sequence.RequestConfig
		funk.Map(config.Extractor, func(extractor *httptpl.YakExtractor) *ypb.HTTPResponseExtractor {
			typeName := ""
			switch extractor.Type {
			case "dsl":
				typeName = "nuclei-dsl"
			case "key-value":
				typeName = "kval"
			default:
				typeName = extractor.Type
			}
			httpExtractor := &ypb.HTTPResponseExtractor{
				Name:             extractor.Name,
				Type:             typeName,
				Scope:            extractor.Scope,
				Groups:           extractor.Groups,
				RegexpMatchGroup: funk.Map(extractor.RegexpMatchGroup, func(n int) int64 { return int64(n) }).([]int64),
				XPathAttribute:   extractor.XPathAttribute,
			}
			if _, ok := extractorMap[extractor.Id]; ok {
				extractorMap[extractor.Id] = append(extractorMap[extractor.Id], httpExtractor)
			} else {
				extractorMap[extractor.Id] = []*ypb.HTTPResponseExtractor{httpExtractor}
			}
			return httpExtractor
		})
		var yakMatchers2HttpResponseMatchers func(matchers []*httptpl.YakMatcher) []*ypb.HTTPResponseMatcher
		yakMatchers2HttpResponseMatchers = func(matchers []*httptpl.YakMatcher) []*ypb.HTTPResponseMatcher {
			return funk.Map(matchers, func(matcher *httptpl.YakMatcher) *ypb.HTTPResponseMatcher {
				scope := ""
				switch matcher.Scope {
				case "status":
					scope = "status_code"
				case "header":
					scope = "all_headers"
				default:
					scope = matcher.Scope
				}
				condition := matcher.Condition
				if matcher.Condition == "" {
					condition = "or"
				}
				httpMatcher := &ypb.HTTPResponseMatcher{
					SubMatchers:         yakMatchers2HttpResponseMatchers(matcher.SubMatchers),
					SubMatcherCondition: matcher.SubMatcherCondition,
					MatcherType:         matcher.MatcherType,
					Scope:               scope,
					Condition:           condition,
					Group:               matcher.Group,
					GroupEncoding:       matcher.GroupEncoding,
					Negative:            matcher.Negative,
					ExprType:            matcher.ExprType,
				}
				if _, ok := matcherMap[matcher.Id]; ok {
					matcherMap[matcher.Id] = append(matcherMap[matcher.Id], httpMatcher)
				} else {
					matcherMap[matcher.Id] = []*ypb.HTTPResponseMatcher{httpMatcher}
				}
				return httpMatcher
			}).([]*ypb.HTTPResponseMatcher)
		}
		// var matchers []*ypb.HTTPResponseMatcher
		var matchersCondition string
		if config.Matcher != nil {
			if len(config.Matcher.SubMatchers) > 0 {
				yakMatchers2HttpResponseMatchers(config.Matcher.SubMatchers)
				matchersCondition = config.Matcher.SubMatcherCondition
			} else {
				yakMatchers2HttpResponseMatchers([]*httptpl.YakMatcher{config.Matcher})
				matchersCondition = config.Matcher.SubMatcherCondition
			}
		}
		var redirectTimes float64
		if config.EnableRedirect {
			redirectTimes = float64(config.MaxRedirects)
		} else {
			redirectTimes = 0
		}
		noFixContentLength := config.NoFixContentLength

		vars := yakTemplate.Variables.ToMap()
		var params []*ypb.FuzzerParamItem
		for k, v := range vars {
			params = append(params, &ypb.FuzzerParamItem{
				Key:   k,
				Value: utils.InterfaceToString(v),
				Type:  "nuclei-dsl",
			})
		}
		//fuzzerReq.IsHTTPS = sequence.IsHTTPS
		//fuzzerReq.IsGmTLS = sequence.IsGmTLS
		//fuzzerReq.ActualAddr = sequence.Host
		//fuzzerReq.Proxy = sequence.Proxy
		//fuzzerReq.NoSystemProxy = sequence.NoSystemProxy
		//
		//fuzzerReq.ForceFuzz = sequence.ForceFuzz
		//noFixContentLength := sequence.NoFixContentLength
		//fuzzerReq.PerRequestTimeoutSeconds = sequence.RequestTimeout
		//
		//fuzzerReq.RepeatTimes = sequence.RepeatTimes
		//fuzzerReq.Concurrent = sequence.Concurrent
		//fuzzerReq.DelayMinSeconds = sequence.DelayMinSeconds
		//fuzzerReq.DelayMaxSeconds = sequence.DelayMaxSeconds
		//
		//fuzzerReq.MaxRetryTimes = sequence.MaxRetryTimes
		//fuzzerReq.RetryInStatusCode = sequence.RetryInStatusCode
		//fuzzerReq.RetryNotInStatusCode = sequence.RetryNotInStatusCode
		//
		noFollowRedirect := !config.EnableRedirect
		//
		//fuzzerReq.FollowJSRedirect = sequence.JsEnableRedirect
		//fuzzerReq.DNSServers = sequence.DNSServers

		inheritCookies := config.CookieInherit
		// inheritVariables := sequence.InheritVariables

		for index, fuzzerReq := range fuzzerReqs {
			if extractorMap[index+1] != nil {
				fuzzerReq.Extractors = extractorMap[index+1]
			}
			reqMatcher := &ypb.HTTPResponseMatcher{
				SubMatcherCondition: matchersCondition,
			}
			if matcherMap[index+1] != nil {
				reqMatcher.SubMatchers = matcherMap[index+1]
			}
			// if not set id
			if index == len(fuzzerReqs)-1 {
				if extractorMap[0] != nil {
					fuzzerReq.Extractors = extractorMap[0]
				}
				if matcherMap[0] != nil {
					reqMatcher.SubMatchers = matcherMap[0]
				}
			}

			fuzzerReq.Matchers = []*ypb.HTTPResponseMatcher{reqMatcher}
			fuzzerReq.NoFixContentLength = noFixContentLength
			fuzzerReq.NoFollowRedirect = noFollowRedirect
			fuzzerReq.RedirectTimes = redirectTimes
			fuzzerReq.InheritCookies = inheritCookies
			fuzzerReq.InheritVariables = true
			for name, v := range config.Payloads.GetRawPayloads() {
				params = append(params, &ypb.FuzzerParamItem{
					Key:   name,
					Value: strings.Join(v.Data, "\n"),
					Type:  "nuclei-dsl",
				})
			}
			fuzzerReq.Params = params
			fuzzerRequest = append(fuzzerRequest, fuzzerReq)
		}
	}

	result := &ypb.ImportHTTPFuzzerTaskFromYamlResponse{
		Requests: &ypb.FuzzerRequests{
			Requests: fuzzerRequest,
		},
		Status: &ypb.GeneralResponse{
			Ok:     true,
			Reason: warningMsgStr,
		},
	}
	return result, nil
}

// ExportHTTPFuzzerTaskToYaml fuzzerRequest -> yakTemplate -> yaml
func (s *Server) ExportHTTPFuzzerTaskToYaml(ctx context.Context, req *ypb.ExportHTTPFuzzerTaskToYamlRequest) (*ypb.ExportHTTPFuzzerTaskToYamlResponse, error) {
	res := &ypb.GeneralResponse{
		Ok:     true,
		Reason: "",
	}
	templateType := req.GetTemplateType()
	// 转换为YakTemplate
	seq := req.GetRequests()
	// Matcher转换
	var HttpResponseMatchers2YakMatchers func(matchers []*ypb.HTTPResponseMatcher) []*httptpl.YakMatcher
	HttpResponseMatchers2YakMatchers = func(matchers []*ypb.HTTPResponseMatcher) []*httptpl.YakMatcher {
		return funk.Map(matchers, func(matcher *ypb.HTTPResponseMatcher) *httptpl.YakMatcher {
			scope := ""
			switch matcher.Scope {
			case "status_code":
				scope = "status"
			case "all_headers":
				scope = "header"
			default:
				scope = matcher.Scope
			}
			return &httptpl.YakMatcher{
				SubMatchers:         HttpResponseMatchers2YakMatchers(matcher.SubMatchers),
				SubMatcherCondition: matcher.SubMatcherCondition,
				MatcherType:         matcher.MatcherType,
				Scope:               scope,
				Condition:           matcher.Condition,
				Group:               matcher.Group,
				GroupEncoding:       matcher.GroupEncoding,
				Negative:            matcher.Negative,
				ExprType:            matcher.ExprType,
			}
		}).([]*httptpl.YakMatcher)
	}
	// 生成请求桶
	// requestBulks := []*httptpl.YakRequestBulkConfig{}
	bulk := &httptpl.YakRequestBulkConfig{}
	bulk.Matcher = &httptpl.YakMatcher{
		SubMatcherCondition: "and",
	}
	vars := httptpl.NewVars()
	for index, request := range seq.GetRequests() {
		for _, param := range request.Params {
			if err := vars.SetWithType(param.Key, param.Value, param.Type); err != nil {
				log.Errorf("set vars error: %v", err)
			}
		}

		etcHosts := map[string]string{}
		for _, pair := range request.EtcHosts {
			etcHosts[pair.Key] = pair.Value
		}

		isRenderFuzzTag := request.ForceFuzz || request.GetFuzzTagMode() != "close"
		varToFuzztagMap := make(map[string]string)
		switch templateType {
		case "path":
			url, err := lowhttp.ExtractURLFromHTTPRequestRaw(request.RequestRaw, false)
			if err != nil {
				log.Error(err)
			}
			// rootPath := utils.ParseStringUrlToWebsiteRootPath(url.Path)
			// path := strings.Replace(url.Path, rootPath, "{{RootUrl}}", 1)
			path := url.Path
			body, headers := string(lowhttp.GetHTTPPacketBody(request.RequestRaw)), lowhttp.GetHTTPPacketHeaders(request.RequestRaw)
			method, _, _ := lowhttp.GetHTTPPacketFirstLine(request.RequestRaw)

			if isRenderFuzzTag {
				bulk.Method = RenderFuzztagParamsToNucleiParams(bulk.Method)
				path = RenderFuzztagParamsToNucleiParams(path)
				for k, v := range headers {
					headers[k] = RenderFuzztagParamsToNucleiParams(v)
				}
				bulk.Body = RenderFuzztagParamsToNucleiParams(body)
			}
			path = "{{RootURL}}" + path

			bulk.Paths = append(bulk.Paths, path)
			bulk.Body = body
			bulk.Headers = headers
			if _, ok := bulk.Headers["Host"]; ok {
				delete(bulk.Headers, "Host")
			}
			bulk.Method = method

		case "raw":
			fallthrough
		default:
			// generalizedRequests := lowhttp.ReplaceHTTPPacketHeader(request.RequestRaw, "Host", "{{Hostname}}")
			requestRaw := string(request.RequestRaw)
			if isRenderFuzzTag {
				requestRaw = RenderFuzztagParamsToNucleiParams(requestRaw)
			}
			requestRaw = string(lowhttp.ReplaceHTTPPacketHeader([]byte(requestRaw), "Host", "{{Hostname}}"))
			bulk.HTTPRequests = append(bulk.HTTPRequests, &httptpl.YakHTTPRequestPacket{
				Request: requestRaw,
				Timeout: time.Duration(request.PerRequestTimeoutSeconds) * time.Second,
			})

		}
		// 如果将fuzztag重新修改为nuclei-tag，写入variables
		if isRenderFuzzTag {
			for k, v := range varToFuzztagMap {
				vars.SetWithType(k, v, "fuzztag")
			}
		}

		topMatcher := &ypb.HTTPResponseMatcher{SubMatchers: []*ypb.HTTPResponseMatcher{}}
		if len(request.GetMatchers()) > 0 {
			topMatcher = request.GetMatchers()[0]
		}

		matchers := HttpResponseMatchers2YakMatchers(topMatcher.SubMatchers)
		for _, matcher := range matchers {
			matcher.Id = index + 1
		}
		if len(matchers) > 0 {
			bulk.Matcher.SubMatchers = append(bulk.Matcher.SubMatchers, matchers...)
			bulk.Matcher.SubMatcherCondition = topMatcher.SubMatcherCondition
		}
		bulk.Extractor = append(bulk.Extractor, funk.Map(request.Extractors, func(extractor *ypb.HTTPResponseExtractor) *httptpl.YakExtractor {
			typeName := ""
			switch extractor.Type {
			case "nuclei-dsl":
				typeName = "dsl"
			case "kval":
				typeName = "key-value"
			default:
				typeName = extractor.Type
			}
			return &httptpl.YakExtractor{
				Id:               index + 1,
				Name:             extractor.Name,
				Type:             typeName,
				Scope:            extractor.Scope,
				Groups:           extractor.Groups,
				RegexpMatchGroup: funk.Map(extractor.RegexpMatchGroup, func(n int64) int { return int(n) }).([]int),
				XPathAttribute:   extractor.XPathAttribute,
			}
		}).([]*httptpl.YakExtractor)...)

		//bulk.IsHTTPS = request.IsHTTPS
		//bulk.IsGmTLS = request.IsGmTLS
		//bulk.Host = request.ActualAddr
		//bulk.Proxy = request.Proxy
		//bulk.NoSystemProxy = request.NoSystemProxy
		//
		//bulk.ForceFuzz = request.ForceFuzz
		bulk.NoFixContentLength = request.NoFixContentLength
		//bulk.RequestTimeout = request.PerRequestTimeoutSeconds
		//
		//bulk.RepeatTimes = request.RepeatTimes
		//bulk.Concurrent = request.Concurrent
		//bulk.DelayMinSeconds = request.DelayMinSeconds
		//bulk.DelayMaxSeconds = request.DelayMaxSeconds
		//
		//bulk.MaxRetryTimes = request.MaxRetryTimes
		//bulk.RetryInStatusCode = request.RetryInStatusCode
		//bulk.RetryNotInStatusCode = request.RetryNotInStatusCode
		//
		bulk.EnableRedirect = !request.NoFollowRedirect
		bulk.MaxRedirects = int(request.RedirectTimes)
		//
		//bulk.JsEnableRedirect = request.FollowJSRedirect
		//bulk.DNSServers = request.DNSServers
		//bulk.EtcHosts = etcHosts
		//bulk.Variables = vars

		bulk.CookieInherit = request.InheritCookies
		bulk.InheritVariables = request.InheritVariables
		bulk.StopAtFirstMatch = request.StopAtFirstMatch
		bulk.AfterRequested = request.AfterRequested
		bulk.RenderFuzzTag = isRenderFuzzTag
		//payloadsMap := map[string]any{}
		//for k, v := range vars.GetRaw() {
		//	data := strings.Split(v.Data, "\n")
		//	payloadsMap[k] = data
		//}
		//payloads, err := httptpl.NewYakPayloads(payloadsMap)
		//if err != nil {
		//	log.Errorf("generate yak payloads error: %v", err)
		//}
		//bulk.Payloads = payloads
	}
	//
	template := &httptpl.YakTemplate{}
	template.HTTPRequestSequences = []*httptpl.YakRequestBulkConfig{bulk}
	// 补充info字段
	randstr := utils.RandStringBytes(8)
	template.Id = fmt.Sprintf("WebFuzzer-Template-%s", randstr)
	template.Name = fmt.Sprintf("WebFuzzer Template %s", randstr)
	template.Author = "god"
	template.Severity = "low"
	template.Description = "write your description here"
	template.Reference = []string{"https://github.com/", "https://cve.mitre.org/"}
	template.Sign = template.SignMainParams()
	if vars.Len() > 0 {
		template.Variables = vars
	}
	// 转换为Yaml
	yamlContent, err := MarshalYakTemplateToYaml(template)
	if err != nil {
		res.Ok = false
		res.Reason = err.Error()
	} else {
		if err := template.CheckTemplateRisks(); err != nil {
			res.Reason = err.Error()
		}
	}
	return &ypb.ExportHTTPFuzzerTaskToYamlResponse{
		YamlContent: yamlContent,
		Status:      res,
	}, nil
}

func RenderFuzztagParamsToNucleiParams(s string) (replaced string) {
	renderParamsFunc := func(s string) []string {
		return []string{fmt.Sprintf("{{%s}}", s)}
	}
	res, err := fuzztagx.ExecuteWithStringHandler(s, map[string]func(string) []string{
		"params": renderParamsFunc,
		"param":  renderParamsFunc,
		"p":      renderParamsFunc,
	})
	if err != nil {
		return s
	}
	return strings.Join(res, "")
}

func MarshalYakTemplateToYaml(y *httptpl.YakTemplate) (string, error) {
	rootMap := NewYamlMapBuilder()
	rootMap.ForceSet("id", y.Id)
	rootMap.AddEmptyLine()
	infoMap := rootMap.NewSubMapBuilder("info")
	rootMap.AddEmptyLine()
	varMap := rootMap.NewSubMapBuilder("variables")
	reqSequencesArray := rootMap.NewSubArrayBuilder("http")
	writeConfig := func(builder *YamlMapBuilder, config *httptpl.RequestConfig) {
		builder.AddEmptyLine()
		builder.AddComment("WebFuzzer请求配置")
		builder.Set("is-https", config.IsHTTPS)
		builder.Set("is-gmtls", config.IsGmTLS)
		builder.Set("host", config.Host)
		builder.Set("proxy", config.Proxy)
		builder.Set("no-system-proxy", config.NoSystemProxy)
		builder.Set("force-fuzz", config.ForceFuzz)
		builder.Set("request-timeout", config.RequestTimeout)
		builder.Set("repeat-times", config.RepeatTimes)
		builder.Set("concurrent", config.Concurrent)
		builder.Set("delay-min-seconds", config.DelayMinSeconds)
		builder.Set("delay-max-seconds", config.DelayMaxSeconds)
		builder.Set("max-retry-times", config.MaxRetryTimes)
		builder.Set("retry-in-status-code", config.RetryInStatusCode)
		builder.Set("retry-not-in-status-code", config.RetryNotInStatusCode)
		builder.Set("js-enable-redirect", config.JsEnableRedirect)
		builder.Set("js-max-redirect", config.JsMaxRedirects)
		builder.Set("enable-redirect", config.EnableRedirect)
		builder.Set("max-redirects", config.MaxRedirects)
		builder.Set("dns-servers", config.DNSServers)
		builder.Set("etc-hosts", config.EtcHosts)
		varBuilder := builder.NewSubMapBuilder("variables")
		if config.Variables != nil {
			vars := config.Variables.ToMap()
			for k, v := range vars {
				varBuilder.Set(k, v)
			}
		}
	}
	_ = writeConfig
	//if len(y.HTTPRequestSequences) == 1 { // 当请求序列长度为1时，优先使用独立配置，无需写入全局配置
	//	writeConfig(rootMap, &y.RequestConfig)
	//}
	// 生成Info
	infoMap.Set("name", y.Name)
	infoMap.Set("author", y.Author)
	infoMap.Set("severity", y.Severity)
	infoMap.Set("description", y.Description)
	infoMap.Set("reference", y.Reference)
	metadata := infoMap.NewSubMapBuilder("metadata")
	classification := infoMap.NewSubMapBuilder("classification")
	classification.Set("cve-id", y.CVE)
	infoMap.Set("tags", strings.Join(y.Tags, ","))
	yakitInfo := infoMap.NewSubMapBuilder("yakit-info")

	y.Variables.Foreach(func(k string, v *httptpl.Var) {
		varMap.Set(k, v.GetValue())
	})
	// 生成req sequences
	maxRequest := 0
	//signElements := make([]string, 0)
	//addSignElements := func(i ...any) {
	//	for _, a := range i {
	//		res, err := json.Marshal(a)
	//		if err != nil {
	//			return
	//		}
	//
	//		signElements = append(signElements, string(res))
	//	}
	//}
	packagesNum := 0
	for _, sequence := range y.HTTPRequestSequences {
		packagesNum += len(sequence.Paths)
		packagesNum += len(sequence.HTTPRequests)
	}
	for _, sequence := range y.HTTPRequestSequences {
		sequenceItem := NewYamlMapBuilder()
		sequenceItem.SetDefaultField(map[string]any{
			"stop-at-first-macth": false,
			"max-size":            0,
			"unsafe":              false,
			"req-condition":       false,
			"redirects":           false,
			"disable-cookie":      false,
			"max-redirects":       0,
			"matchers-condition":  "or",
		})
		// 请求配置
		isPaths := len(sequence.Paths) > 0
		payloadsMap := sequenceItem.NewSubMapBuilder("payloads")
		if isPaths {
			// addSignElements(sequence.Method, sequence.Paths, sequence.Headers, sequence.Body)
			maxRequest += len(sequence.Paths)
			sequenceItem.Set("method", sequence.Method)
			sequenceItem.Set("path", sequence.Paths)
			sequenceItem.Set("headers", sequence.Headers)
			sequenceItem.Set("body", sequence.Body)
			sequenceItem.AddEmptyLine()
		} else {
			// addSignElements(sequence.HTTPRequests)
			maxRequest += len(sequence.HTTPRequests)
			reqArray := []string{}
			for _, request := range sequence.HTTPRequests {
				prefix := ""
				reqContent := request.Request
				if request.SNI != "" {
					prefix += "@tls-sni: " + request.SNI + "\n"
				}
				if request.Timeout != 0 {
					prefix += "@timeout: " + request.Timeout.String() + "\n"
				}
				if request.OverrideHost != "" {
					prefix += "@Host: " + request.OverrideHost + "\n"
				}
				reqArray = append(reqArray, strings.Replace(prefix+reqContent, "\r\n", "\n", -1))
			}
			sequenceItem.Set("raw", reqArray)
			sequenceItem.AddEmptyLine()
		}
		// 写入payloads
		if sequence.Payloads != nil && len(sequence.Payloads.GetRawPayloads()) > 0 {
			sequenceItem.AddComment("attack: pitchfork")
			for k, payload := range sequence.Payloads.GetRawPayloads() {
				if payload.FromFile != "" {
					payloadsMap.Set(k, payload.FromFile)
				} else {
					payloadsMap.Set(k, payload.Data)
				}
			}
			sequenceItem.AddEmptyLine()
		}
		// 其它配置
		sequenceItem.Set("redirects", sequence.EnableRedirect)
		sequenceItem.Set("max-redirects", sequence.MaxRedirects)

		sequenceItem.Set("stop-at-first-macth", sequence.StopAtFirstMatch)
		sequenceItem.Set("disable-cookie", !sequence.CookieInherit)
		sequenceItem.Set("max-size", sequence.MaxSize)
		sequenceItem.Set("unsafe", sequence.NoFixContentLength)
		sequenceItem.Set("req-condition", sequence.AfterRequested)

		// sequenceItem.Set("attack-mode", sequence.AttackMode)
		// sequenceItem.Set("inherit-variables", sequence.InheritVariables)
		// sequenceItem.Set("hot-patch-code", sequence.HotPatchCode)
		// matcher生成
		if sequence.Matcher != nil {
			matcher := sequence.Matcher
			// addSignElements(matcher)
			matcherCondition := matcher.SubMatcherCondition
			if matcherCondition == "" {
				matcherCondition = "or"
			}
			sequenceItem.Set("matchers-condition", matcherCondition)
			matcherArray := sequenceItem.NewSubArrayBuilder("matchers")
			for _, subMatcher := range matcher.SubMatchers {
				matcherItem := NewYamlMapBuilder()
				matcherItem.SetDefaultField(map[string]any{
					"negative":  false,
					"part":      "raw",
					"condition": "or",
				})
				if packagesNum > 1 {
					matcherItem.Set("id", subMatcher.Id)
				}
				switch subMatcher.MatcherType {
				case "word":
					matcherItem.Set("type", "word")
					matcherItem.Set("part", subMatcher.Scope)
					matcherItem.Set("words", subMatcher.Group)
				case "status_code":
					matcherItem.Set("type", "status")
					matcherItem.Set("status", lo.FilterMap(subMatcher.Group, func(s string, _ int) (int, bool) {
						i, err := strconv.Atoi(s)
						return i, err == nil
					}))
				case "content_length":
					matcherItem.Set("type", "size")
					matcherItem.Set("part", subMatcher.Scope)
					matcherItem.Set("size", lo.FilterMap(subMatcher.Group, func(s string, _ int) (int, bool) {
						i, err := strconv.Atoi(s)
						return i, err == nil
					}))
				case "binary":
					matcherItem.Set("type", "binary")
					matcherItem.Set("part", subMatcher.Scope)
					matcherItem.Set("binary", subMatcher.Group)
				case "regex":
					matcherItem.Set("type", "regex")
					matcherItem.Set("part", subMatcher.Scope)
					matcherItem.Set("regex", subMatcher.Group)
				case "expr":
					matcherItem.Set("type", "dsl")
					matcherItem.Set("part", subMatcher.Scope)
					matcherItem.Set("dsl", subMatcher.Group)
				}
				matcherItem.Set("negative", subMatcher.Negative)
				matcherItem.Set("condition", subMatcher.Condition)
				matcherItem.AddEmptyLine()
				matcherArray.Add(matcherItem)
			}
		}
		sequenceItem.Set("attack", sequence.AttackMode)
		// addSignElements(sequence.Extractor)
		// extractor生成
		extratorsArray := sequenceItem.NewSubArrayBuilder("extractors")
		for _, extractor := range sequence.Extractor {
			extractorItem := NewYamlMapBuilder()
			if packagesNum > 0 {
				extractorItem.Set("id", extractor.Id)
			}
			extractorItem.Set("name", extractor.Name)
			extractorItem.Set("scope", extractor.Scope)
			switch extractor.Type {
			case "regex":
				extractorItem.Set("type", "regex")
				extractorItem.Set("regex", extractor.Groups)
				extractorItem.SetDefaultField(map[string]any{
					"group": 0,
				})
				groupNumber := 0
				if len(extractor.RegexpMatchGroup) > 0 {
					groupNumber = extractor.RegexpMatchGroup[0]
				}
				extractorItem.Set("group", groupNumber)
			case "key-value":
				extractorItem.Set("type", "kval")
				extractorItem.Set("kval", extractor.Groups)
			case "json":
				extractorItem.Set("type", "json")
				extractorItem.Set("json", extractor.Groups)
			case "xpath":
				extractorItem.Set("type", "xpath")
				extractorItem.Set("xpath", extractor.Groups)
				extractorItem.Set("attribute", extractor.XPathAttribute)
			case "dsl":
				extractorItem.Set("type", "dsl")
				extractorItem.Set("dsl", extractor.Groups)
			}
			extractorItem.AddEmptyLine()
			extratorsArray.Add(extractorItem)
		}

		// WebFuzzer请求配置
		// writeConfig(sequenceItem, &sequence.RequestConfig)

		reqSequencesArray.Add(sequenceItem)
	}
	metadata.Set("max-request", maxRequest)
	metadata.ForceSet("shodan-query", "")
	metadata.Set("verified", true)
	yakitInfo.Set("sign", y.Sign) // 对 method, paths, headers, body、raw、matcher、extractor、payloads 签名

	rootMap.AddEmptyLine()
	rootMap.AddComment("Generated From WebFuzzer on " + time.Now().Format("2006-01-02 15:04:05"))
	return rootMap.MarshalToString()
}
