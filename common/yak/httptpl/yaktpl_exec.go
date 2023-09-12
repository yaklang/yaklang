package httptpl

import (
	"bytes"
	"fmt"
	"regexp"
	"sync/atomic"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var paramsRegexp = regexp.MustCompile(`(?i)\{\{params\(([a-z_][a-z0-9_]*)\)}}`)

type requestRaw struct {
	Raw     []byte
	IsHttps bool
	Params  map[string]interface{}
	Origin  *YakRequestBulkConfig
}

type RequestBulk struct {
	Requests      []*requestRaw
	RequestConfig *YakRequestBulkConfig
}

func (y *YakTemplate) generateRequests() chan *RequestBulk {
	var requests = make(chan *RequestBulk)
	go func() {
		for _, req := range y.HTTPRequestSequences {
			for _, raw := range req.GenerateRaw() {
				requests <- raw
			}
		}
		close(requests)
	}()
	return requests
}

func (y *YakTemplate) Exec(config *Config, isHttps bool, reqOrigin []byte, opts ...lowhttp.LowhttpOpt) (int, error) {
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
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqOrigin, isHttps)
	if err != nil {
		return 0, err
	}
	_ = urlIns
	vars, err := createVarsFromHTTPRequest(isHttps, reqOrigin)
	if err != nil {
		return 0, utils.Errorf("cannot create vars from http request: %v", err)
	}
	addToVars := func(k string, v any) {
		varsOperatorMutex.Lock()
		defer varsOperatorMutex.Unlock()
		vars[k] = v
	}

	if y.ReverseConnectionNeed {
		var err error
		var require func(...float64) (string, string, error)
		if config.OOBRequireCallback == nil {
			require = RequireOOBAddr
		} else {
			require = config.OOBRequireCallback
		}
		vars["reverse_url"], vars["reverse_dnslog_token"], err = require(config.OOBTimeout)
		if err != nil {
			log.Errorf("require oob addr failed: %v", err)
			return 0, err
		}
	}

	if y.Variables != nil {
		for k, v := range y.Variables.ToMap() {
			vars[k] = v
		}
	}

	tplConcurrent := config.ConcurrentInTemplates

	handleReqSeqs := func(reqOrigin *YakRequestBulkConfig, reqSeqs []*requestRaw, params map[string]interface{}) ([]*lowhttp.LowhttpResponse, bool, map[string]interface{}) {
		defer func() {
			if err := recover(); err != nil {
				log.Error(err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		if reqOrigin == nil {
			log.Error("request origin cannot be nil")
			return nil, false, map[string]interface{}{}
		}

		if reqOrigin.Matcher == nil && len(reqOrigin.Extractor) == 0 {
			log.Error("request sequence matcher and extractor all empty!")
			return nil, false, map[string]interface{}{}
		}

		extracted := make(map[string]interface{})
		multiRequest := len(reqSeqs) > 1

		reqConfig := reqOrigin
		matcher := reqConfig.Matcher

		var matchResults []any
		var responses []*lowhttp.LowhttpResponse

		for index, req := range reqSeqs {
			atomic.AddInt64(&count, 1)
			log.Debugf("sequence exec with Req Index:%v", index)
			if req.Params != nil {
				for k, v := range req.Params {
					params[k] = v
				}
			}
			raw := req.Raw

			//? 2023-8-2 暂时性解决方案
			//? 尝试将placHolder替换为params真正的值
			placeHolderMap := y.PlaceHolderMap
			if len(placeHolderMap) > 0 {
				for ph, k := range placeHolderMap {
					if v, ok := params[k]; ok {
						raw = bytes.ReplaceAll(raw, []byte(ph), toBytes(v))
					} else {
						raw = bytes.ReplaceAll(raw, []byte(ph), []byte(k))
					}
				}
			}

			reqs, err := mutate.FuzzTagExec(raw, mutate.Fuzz_WithExtraFuzzTagHandler(
				"expr:nucleidsl", func(s string) []string {
					data, err := NewNucleiDSLYakSandbox().Execute(s, params)
					if err != nil {
						return []string{""}
					}
					return []string{toString(data)}
				}),
			)

			if err != nil {
				log.Errorf("cannot mutate request: %v", err)
				continue
			}
			if len(reqs) <= 0 {
				log.Error("mutate request failed, empty requests")
				continue
			}

			reqRaw := reqs[0]
			var sufs = []string{fmt.Sprintf("_%v", index+1)}
			_ = reqRaw
			_ = sufs
			// multiRequest
			var packetOpt []lowhttp.LowhttpOpt
			packetOpt = append(
				packetOpt, lowhttp.WithPacketBytes([]byte(reqRaw)), lowhttp.WithHttps(isHttps),
				lowhttp.WithSaveHTTPFlow(true), lowhttp.WithSource(y.Name),
			)
			packetOpt = append(packetOpt, opts...)
			if config.Debug && config.DebugRequest {
				fmt.Printf("--------------REQ---------------\n")
				fmt.Println(reqRaw)
			}

			utils.Debug(func() {
				log.Info("nuclei lowhttp.Exec! ")
				spew.Dump(reqRaw)
			})
			rsp, err := lowhttp.HTTP(packetOpt...)
			if err != nil {
				log.Error(err)
				return responses, false, extracted
			}
			if config.Debug && config.DebugResponse {
				fmt.Printf("--------------RSP---------------\n")
				fmt.Println(string(rsp.RawPacket))
			}
			if rsp != nil {
				responses = append(responses, rsp)
			}
			if !multiRequest {
				sufs = nil
			}

			for index, extractor := range req.Origin.Extractor {
				varIns, err := extractor.Execute(rsp.RawPacket)
				if err != nil {
					log.Error("extractor execute failed: ", err)
					continue
				}
				_ = index
				if varIns != nil {
					for k, v := range varIns {
						v := ExtractResultToString(v)
						addToVars(k, v)
						extracted[k] = v
					}
				}
			}

			if req.Origin.Matcher != nil {
				var varsInResponse = make(map[string]interface{})
				if len(sufs) == 0 {
					varsInResponse = LoadVarFromRawResponse(rsp.RawPacket, rsp.GetDurationFloat())
				} else {
					varsInResponse = LoadVarFromRawResponse(rsp.RawPacket, rsp.GetDurationFloat(), sufs...)
				}
				if varsInResponse != nil {
					for k, v := range varsInResponse {
						addToVars(k, v)
					}
				}

				if !reqOrigin.AfterRequested {
					matchResult, err := matcher.ExecuteWithConfig(config, rsp, vars)
					if err != nil {
						log.Error("matcher execute failed: ", err)
					}
					matchResults = append(matchResults, matchResult)
					if matchResult && reqOrigin.StopAtFirstMatch {
						// 第一次匹配就退出
						return responses, true, extracted
					}
				}
			}
		}

		if reqOrigin.AfterRequested {
			if matcher != nil {
				for _, rsp := range responses {
					matchResult, err := matcher.ExecuteWithConfig(config, rsp, vars)
					if err != nil {
						log.Error("matcher execute failed: ", err)
					}
					matchResults = append(matchResults, matchResult)
				}
			}
		}
		return responses, funk.Any(matchResults...), extracted
	}

	if len(y.HTTPRequestSequences) > 0 {
		swg := utils.NewSizedWaitGroup(tplConcurrent)
		for reqSeqs := range y.generateRequests() {
			swg.Add()
			varsOperatorMutex.Lock()
			p := utils.CopyMapInterface(vars)
			varsOperatorMutex.Unlock()
			go func(ret *RequestBulk, params map[string]interface{}) {
				defer swg.Done()

				rsps, result, extracted := handleReqSeqs(ret.RequestConfig, ret.Requests, params)
				if result {
					log.Infof("[%v]-[%v] matched", y.Name, y.Id)
				}
				config.ExecuteResultCallback(y, ret.RequestConfig, rsps, result, extracted)
			}(reqSeqs, p)
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
				varsOperatorMutex.Lock()
				p := utils.CopyMapInterface(vars)
				varsOperatorMutex.Unlock()

				lowhttpConfig := lowhttp.NewLowhttpOption()
				for _, opt := range opts {
					opt(lowhttpConfig)
				}

				err := tcpReq.Execute(config, p, y.PlaceHolderMap, lowhttpConfig, func(response []*NucleiTcpResponse, matched bool, extractorResults map[string]any) {
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
