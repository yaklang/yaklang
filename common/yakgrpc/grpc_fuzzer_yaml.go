package yakgrpc

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
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
		var hTTPRequest *httptpl.YakHTTPRequestPacket
		if len(sequence.HTTPRequests) > 0 {
			hTTPRequest = sequence.HTTPRequests[0]
		} else {
			continue
		}
		fuzzerReq := &ypb.FuzzerRequest{
			Request:                  hTTPRequest.Request,
			RequestRaw:               []byte(hTTPRequest.Request),
			PerRequestTimeoutSeconds: hTTPRequest.Timeout.Seconds(),
			Params:                   nil,
		}
		fuzzerReq.Extractors = funk.Map(sequence.Extractor, func(extractor *httptpl.YakExtractor) *ypb.HTTPResponseExtractor {
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
		if len(sequence.Matcher.SubMatchers) > 0 {
			fuzzerReq.Matchers = yakMatchers2HttpResponseMatchers(sequence.Matcher.SubMatchers)
			fuzzerReq.MatchersCondition = sequence.Matcher.Condition
		} else {
			fuzzerReq.Matchers = yakMatchers2HttpResponseMatchers([]*httptpl.YakMatcher{sequence.Matcher})
			fuzzerReq.MatchersCondition = "and"
		}

		if sequence.EnableRedirect {
			fuzzerReq.RedirectTimes = float64(sequence.MaxRedirects)
		} else {
			fuzzerReq.RedirectTimes = 0
		}
		fuzzerReq.NoFixContentLength = sequence.NoFixContentLength

		vars := yakTemplate.Variables.ToMap()
		for k, v := range vars {
			fuzzerReq.Params = append(fuzzerReq.Params, &ypb.FuzzerParamItem{
				Key:   k,
				Value: utils.InterfaceToString(v),
				Type:  "raw",
			})
		}
		fuzzerRequest = append(fuzzerRequest, fuzzerReq)

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
			vars.SetWithType(param.Key, param.Value, param.Type)
		}
		etcHosts := map[string]string{}
		for _, pair := range request.EtcHosts {
			etcHosts[pair.Key] = pair.Value
		}
		bulk := &httptpl.YakRequestBulkConfig{}
		requestBulks = append(requestBulks, bulk)
		bulk.HTTPRequests = []*httptpl.YakHTTPRequestPacket{&httptpl.YakHTTPRequestPacket{
			Request: string(request.RequestRaw),
			Timeout: time.Duration(request.PerRequestTimeoutSeconds) * time.Second,
		}}
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

		bulk.IsHTTPS = request.IsHTTPS
		bulk.IsGmTLS = request.IsGmTLS
		bulk.Host = request.ActualAddr
		bulk.Proxy = request.Proxy
		bulk.NoSystemProxy = request.NoSystemProxy

		bulk.ForceFuzz = request.ForceFuzz
		bulk.NoFixContentLength = request.NoFixContentLength
		bulk.RequestTimeout = request.PerRequestTimeoutSeconds

		bulk.RepeatTimes = request.RepeatTimes
		bulk.Concurrent = request.Concurrent
		bulk.DelayMinSeconds = request.DelayMinSeconds
		bulk.DelayMaxSeconds = request.DelayMaxSeconds

		bulk.MaxRetryTimes = request.MaxRetryTimes
		bulk.RetryInStatusCode = request.RetryInStatusCode
		bulk.RetryNotInStatusCode = request.RetryNotInStatusCode

		bulk.EnableRedirect = !request.NoFollowRedirect
		bulk.MaxRedirects = int(request.RedirectTimes)

		bulk.JsEnableRedirect = request.FollowJSRedirect
		bulk.DNSServers = request.DNSServers
		bulk.EtcHosts = etcHosts
		bulk.Variables = vars

		bulk.CookieInherit = request.InheritCookies
		bulk.InheritVariables = request.InheritVariables
		bulk.HotPatchCode = request.HotPatchCode
		if bulk.Matcher != nil || len(bulk.Extractor) > 0 {
			hasMetcherOrExtractor = true
		}
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
		// 请求配置
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
			matcherItem.Set("negative", subMatcher.Negative)
			matcherItem.Set("condition", subMatcher.Condition)
			matcherItem.Set("part", subMatcher.Scope)
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
			matcherArray.Add(matcherItem)
		}
		//payloads 生成
		payloadsMap := sequenceItem.NewSubMapBuilder("payloads")
		if sequence.Payloads != nil {
			for k, payload := range sequence.Payloads.GetRawPayloads() {
				if payload.FromFile != "" {
					payloadsMap.Set(k, payload.FromFile)
				} else {
					payloadsMap.Set(k, payload.Data)
				}
			}
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
		// 其它请求配置
		sequenceItem.Set("redirects", sequence.EnableRedirect)
		sequenceItem.Set("max-redirects", sequence.MaxRedirects)
		sequenceItem.Set("cookie-reuse", sequence.CookieInherit)
		sequenceItem.Set("max-size", sequence.MaxSize)
		sequenceItem.Set("unsafe", sequence.NoFixContentLength)
		sequenceItem.Set("req-condition", sequence.AfterRequested)

		reqSequencesArray.Add(sequenceItem)
	}
	return rootMap.MarshalToString()
}
