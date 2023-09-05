package yakgrpc

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"time"
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
	if err != nil {
		return nil, utils.Errorf("cannot create yak template from yaml: %v", err)
	}
	// 转FuzzerRequest
	for _, sequence := range yakTemplate.HTTPRequestSequences {
		var fuzzerReqs []*ypb.FuzzerRequest
		if len(sequence.HTTPRequests) == 0 {
			fuzzerReqs = make([]*ypb.FuzzerRequest, len(sequence.HTTPRequests)) // 每个package对应一个fuzzerRequest,使用相同配置
			for i, request := range sequence.HTTPRequests {
				req := request.Request
				for k, v := range yakTemplate.PlaceHolderMap { // 做替换这些事应该是执行时做，需要改，这里暂时做下兼容
					if strings.HasPrefix(v, "__") && strings.HasSuffix(v, "__") {
						newv := ""
						var nextAdd = ('A' - 'a') //首字母大写
						for _, ch := range v[2 : len(v)-2] {
							if ch == '_' {
								nextAdd += ('A' - 'a')
								continue
							} else {
								newv += string(ch + nextAdd)
								nextAdd = 0
							}
						}
						v = newv
					}
					if v == "Hostname" {
						v = "www.example.com"
					} else {
						v = "{{" + v + "}}"
					}
					req = strings.ReplaceAll(req, k, v)
				}
				fuzzerReqs[i] = &ypb.FuzzerRequest{
					Request:                  req,
					RequestRaw:               []byte(req),
					PerRequestTimeoutSeconds: request.Timeout.Seconds(),
				}
			}
		} else {
			fuzzerReqs = make([]*ypb.FuzzerRequest, len(sequence.Paths))
			for i, path := range sequence.Paths {
				httpPackage := lowhttp.UrlToRequestPacket(sequence.Method, path, nil, false)
				for k, v := range sequence.Headers {
					httpPackage = lowhttp.ReplaceHTTPPacketHeader(httpPackage, k, v)
				}
				lowhttp.ReplaceHTTPPacketBody(httpPackage, []byte(sequence.Body), false)
				fuzzerReqs[i] = &ypb.FuzzerRequest{
					Request:    string(httpPackage),
					RequestRaw: httpPackage,
				}
			}
		}
		extractors := funk.Map(sequence.Extractor, func(extractor *httptpl.YakExtractor) *ypb.HTTPResponseExtractor {
			return &ypb.HTTPResponseExtractor{
				Name:             extractor.Name,
				Type:             extractor.Type,
				Scope:            extractor.Scope,
				Groups:           extractor.Groups,
				RegexpMatchGroup: funk.Map(extractor.RegexpMatchGroup, func(n int) int64 { return int64(n) }).([]int64),
				XPathAttribute:   extractor.XPathAttribute,
			}
		}).([]*ypb.HTTPResponseExtractor)
		var yakMatchers2HttpResponseMatchers func(matchers []*httptpl.YakMatcher) []*ypb.HTTPResponseMatcher
		yakMatchers2HttpResponseMatchers = func(matchers []*httptpl.YakMatcher) []*ypb.HTTPResponseMatcher {
			return funk.Map(matchers, func(matcher *httptpl.YakMatcher) *ypb.HTTPResponseMatcher {
				return &ypb.HTTPResponseMatcher{
					SubMatchers:         yakMatchers2HttpResponseMatchers(matcher.SubMatchers),
					SubMatcherCondition: matcher.SubMatcherCondition,
					MatcherType:         matcher.MatcherType,
					Scope:               matcher.Scope,
					Condition:           matcher.Condition,
					Group:               matcher.Group,
					GroupEncoding:       matcher.GroupEncoding,
					Negative:            matcher.Negative,
					ExprType:            matcher.ExprType,
				}
			}).([]*ypb.HTTPResponseMatcher)
		}
		var matchers []*ypb.HTTPResponseMatcher
		var matchersCondition string
		if len(sequence.Matcher.SubMatchers) > 0 {
			matchers = yakMatchers2HttpResponseMatchers(sequence.Matcher.SubMatchers)
			matchersCondition = sequence.Matcher.Condition
		} else {
			matchers = yakMatchers2HttpResponseMatchers([]*httptpl.YakMatcher{sequence.Matcher})
			matchersCondition = "and"
		}
		var redirectTimes float64
		if sequence.EnableRedirect {
			redirectTimes = float64(sequence.MaxRedirects)
		} else {
			redirectTimes = 0
		}
		noFixContentLength := sequence.NoFixContentLength

		vars := yakTemplate.Variables.ToMap()
		var params []*ypb.FuzzerParamItem
		for k, v := range vars {
			params = append(params, &ypb.FuzzerParamItem{
				Key:   k,
				Value: utils.InterfaceToString(v),
				Type:  "raw",
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
		noFollowRedirect := sequence.EnableRedirect
		//
		//fuzzerReq.FollowJSRedirect = sequence.JsEnableRedirect
		//fuzzerReq.DNSServers = sequence.DNSServers

		inheritCookies := sequence.CookieInherit
		inheritVariables := sequence.InheritVariables

		for _, fuzzerReq := range fuzzerReqs {
			fuzzerReq.Extractors = extractors
			fuzzerReq.Matchers = matchers
			fuzzerReq.MatchersCondition = matchersCondition
			fuzzerReq.NoFixContentLength = noFixContentLength
			fuzzerReq.NoFollowRedirect = noFollowRedirect
			fuzzerReq.RedirectTimes = redirectTimes
			fuzzerReq.InheritCookies = inheritCookies
			fuzzerReq.InheritVariables = inheritVariables
			fuzzerReq.Params = params
			fuzzerRequest = append(fuzzerRequest, fuzzerReq)
		}
	}
	result := &ypb.ImportHTTPFuzzerTaskFromYamlResponse{
		Requests: &ypb.FuzzerRequests{
			Requests: fuzzerRequest,
		},
		Status: &ypb.GeneralResponse{
			Ok: true,
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
			return &httptpl.YakMatcher{
				SubMatchers:         HttpResponseMatchers2YakMatchers(matcher.SubMatchers),
				SubMatcherCondition: matcher.SubMatcherCondition,
				MatcherType:         matcher.MatcherType,
				Scope:               matcher.Scope,
				Condition:           matcher.Condition,
				Group:               matcher.Group,
				GroupEncoding:       matcher.GroupEncoding,
				Negative:            matcher.Negative,
				ExprType:            matcher.ExprType,
			}
		}).([]*httptpl.YakMatcher)
	}
	// 生成请求桶
	requestBulks := []*httptpl.YakRequestBulkConfig{}
	hasMetcherOrExtractor := false
	for _, request := range seq.GetRequests() {
		vars := httptpl.NewVars()
		for _, param := range request.Params {
			if err := vars.SetWithType(param.Key, param.Value, param.Type); err != nil {
				log.Errorf("set vars error: %v", err)
			}
		}
		etcHosts := map[string]string{}
		for _, pair := range request.EtcHosts {
			etcHosts[pair.Key] = pair.Value
		}
		bulk := &httptpl.YakRequestBulkConfig{}
		requestBulks = append(requestBulks, bulk)
		switch templateType {
		case "path":
			url, err := lowhttp.ExtractURLFromHTTPRequestRaw(request.RequestRaw, false)
			if err != nil {
				log.Error(err)
			}
			rootPath := utils.ParseStringUrlToWebsiteRootPath(url.Path)
			path := strings.Replace(url.Path, rootPath, "{{RootUrl}}", 1)
			bulk.Paths = []string{path}
			bulk.Body = string(lowhttp.GetHTTPPacketBody(request.RequestRaw))
			bulk.Headers = lowhttp.GetHTTPPacketHeaders(request.RequestRaw)
			req, err := lowhttp.ParseBytesToHttpRequest(request.RequestRaw)
			if err != nil {
				log.Error(err)
				bulk.Method = "GET"
			} else {
				bulk.Method = req.Method
			}
		case "raw":
			fallthrough
		default:
			generalizedRequests := lowhttp.ReplaceHTTPPacketHeader(request.RequestRaw, "Host", "{{Hostname}}")
			bulk.HTTPRequests = []*httptpl.YakHTTPRequestPacket{&httptpl.YakHTTPRequestPacket{
				Request: string(generalizedRequests),
				Timeout: time.Duration(request.PerRequestTimeoutSeconds) * time.Second,
			}}
		}
		bulk.Matcher = &httptpl.YakMatcher{
			SubMatchers:         HttpResponseMatchers2YakMatchers(request.Matchers),
			SubMatcherCondition: request.MatchersCondition,
		}
		bulk.Extractor = funk.Map(request.Extractors, func(extractor *ypb.HTTPResponseExtractor) *httptpl.YakExtractor {
			return &httptpl.YakExtractor{
				Name:             extractor.Name,
				Type:             extractor.Type,
				Scope:            extractor.Scope,
				Groups:           extractor.Groups,
				RegexpMatchGroup: funk.Map(extractor.RegexpMatchGroup, func(n int64) int { return int(n) }).([]int),
				XPathAttribute:   extractor.XPathAttribute,
			}
		}).([]*httptpl.YakExtractor)

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
		//bulk.EnableRedirect = !request.NoFollowRedirect
		//bulk.MaxRedirects = int(request.RedirectTimes)
		//
		//bulk.JsEnableRedirect = request.FollowJSRedirect
		//bulk.DNSServers = request.DNSServers
		//bulk.EtcHosts = etcHosts
		//bulk.Variables = vars

		bulk.CookieInherit = request.InheritCookies
		bulk.InheritVariables = request.InheritVariables
		if bulk.Matcher != nil || len(bulk.Extractor) > 0 {
			hasMetcherOrExtractor = true
		}
		bulk.StopAtFirstMatch = request.StopAtFirstMatch
		bulk.AfterRequested = request.AfterRequested
	}
	if !hasMetcherOrExtractor {
		res.Ok = false
		res.Reason = "no matcher or extractor"
		return &ypb.ExportHTTPFuzzerTaskToYamlResponse{
			YamlContent: "",
			Status:      res,
		}, nil
	}
	//
	template := &httptpl.YakTemplate{}
	template.HTTPRequestSequences = requestBulks
	// 转换为Yaml
	yamlContent, err := MarshalYakTemplateToYaml(template)
	if err != nil {
		res.Ok = false
		res.Reason = err.Error()
	}
	return &ypb.ExportHTTPFuzzerTaskToYamlResponse{
		YamlContent: yamlContent,
		Status:      res,
	}, nil
}

func MarshalYakTemplateToYaml(y *httptpl.YakTemplate) (string, error) {
	rootMap := NewYamlMapBuilder()
	rootMap.Set("id", y.Id)
	infoMap := rootMap.NewSubMapBuilder("info")
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
	infoMap.Set("tags", strings.Join(y.Tags, ","))
	classification := infoMap.NewSubMapBuilder("classification")
	classification.Set("cve-id", y.CVE)
	//生成req sequences
	for _, sequence := range y.HTTPRequestSequences {
		sequenceItem := NewYamlMapBuilder()
		sequenceItem.SetDefaultField(map[string]any{
			"stop-at-first-macth": true,
			"max-size":            0,
			"unsafe":              false,
			"req-condition":       false,
			"redirects":           false,
			"max-redirects":       0,
			"matchers-condition":  "and",
		})
		// 请求配置
		isPaths := len(sequence.Paths) > 0
		var payloadsMap *YamlMapBuilder
		if isPaths {
			sequenceItem.Set("method", sequence.Method)
			sequenceItem.Set("path", sequence.Paths)
			payloadsMap = sequenceItem.NewSubMapBuilder("payloads")
			sequenceItem.Set("headers", sequence.Headers)
			sequenceItem.Set("body", sequence.Body)
		} else {
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
			payloadsMap = sequenceItem.NewSubMapBuilder("payloads")
		}
		// 写入payloads
		if sequence.Payloads != nil {
			for k, payload := range sequence.Payloads.GetRawPayloads() {
				if payload.FromFile != "" {
					payloadsMap.Set(k, payload.FromFile)
				} else {
					payloadsMap.Set(k, payload.Data)
				}
			}
		}
		// matcher生成
		matcher := sequence.Matcher
		matcherCondition := matcher.Condition
		if matcherCondition == "" {
			matcherCondition = "and"
		}
		sequenceItem.Set("matchers-condition", matcherCondition)
		matcherArray := sequenceItem.NewSubArrayBuilder("matchers")
		for _, subMatcher := range matcher.SubMatchers {
			matcherItem := NewYamlMapBuilder()
			matcherItem.SetDefaultField(map[string]any{
				"negative": false,
				"part":     "raw",
			})
			switch subMatcher.MatcherType {
			case "word":
				matcherItem.Set("type", "word")
				matcherItem.Set("words", subMatcher.Group)
			case "status_code":
				matcherItem.Set("type", "status")
				matcherItem.Set("status", subMatcher.Group)
			case "content_length":
				matcherItem.Set("type", "size")
				matcherItem.Set("size", subMatcher.Group)
			case "binary":
				matcherItem.Set("type", "binary")
				matcherItem.Set("binary", subMatcher.Group)
			case "regex":
				matcherItem.Set("type", "regex")
				matcherItem.Set("regex", subMatcher.Group)
			case "expr":
				matcherItem.Set("type", "dsl")
				matcherItem.Set("dsl", subMatcher.Group)
			}
			matcherItem.Set("negative", subMatcher.Negative)
			matcherItem.Set("condition", subMatcher.Condition)
			matcherItem.Set("part", subMatcher.Scope)
			matcherArray.Add(matcherItem)
		}

		sequenceItem.Set("attack", sequence.AttackMode)

		// extractor生成
		extratorsArray := sequenceItem.NewSubArrayBuilder("extractors")
		for _, extractor := range sequence.Extractor {
			extractorItem := NewYamlMapBuilder()
			extractorItem.Set("name", extractor.Name)
			extractorItem.Set("scope", extractor.Scope)
			switch extractor.Type {
			case "regex":
				extractorItem.Set("type", "regex")
				extractorItem.Set("regex", extractor.Groups)
				extractorItem.Set("group", extractor.RegexpMatchGroup)
			case "key-value":
				extractorItem.Set("type", "kval")
				extractorItem.Set("group", extractor.Groups)
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
			extratorsArray.Add(extractorItem)
		}

		// 其它配置
		sequenceItem.Set("redirects", sequence.EnableRedirect)
		sequenceItem.Set("max-redirects", sequence.MaxRedirects)

		//sequenceItem.Set("stop-at-first-macth", sequence.StopAtFirstMatch)
		//sequenceItem.Set("cookie-reuse", sequence.CookieInherit)
		//sequenceItem.Set("max-size", sequence.MaxSize)
		//sequenceItem.Set("unsafe", sequence.NoFixContentLength)
		//sequenceItem.Set("req-condition", sequence.AfterRequested)
		//sequenceItem.Set("attack-mode", sequence.AttackMode)
		//sequenceItem.Set("inherit-variables", sequence.InheritVariables)
		//sequenceItem.Set("hot-patch-code", sequence.HotPatchCode)

		// WebFuzzer请求配置
		//writeConfig(sequenceItem, &sequence.RequestConfig)

		reqSequencesArray.Add(sequenceItem)
	}
	return rootMap.MarshalToString()
}
