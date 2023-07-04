package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"time"
)

func (s *Server) ImportHTTPFuzzerTaskFromYaml(ctx context.Context, req *ypb.ImportHTTPFuzzerTaskFromYamlRequest) (*ypb.ImportHTTPFuzzerTaskFromYamlResponse, error) {
	yamlPath := req.GetYamlPath()
	if yamlPath == "" {
		return nil, utils.Errorf("yaml path is empty")
	}
	var result ypb.ImportHTTPFuzzerTaskFromYamlResponse
	var fuzzerRequest []*ypb.FuzzerRequest
	content, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, utils.Errorf("cannot read yaml file: %v", err)
	}
	yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(string(content))
	if err != nil {
		return nil, utils.Errorf("cannot create yak template from nuclei template: %v", err)
	}
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
		fuzzerReq.Matchers = yakMatchers2HttpResponseMatchers(sequence.Matcher.SubMatchers)
		fuzzerReq.MatchersCondition = sequence.Matcher.Condition
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
	result.FuzzerRequests = fuzzerRequest
	return &result, nil
}
func (s *Server) ExportHTTPFuzzerTaskToYaml(ctx context.Context, req *ypb.ExportHTTPFuzzerTaskToYamlRequest) (*ypb.GeneralResponse, error) {
	res := &ypb.GeneralResponse{
		Ok:     true,
		Reason: "",
	}
	fuzzerRequests := req.GetFuzzerRequests()
	template := &httptpl.YakTemplate{}
	template.HTTPRequestSequences = make([]*httptpl.YakRequestBulkConfig, len(fuzzerRequests))
	//requestBulkConfig := template.HTTPRequestSequences[0]
	//requestBulkConfig.HTTPRequests = funk.Map(fuzzerRequests, func(fuzzerRequest *ypb.FuzzerRequest) *httptpl.YakHTTPRequestPacket {
	//	return &httptpl.YakHTTPRequestPacket{
	//		Request: fuzzerRequest.Request,
	//		Timeout: time.Duration(fuzzerRequest.PerRequestTimeoutSeconds) * time.Second,
	//	}
	//}).([]*httptpl.YakHTTPRequestPacket)
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
	for i, request := range fuzzerRequests {
		template.HTTPRequestSequences[i].HTTPRequests = make([]*httptpl.YakHTTPRequestPacket, 1)
		template.HTTPRequestSequences[i].HTTPRequests[0] = &httptpl.YakHTTPRequestPacket{
			Request: request.Request,
			Timeout: time.Duration(request.PerRequestTimeoutSeconds) * time.Second,
		}
		template.HTTPRequestSequences[i].Matcher = &httptpl.YakMatcher{
			SubMatchers:         HttpResponseMatchers2YakMatchers(request.Matchers),
			SubMatcherCondition: request.MatchersCondition,
		}
		template.HTTPRequestSequences[i].Extractor = funk.Map(request.Extractors, func(extractor *ypb.HTTPResponseExtractor) *httptpl.YakExtractor {
			return &httptpl.YakExtractor{
				Name:             extractor.Name,
				Type:             extractor.Type,
				Scope:            extractor.Scope,
				Groups:           extractor.Groups,
				RegexpMatchGroup: funk.Map(extractor.RegexpMatchGroup, func(n int64) int { return int(n) }).([]int),
				XPathAttribute:   extractor.XPathAttribute,
			}
		}).([]*httptpl.YakExtractor)
		template.HTTPRequestSequences[i].EnableRedirect = request.RedirectTimes > 0
		template.HTTPRequestSequences[i].MaxRedirects = int(request.RedirectTimes)
		template.HTTPRequestSequences[i].NoFixContentLength = request.NoFixContentLength
	}
	byts, err := template.MarshalToYaml()
	if err != nil {
		res.Ok = false
		res.Reason = err.Error()
		return res, nil
	}
	err = os.WriteFile(req.YamlPath, byts, os.ModePerm)
	if err != nil {
		res.Ok = false
		res.Reason = err.Error()
		return res, nil
	}
	return res, nil
}
