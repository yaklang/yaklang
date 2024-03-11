package httptpl

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	utils2 "github.com/yaklang/yaklang/common/yak/httptpl/utils"
)

type requestRaw struct {
	Raw          []byte
	IsHttps      bool
	SNI          string
	Timeout      time.Duration
	OverrideHost string
	Params       map[string]interface{}
	Origin       *YakRequestBulkConfig
}

type RequestBulk struct {
	Requests      []*requestRaw
	RequestConfig *YakRequestBulkConfig
}

func (y *YakTemplate) GenerateRequestSequences(u string) []*RequestBulk {
	vars := utils.InterfaceToMapInterface(utils2.ExtractorVarsFromUrl(u))
	result := []*RequestBulk{}
	for _, sequenceCfg := range y.HTTPRequestSequences {
		seq := &RequestBulk{
			RequestConfig: sequenceCfg,
		}
		for _, path := range sequenceCfg.Paths {
			path, err := RenderNucleiTagWithVar(path, vars)
			if err != nil {
				log.Error(err)
				continue
			}
			isHttps := strings.HasPrefix(strings.ToLower(path), "https://")
			// isHttps, packet, err := lowhttp.ParseUrlToHttpRequestRaw(sequenceCfg.Method, path)
			uarlIns, err := url.Parse(path)
			if err != nil {
				log.Error(err)
				continue
			}
			packetStr := fmt.Sprintf(`%s %s HTTP/1.1
Host: %s
User-Agent: %s

`, sequenceCfg.Method, uarlIns.RequestURI(), uarlIns.Host, consts.DefaultUserAgent)

			packet := []byte(packetStr)
			for k, v := range sequenceCfg.Headers {
				packet = lowhttp.ReplaceHTTPPacketHeader(packet, k, v)
			}
			packet = append(packet, sequenceCfg.Body...)
			packetRaw, err := RenderNucleiTagWithVar(string(packet), vars)
			if err != nil {
				log.Error(err)
				continue
			}
			seq.Requests = append(seq.Requests, &requestRaw{
				Raw:     []byte(packetRaw),
				Origin:  sequenceCfg,
				IsHttps: isHttps,
			})
		}
		for _, request := range sequenceCfg.HTTPRequests {
			req, err := RenderNucleiTagWithVar(request.Request, vars)
			if err != nil {
				log.Error(err)
				continue
			}
			isHttps := false
			v, ok := vars["Schema"]
			if ok {
				isHttps = v == "https"
			}
			seq.Requests = append(seq.Requests, &requestRaw{
				Raw:          []byte(req),
				SNI:          request.SNI,
				Timeout:      request.Timeout,
				OverrideHost: request.OverrideHost,
				Origin:       sequenceCfg,
				IsHttps:      isHttps,
			})
		}
		result = append(result, seq)
	}
	return result
}

func (y *YakTemplate) ExecWithUrl(u string, config *Config, opts ...lowhttp.LowhttpOpt) (int, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if y.SelfContained {
		log.Debugf("skip self-contained template: %v", y.Name)
		return 0, nil
	}

	if config == nil {
		config = NewConfig()
	}

	var count int64 = 0
	if y.ReverseConnectionNeed {
		var err error
		var require func(...float64) (string, string, error)
		if config.OOBRequireCallback == nil {
			require = RequireOOBAddr
		} else {
			require = config.OOBRequireCallback
		}
		reverse_url, reverse_dnslog_token, err := require(config.OOBTimeout)
		y.Variables.Set("interactsh-url", reverse_url)
		y.Variables.Set("interactsh", reverse_url)
		y.Variables.Set("interactsh_url", reverse_url)
		y.Variables.Set("reverse_dnslog_token", reverse_dnslog_token)
		if err != nil {
			log.Errorf("require oob addr failed: %v", err)
			return 0, err
		}
	}

	tplConcurrent := config.ConcurrentInTemplates
	if len(y.HTTPRequestSequences) > 0 {
		swg := utils.NewSizedWaitGroup(tplConcurrent)
		for _, reqSeq := range y.GenerateRequestSequences(u) {
			swg.Add()
			go func(ret *RequestBulk, payload map[string][]string) {
				defer swg.Done()
				rsps, allResult, extracted, reqCount := y.handleRequestSequences(config, ret.RequestConfig, ret.Requests, payload, func(raw []byte, req *requestRaw) (*lowhttp.LowhttpResponse, error) {
					if config.BeforeSendPackage != nil {
						raw = config.BeforeSendPackage(raw, req.IsHttps)
					}
					packetOpt := opts
					redictTimes := 0
					if ret.RequestConfig.EnableRedirect {
						redictTimes = ret.RequestConfig.MaxRedirects
					}
					packetOpt = append(
						packetOpt,
						lowhttp.WithPacketBytes(raw),
						lowhttp.WithHttps(req.IsHttps),
						lowhttp.WithSaveHTTPFlow(true),
						lowhttp.WithSource(y.Name),
						lowhttp.WithNoFixContentLength(ret.RequestConfig.NoFixContentLength),
						lowhttp.WithRedirectTimes(redictTimes),
						lowhttp.WithTimeout(req.Timeout),
					)
					if req.OverrideHost != "" {
						packetOpt = append(packetOpt, lowhttp.WithHost(req.OverrideHost))
					}

					if config.Debug && config.DebugRequest {
						fmt.Printf("--------------REQ---------------\n")
						fmt.Println(string(raw))
					}

					utils.Debug(func() {
						log.Info("nuclei lowhttp.Exec! ")
						spew.Dump(raw)
					})
					rsp, err := lowhttp.HTTP(packetOpt...)
					if err != nil {
						log.Error(err)
						return nil, err
					}
					if config.Debug && config.DebugResponse {
						fmt.Printf("--------------RSP---------------\n")
						fmt.Println(string(rsp.RawPacket))
					}
					return rsp, nil
				})
				result := false
				for _, b := range allResult {
					result = result || b
				}
				if result {
					log.Infof("[%v]-[%v] matched", y.Name, y.Id)
				}
				atomic.AddInt64(&count, reqCount)
				config.ExecuteResultCallback(y, ret.RequestConfig, rsps, result, extracted)
			}(reqSeq, reqSeq.RequestConfig.Payloads.GetData())
		}
		swg.Wait()
		return int(count), nil
	} else if len(y.TCPRequestSequences) > 0 {
		swg := utils.NewSizedWaitGroup(tplConcurrent)
		for _, tcpReq := range y.TCPRequestSequences {
			swg.Add()
			tcpReq := tcpReq

			go func() {
				defer swg.Done()
				defer func() {
					if err := recover(); err != nil {
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				p := y.Variables.ToMap()

				lowhttpConfig := lowhttp.NewLowhttpOption()
				for _, opt := range opts {
					opt(lowhttpConfig)
				}
				renderVars := utils2.ExtractorVarsFromUrl(u)
				err := tcpReq.Execute(config, p, renderVars, lowhttpConfig, func(response []*NucleiTcpResponse, matched bool, extractorResults map[string]any) {
					atomic.AddInt64(&count, 1)
					config.ExecuteTCPResultCallback(y, tcpReq, response, matched, extractorResults)
					if config.Debug || config.DebugResponse {
						fmt.Println("---------------------TCP RESPONSE---------------------")
						spew.Dump(response)
						fmt.Println("------------------------------------------------------")
					}

					if config.Debug {
						fmt.Println("---------------------TCP RESULT---------------------")
						fmt.Printf("%v Matched: %v\n", y.Name, matched)
						fmt.Println("--------------------- EXTRACTOR ----------------------")
						spew.Dump(extractorResults)
					} else {
						log.Infof("%v Matched: %v", y.Name, matched)
					}
				})
				if err != nil {
					log.Errorf("tcpReq.Execute failed: %s", err)
				}
			}()
		}
		swg.Wait()
		return int(count), nil
	} else {
		return 0, utils.Errorf("[%s] tcp/http is all empty!", y.Name)
	}
}

func (y *YakTemplate) Exec(config *Config, isHttps bool, reqOrigin []byte, opts ...lowhttp.LowhttpOpt) (int, error) {
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqOrigin, isHttps)
	if err != nil {
		return 0, err
	}
	return y.ExecWithUrl(urlIns.String(), config, opts...)
}

// handleRequestSequences 渲染、发包、匹配、提取
func (y *YakTemplate) handleRequestSequences(config *Config, reqOrigin *YakRequestBulkConfig, reqSeqs []*requestRaw, payload map[string][]string, sender func(raw []byte, req *requestRaw) (*lowhttp.LowhttpResponse, error)) ([]*lowhttp.LowhttpResponse, []bool, map[string]interface{}, int64) {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	var count int64 = 0
	if reqOrigin == nil {
		log.Error("request origin cannot be nil")
		return nil, nil, map[string]interface{}{}, count
	}

	if reqOrigin.Matcher == nil && len(reqOrigin.Extractor) == 0 {
		log.Error("request sequence matcher and extractor all empty!")
		return nil, nil, map[string]interface{}{}, count
	}

	extracted := make(map[string]interface{})

	reqConfig := reqOrigin
	matcher := reqConfig.Matcher
	var matchers []*YakMatcher
	matchersCondition := "and"
	if matcher != nil {
		if len(matcher.SubMatchers) > 0 {
			matchers = matcher.SubMatchers
			matchersCondition = matcher.SubMatcherCondition
		} else {
			matchers = []*YakMatcher{matcher}
		}
	}
	var matchResults []bool
	var responses []*lowhttp.LowhttpResponse
	cacheRes := make(map[string]bool)
	runtimeVars := map[string]any{}
	matchHelper := func(rsp *lowhttp.LowhttpResponse, index int) bool {
		var tempMatchersResult []any
		for matcherIndex, matcher := range matchers {
			if matcher.Id == 0 {
				matchResult, err := matcher.ExecuteWithConfig(config, rsp, runtimeVars)
				if err != nil {
					log.Error("matcher execute failed: ", err)
				}
				tempMatchersResult = append(tempMatchersResult, matchResult)
			} else {
				if matcher.Id != index+1 {
					targetIndex := matcher.Id - 1
					hashKey := fmt.Sprintf("%v-%v", matcherIndex, targetIndex)
					if v, ok := cacheRes[hashKey]; ok {
						tempMatchersResult = append(tempMatchersResult, v)
					} else {
						if targetIndex >= len(responses) {
							tempMatchersResult = append(tempMatchersResult, false)
						} else {
							matchResult, err := matcher.ExecuteWithConfig(config, responses[targetIndex], runtimeVars)
							if err != nil {
								log.Error("matcher execute failed: ", err)
							}
							tempMatchersResult = append(tempMatchersResult, matchResult)
							cacheRes[hashKey] = matchResult
						}
					}
					continue
				}
				matchResult, err := matcher.ExecuteWithConfig(config, rsp, runtimeVars)
				if err != nil {
					log.Error("matcher execute failed: ", err)
				}
				tempMatchersResult = append(tempMatchersResult, matchResult)
				hashKey := fmt.Sprintf("%v-%v", matcherIndex, index)
				cacheRes[hashKey] = matchResult
			}
		}
		var matchRes bool
		if len(tempMatchersResult) > 0 {
			if matchersCondition == "or" {
				matchRes = funk.Any(tempMatchersResult...)
			} else {
				matchRes = funk.All(tempMatchersResult...)
			}
		}
		matchResults = append(matchResults, matchRes)
		return matchRes
	}
	for index, req := range reqSeqs {
		log.Debugf("sequence exec with Req Index:%v", index)
		raw := req.Raw
		reqs, err := FuzzNucleiTag(string(raw), y.Variables.ToMap(), payload, reqConfig.AttackMode)
		if err != nil {
			log.Errorf("cannot mutate request: %v", err)
			continue
		}
		if len(reqs) <= 0 {
			log.Error("mutate request failed, empty requests")
			continue
		}
		for _, reqRaw := range reqs {
			atomic.AddInt64(&count, 1)
			rsp, err := sender([]byte(reqRaw), req)
			if err == nil {
				responses = append(responses, rsp)
			} else {
				log.Error(err)
				continue
			}
			varsInResponse := LoadVarFromRawResponse(rsp.RawPacket, rsp.GetDurationFloat(), fmt.Sprintf("_%d", index+1))
			for k, v := range varsInResponse {
				runtimeVars[k] = v
			}
			for _, extractor := range reqOrigin.Extractor {
				if extractor.Id != 0 && extractor.Id != index+1 {
					continue
				}
				varIns, err := extractor.Execute(rsp.RawPacket, y.Variables.ToMap())
				if err != nil {
					log.Error("extractor execute failed: ", err)
					continue
				}
				if varIns != nil {
					for k, v := range varIns {
						v := ExtractResultToString(v)
						y.Variables.Set(k, v)
						extracted[k] = v
					}
				}
			}
			for k, v := range y.Variables.ToMap() {
				runtimeVars[k] = v
			}
			if !reqOrigin.AfterRequested {
				matchRes := matchHelper(rsp, index)
				if matchRes && reqOrigin.StopAtFirstMatch {
					// 第一次匹配就退出
					return responses, matchResults, extracted, count
				}
			}
		}

	}
	//if len(responses) > 0 {
	//	lastRsp := responses[len(responses)-1]
	//
	//}
	if reqOrigin.AfterRequested {
		for index, rsp := range responses {
			matchHelper(rsp, index)
		}
	}
	return responses, matchResults, extracted, count
}
