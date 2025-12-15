package yakgrpc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/davecgh/go-spew/spew"
	"github.com/samber/lo"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	Hijack_List_Add    = "add"
	Hijack_List_Delete = "delete"
	Hijack_List_Update = "update"
	Hijack_List_Reload = "reload"
)

var (
	Hijack_Status_Request  = "hijacking request"
	Hijack_Status_Response = "hijacking response"
	Hijack_Status_Waiting  = "wait hijack"
	Hijack_Status_WS       = "hijacking ws"
)

var (
	mitmPluginCallerGlobal *yak.MixPluginCaller
	// 添加生命周期通知机制
	mitmPluginCallerNotifyChan chan struct{}
)

func (s *Server) MITMV2(stream ypb.Yak_MITMV2Server) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("mitm panic... %v", err)
		}
	}()

	var mServer *crep.MITMServer

	send := func(rsp *ypb.MITMV2Response) (sendError error) {
		defer func() {
			if err := recover(); err != nil {
				rspRaw, _ := json.Marshal(rsp)
				spew.Dump(rspRaw)
				sendError = utils.Errorf("send error: %s", err)
			}
		}()

		sendError = stream.Send(safeUTF8MITMV2Resp(rsp))
		return
	}
	sendLogged := func(rsp *ypb.MITMV2Response) {
		err := send(rsp)
		if err != nil {
			log.Errorf("send mitm response error: %v", err)
		}
	}
	notifyMITMStartFailed := func(reason string) {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return
		}
		sendLogged(&ypb.MITMV2Response{
			HaveNotification:    true,
			NotificationContent: []byte(fmt.Sprintf("启动 MITM 服务器 ERROR!\n%v", reason)),
		})
	}

	execFeedback := yak.YakitCallerIf(func(result *ypb.ExecResult) error {
		return send(&ypb.MITMV2Response{Message: result, HaveMessage: true})
	})

	feedbackToUser := feedbackFactory(s.GetProjectDatabase(), execFeedback, false, "")

	getPlainRequestBytes := func(req *http.Request) []byte {
		var plainRequest []byte
		if httpctx.GetRequestIsModified(req) {
			plainRequest = httpctx.GetHijackedRequestBytes(req)
		} else {
			plainRequest = httpctx.GetPlainRequestBytes(req)
			if len(plainRequest) <= 0 {
				decoded := lowhttp.DeletePacketEncoding(httpctx.GetBareRequestBytes(req))
				httpctx.SetPlainRequestBytes(req, decoded)
				plainRequest = decoded
			}
		}
		return plainRequest
	}
	getPlainResponseBytes := func(req *http.Request) []byte {
		var plainResponse []byte
		if httpctx.GetResponseIsModified(req) {
			plainResponse = httpctx.GetHijackedResponseBytes(req)
		} else {
			plainResponse = httpctx.GetPlainResponseBytes(req)
			if len(plainResponse) <= 0 {
				plainResponse = lowhttp.DeletePacketEncoding(httpctx.GetBareResponseBytes(req))
			}
		}
		return plainResponse
	}

	firstReq, err := stream.Recv()
	if err != nil {
		return utils.Errorf("recv first req failed: %s", err)
	}
	feedbackToUser("接收到 MITM 启动参数 / receive mitm config request")

	getDownstreamProxy := func(request *ypb.MITMV2Request) ([]string, map[string][]string, error) {
		downstreamProxy := strings.TrimSpace(request.GetDownstreamProxy())
		// 容错处理一下代理
		downstreamProxy = strings.Trim(downstreamProxy, `":`)
		if downstreamProxy == "0" {
			downstreamProxy = ""
		}
		downstreamProxys := strings.Split(downstreamProxy, ",")
		var (
			proxys      []string
			proxyErrors []string // 格式错误等致命错误
		)
		for _, proxy := range downstreamProxys {
			if strings.TrimSpace(proxy) == "" {
				continue
			}
			proxyUrl, err2 := url.Parse(proxy)
			if err2 != nil {
				errMsg := fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", proxy, err2)
				log.Errorf("代理检测失败 / proxy check failed:[%v] %v", proxy, err2)
				feedbackToUser(errMsg)
				proxyErrors = append(proxyErrors, errMsg)
				continue
			}
			_, port, err := utils.ParseStringToHostPort(proxyUrl.Host)
			if err != nil {
				errMsg := fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", proxy, "parse host to host:port failed "+err.Error())
				log.Errorf("代理检测失败 / proxy check failed:[%v] %v", proxy, err)
				feedbackToUser(errMsg)
				proxyErrors = append(proxyErrors, errMsg)
				continue
			}
			if port <= 0 {
				errMsg := fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", proxy, "缺乏端口（Miss Port）")
				log.Errorf("代理检测失败 / proxy check failed:[%v] 缺乏端口（Miss Port）", proxy)
				feedbackToUser(errMsg)
				proxyErrors = append(proxyErrors, errMsg)
				continue
			}
			proxys = append(proxys, proxyUrl.String())
		}
		// 如果全部是致命错误（格式等），依然报错；连通性失败只告警不阻塞
		if len(proxys) == 0 && len(proxyErrors) > 0 {
			return nil, nil, utils.Errorf(strings.Join(proxyErrors, "; "))
		}
		ruleID := strings.TrimSpace(request.GetDownstreamProxyRuleId())
		var routeMap map[string][]string
		if ruleID != "" {
			cfg, err := yakit.GetGlobalProxyRulesConfig()
			if err != nil {
				return nil, nil, err
			}
			endpointURLByID := make(map[string]string, len(cfg.GetEndpoints()))
			for _, endpoint := range cfg.GetEndpoints() {
				if endpoint == nil {
					continue
				}
				url := yakit.BuildProxyEndpointURL(endpoint)
				if strings.TrimSpace(url) == "" {
					continue
				}
				endpointURLByID[endpoint.GetId()] = url
			}
			var (
				rule         *ypb.ProxyRoute
				ruleDisabled bool
			)
			for _, r := range cfg.GetRoutes() {
				if r == nil || !strings.EqualFold(r.GetId(), ruleID) {
					continue
				}
				if r.GetDisabled() {
					ruleDisabled = true
					break
				}
				rule = r
				break
			}
			if rule == nil {
				if ruleDisabled {
					feedbackToUser(fmt.Sprintf("代理规则已禁用 / proxy rule disabled: %s", ruleID))
				} else {
					feedbackToUser(fmt.Sprintf("未找到代理规则 / proxy rule not found: %s", ruleID))
				}
			} else {
				ruleProxies := make([]string, 0, len(rule.GetEndpointIds()))
				for _, endpointID := range rule.GetEndpointIds() {
					if url := strings.TrimSpace(endpointURLByID[endpointID]); url != "" {
						ruleProxies = append(ruleProxies, url)
					}
				}
				ruleProxies = lo.Uniq(ruleProxies)
				if len(ruleProxies) > 0 {
					for _, pattern := range rule.GetPatterns() {
						pattern = strings.TrimSpace(pattern)
						if pattern == "" {
							continue
						}
						if routeMap == nil {
							routeMap = make(map[string][]string)
						}
						routeMap[pattern] = append(routeMap[pattern], ruleProxies...)
					}
				}
			}
		}
		proxys = lo.Uniq(proxys)
		return proxys, routeMap, nil
	}
	var (
		host                        = "127.0.0.1"
		port                        = 8089
		packetLimit                 = 8 * 10 * 1000 * 1000 // 80M
		enableGMTLS                 = firstReq.GetEnableGMTLS()
		preferGMTLS                 = firstReq.GetPreferGMTLS()
		onlyGMTLS                   = firstReq.GetOnlyEnableGMTLS()
		proxyUsername               = firstReq.GetProxyUsername()
		proxyPassword               = firstReq.GetProxyPassword()
		dnsServers                  = firstReq.GetDnsServers()
		disableSystemProxy          = firstReq.GetDisableSystemProxy()
		forceDisableKeepAlive       = firstReq.GetForceDisableKeepAlive()
		disableCACertPage           = firstReq.GetDisableCACertPage()
		disableWebsocketCompression = firstReq.GetDisableWebsocketCompression()
		randomJA3                   = firstReq.GetRandomJA3()
		filterWebSocket             = utils.NewBool(firstReq.GetFilterWebsocket())
		pluginConcurrency           = firstReq.GetPluginConcurrency()
	)

	if firstReq.GetHost() != "" {
		host = firstReq.GetHost()
	}

	if firstReq.GetPort() > 0 {
		port = int(firstReq.GetPort())
	}

	if pluginConcurrency <= 0 {
		pluginConcurrency = 20
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	downstreamProxy, downstreamProxyRoutes, err := getDownstreamProxy(firstReq)
	if err != nil {
		notifyMITMStartFailed(err.Error())
		log.Errorf("close mitm server for %s, reason: %v", addr, err)
		return err
	}
	hostMapping := make(map[string]string)
	for _, pair := range firstReq.GetHostsMapping() {
		hostMapping[pair.GetKey()] = pair.GetValue()
	}

	if !firstReq.GetEnableProxyAuth() {
		// 如果用户名密码不启用，设置为空
		proxyUsername = ""
		proxyPassword = ""
	}

	// restriction for username
	if strings.Contains(proxyUsername, ":") {
		return utils.Errorf("proxy username cannot contains ':'")
	}

	streamCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	log.Infof("start to create mitm server instance for %v", addr)

	/*
		设置过滤器
	*/
	var (
		filterManager       = GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
		hijackFilterManager = GetMITMHijackFilterManager(s.GetProjectDatabase())
	)

	/*
		设置内容替换模块，通过正则驱动
	*/
	replacer := yakit.NewMITMReplacer(func() []*ypb.MITMContentReplacer {
		result := yakit.GetKey(s.GetProfileDatabase(), yakit.MITMReplacerKeyRecords)
		if result != "" {
			var rules []*ypb.MITMContentReplacer
			_ = json.Unmarshal([]byte(result), &rules)
			if len(rules) > 0 {
				return rules
			}
		}
		return nil
	})

	replacer.AutoSaveCallback(func(items ...*yakit.MITMReplaceRule) {
		if len(items) <= 0 {
			yakit.SetKey(s.GetProfileDatabase(), yakit.MITMReplacerKeyRecords, "[]")
			return
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return
		}
		yakit.SetKey(s.GetProfileDatabase(), yakit.MITMReplacerKeyRecords, string(raw))
	})

	recoverFilterAndReplacerSend := func() {
		sendLogged(&ypb.MITMV2Response{
			JustFilter:          true,
			FilterData:          filterManager.Data,
			JustContentReplacer: true,
			Replacers:           replacer.GetRules(),
		})
	}

	feedbackToUser("初始化过滤器... / initializing filters")

	mitmPluginCaller, err := yak.NewMixPluginCaller()
	if err != nil {
		return utils.Errorf("create mitm plugin manager failed: %s", err)
	}
	mitmPluginCaller.SetFeedback(execFeedback)
	mitmPluginCaller.SetDividedContext(true)
	mitmPluginCaller.SetConcurrent(int(pluginConcurrency))
	mitmPluginCaller.SetLoadPluginTimeout(10)
	mitmPluginCaller.SetCallPluginTimeout(consts.GetGlobalCallerCallPluginTimeout())

	mitmPluginCallerGlobal = mitmPluginCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		close(mitmPluginCallerNotifyChan)
		mitmPluginCallerGlobal = nil
	}()

	allProxies := append([]string{}, downstreamProxy...)
	for _, proxies := range downstreamProxyRoutes {
		allProxies = append(allProxies, proxies...)
	}
	allProxies = lo.Uniq(allProxies)
	if len(allProxies) > 0 {
		mitmPluginCaller.SetProxy(allProxies...)
	}

	cacheDebounce, _ := lo.NewDebounce(1*time.Second, func() {
		sendLogged(&ypb.MITMV2Response{
			HaveNotification:    true,
			NotificationContent: []byte("MITM 插件去重缓存已重置"),
		})
	})

	clearPluginHTTPFlowCache := func() {
		if mitmPluginCaller != nil {
			mitmPluginCaller.ResetFilter()
		}
		cacheDebounce()
	}

	hijackManger := newManualHijackManager()
	hijackListReload := func() {
		sendLogged(&ypb.MITMV2Response{
			ManualHijackListAction: Hijack_List_Reload,
			ManualHijackList:       hijackManger.getHijackingTaskInfo(),
		})
	}

	hijackListFeedback := func(action string, resp ...*ypb.SingleManualHijackInfoMessage) {
		for _, message := range resp { // utf8 check
			if lowhttp.IsMultipartFormDataRequest(message.Request) || !utf8.Valid(message.Request) {
				message.Request = lowhttp.ConvertHTTPRequestToFuzzTag(message.Request)
			}

			if !utf8.Valid(message.Response) {
				message.Response = lowhttp.ConvertHTTPRequestToFuzzTag(message.Response)
			}

			if !utf8.Valid(message.Payload) {
				message.Payload = lowhttp.ConvertHTTPRequestToFuzzTag(message.Payload)
			}
		}
		if hijackManger.isManual() {
			sendLogged(&ypb.MITMV2Response{
				ManualHijackListAction: action,
				ManualHijackList:       resp,
			})
		}
	}

	go func() {
		for {
			reqInstance, err := stream.Recv()
			if err != nil {
				log.Errorf("stream recv error: %v", err)
				return
			}

			if reqInstance.GetRecoverContext() {
				log.Infof("retry recover mitm session")
				sendLogged(&ypb.MITMV2Response{
					JustFilter:          true,
					FilterData:          filterManager.Data,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
				})
				continue
			}

			if reqInstance.GetResetFilter() {
				filterManager.Recover()
				sendLogged(&ypb.MITMV2Response{
					JustFilter: true,
					FilterData: filterManager.Data,
				})
				clearPluginHTTPFlowCache()
				continue
			}

			if reqInstance.GetSetContentReplacers() {
				log.Infof("recv mitm content-replacers[%v]", len(reqInstance.Replacers))
				if len(reqInstance.Replacers) > 0 {
					replacer.SetRules(reqInstance.Replacers...)
				} else {
					log.Infof("remove all content-replacer")
					replacer.SetRules()
				}
				recoverFilterAndReplacerSend()
				clearPluginHTTPFlowCache()
				continue
			}

			// 自动加载所有 MITM 插件（基础插件）
			if reqInstance.GetSetPluginMode() {
				clearPluginHTTPFlowCache()
				if len(reqInstance.GetInitPluginNames()) > 0 {
					plugins := reqInstance.GetInitPluginNames()

					sendLogged(&ypb.MITMV2Response{HaveLoadingSetter: true, LoadingFlag: true})
					swg := utils.NewSizedWaitGroup(50)
					var failedCount, successCount atomic.Int64
					startTime := time.Now()
					for _, script := range yakit.QueryYakScriptByNames(s.GetProfileDatabase(), plugins...) {
						swg.Add()
						script := script
						go func() {
							defer swg.Done()
							err := mitmPluginCaller.LoadPluginEx(
								streamCtx,
								script, reqInstance.GetYakScriptParams()...,
							)
							if err != nil {
								failedCount.Add(1)
								log.Errorf("load %v failed: %s", script.ScriptName, err)
							} else {
								successCount.Add(1)
							}
						}()
					}
					swg.Wait()
					duration := time.Now().Sub(startTime).Seconds()
					sendLogged(&ypb.MITMV2Response{HaveLoadingSetter: true, LoadingFlag: false})
					sendLogged(&ypb.MITMV2Response{HaveNotification: true, NotificationContent: []byte(fmt.Sprintf(
						"初始化加载插件完成，加载成功【%v】个，失败【%v】个, 共耗时 %f 秒。", successCount.Load(), failedCount.Load(), duration,
					))})
				}
				clearPluginHTTPFlowCache()
				continue
			}

			// 设置中间人插件
			if reqInstance.GetSetYakScript() {
				clearPluginHTTPFlowCache()
				script, _ := yakit.GetYakScript(s.GetProfileDatabase(), reqInstance.GetYakScriptID())
				if script != nil {
					if script.Type == "mitm" || script.Type == "port-scan" {
						log.Infof("start to load yakScript[%v]: %v 's capabilities", script.ID, script.ScriptName)
						err = mitmPluginCaller.LoadPluginEx(streamCtx, script, reqInstance.GetYakScriptParams()...)
						if err != nil {
							if len(script.GetParams()) > 0 {
								sendLogged(&ypb.MITMV2Response{HaveNotification: true, NotificationContent: []byte(fmt.Sprintf(
									"加载插件【%s】，参数【%v】失败", script.ScriptName, reqInstance.GetYakScriptParams(),
								))})
							}
							log.Error(err)
						}
					}
				} else if reqInstance.GetYakScriptContent() != "" {
					hotPatchScript := reqInstance.GetYakScriptContent()
					log.Info("start to load yakScriptContent content")
					err := mitmPluginCaller.LoadHotPatch(streamCtx, reqInstance.GetYakScriptParams(), hotPatchScript)
					if err != nil {
						if strings.Contains(err.Error(), "YakVM Panic:") {
							splitErr := strings.SplitN(err.Error(), "YakVM Panic:", 2)
							err = utils.Error(splitErr[1])
						}
						sendLogged(&ypb.MITMV2Response{HaveNotification: true, NotificationContent: []byte(fmt.Sprintf("mitm load hotpatch script error:%v", err))})
					}
				}

				sendLogged(&ypb.MITMV2Response{
					GetCurrentHook: true,
					Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
				})
				continue
			}

			if reqInstance.GetSetClearMITMPluginContext() {
				clearPluginHTTPFlowCache()
				continue
			}

			if reqInstance.GetRemoveHook() {
				clearPluginHTTPFlowCache()
				mitmPluginCaller.GetNativeCaller().Remove(reqInstance.GetRemoveHookParams())
				sendLogged(&ypb.MITMV2Response{
					GetCurrentHook: true,
					Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
				})
				continue
			}

			if reqInstance.GetGetCurrentHook() {
				sendLogged(&ypb.MITMV2Response{
					GetCurrentHook: true,
					Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
				})
				continue
			}

			if reqInstance.GetUpdateFilter() {
				clearPluginHTTPFlowCache()
				filterManager.Update(reqInstance.FilterData)
				filterManager.Save()
				recoverFilterAndReplacerSend()
				continue
			}

			if reqInstance.GetUpdateHijackFilter() {
				if hijackFilterManager == nil {
					hijackFilterManager = NewMITMFilter(reqInstance.HijackFilterData)
				} else {
					hijackFilterManager.Update(reqInstance.HijackFilterData)
				}
				continue
			}

			// 是否过滤ws
			if reqInstance.GetUpdateFilterWebsocket() {
				filterWebSocket.SetTo(reqInstance.GetFilterWebsocket())
				continue
			}

			// 运行时更新代理
			if reqInstance.GetSetDownstreamProxy() {
				downstreamProxy, downstreamProxyRoutes, err := getDownstreamProxy(reqInstance)
				if err == nil && mServer != nil {
					err = mServer.Configure(
						crep.MITM_SetDownstreamProxy(downstreamProxy...),
						crep.MITM_SetDownstreamProxyRoutes(downstreamProxyRoutes),
					)
					if err != nil {
						feedbackToUser(fmt.Sprintf("设置下游代理失败 / set downstream proxy failed: %v", err))
						log.Errorf("set downstream proxy failed: %s", err)
					}
					allProxies := append([]string{}, downstreamProxy...)
					for _, proxies := range downstreamProxyRoutes {
						allProxies = append(allProxies, proxies...)
					}
					allProxies = lo.Uniq(allProxies)
					if len(allProxies) > 0 {
						mitmPluginCaller.SetProxy(allProxies...)
					}
					if len(downstreamProxyRoutes) > 0 {
						feedbackToUser(fmt.Sprintf("设置代理规则成功 / set downstream proxy routes successful: %v", reqInstance.GetDownstreamProxyRuleId()))
					} else {
						feedbackToUser(fmt.Sprintf("设置下游代理成功 / set downstream proxy successful: %v", downstreamProxy))
					}
				}
				continue
			}

			if reqInstance.GetSetAutoForward() {
				autoForwardValue := reqInstance.GetAutoForwardValue()
				hijackManger.setCanRegister(!autoForwardValue)
				continue
			}

			if reqInstance.GetRecoverManualHijack() {
				hijackListReload()
				continue
			}

			if reqInstance.GetManualHijackControl() {
				hijackManger.unicast(reqInstance.GetManualHijackMessage())
				continue
			}
		}
	}()

	feedbackToUser("创建 MITM 服务器 / creating mitm server")

	/*
		设置数据包计数器
	*/
	count := 0
	packetCountLock := new(sync.Mutex)
	addCounter := func() {
		packetCountLock.Lock()
		defer packetCountLock.Unlock()
		count++
	}

	// 缓存 Websocket ID (当前程序的指针，一般不太会有问题)
	/*
		真正开始劫持的函数，以下内容分别针对
		1. 劫持 Websocket 的请求和响应
		2. 劫持普通 HTTP 的请求和响应
		3. 镜像 HTTP 请求和响应
	*/
	var wshashFrameIndexLock sync.Mutex
	wshashFrameIndex := make(map[string]int)
	requireWsFrameIndexByWSHash := func(i string) int {
		/*这个函数目前用在 Hijack 里面，不太需要加锁，因为 mitmLock 已经一般生效了*/
		wshashFrameIndexLock.Lock()
		defer wshashFrameIndexLock.Unlock()
		result, ok := wshashFrameIndex[i]
		if !ok {
			wshashFrameIndex[i] = 1
			return 1
		}
		wshashFrameIndex[i] = result + 1
		return wshashFrameIndex[i]
	}

	handleHijackWsResponse := func(raw []byte, req *http.Request, rsp *http.Response, ts int64) (finalResult []byte) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("(ws) hijack response error: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		/* 这儿比单纯劫持响应要简单的多了 */
		originRspRaw := raw[:]
		finalResult = originRspRaw

		if filterWebSocket.IsSet() {
			return originRspRaw
		}

		wshash := httpctx.GetWebsocketRequestHash(req)
		if wshash == "" {
			wshash = utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		}

		defer func() {
			wsFlow := yakit.BuildWebsocketFlow(true, wshash, requireWsFrameIndexByWSHash(wshash), finalResult[:])
			replacer.HookColorWs(finalResult, wsFlow)
			yakit.SaveWebsocketFlowEx(s.GetProjectDatabase(), wsFlow, func(err error) {
				if err != nil {
					log.Warnf("save websocket flow(from server) failed: %s", err)
				}
			})
		}()

		if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return raw
		}

		feedbackOrigin := &ypb.SingleManualHijackInfoMessage{
			IsHttps:     httpctx.GetRequestHTTPS(req),
			URL:         req.URL.String(),
			RemoteAddr:  httpctx.GetRemoteAddr(req),
			IsWebsocket: true,
			Payload:     raw,
			Status:      Hijack_Status_WS,
			Method:      "WS",
		}

		task := hijackManger.register(feedbackOrigin)
		if task == nil {
			return raw
		}

		hijackListFeedback(Hijack_List_Add, task.infoMessage)
		defer hijackListFeedback(Hijack_List_Delete, task.infoMessage)
		taskInfo := task.infoMessage
		taskInfo.IsWebsocket = true
		taskInfo.Payload = raw

		for {
			select {
			case <-streamCtx.Done():
				return raw
			case controlMessage, ok := <-task.messageChan:
				if !ok {
					return originRspRaw
				}
				if controlMessage.GetDrop() {
					return nil
				}
				if controlMessage.GetForward() {
					return originRspRaw
				}
				if controlMessage.GetSendPacket() {
					task.infoMessage.Payload = controlMessage.GetPayload()
					return task.infoMessage.Payload
				}
			}
		}
	}

	handleHijackResponse := func(isHttps bool, req *http.Request, rspInstance *http.Response, rsp []byte, remoteAddr string) (hijackRsp []byte) {
		pluginCtx := httpctx.GetPluginContext(req)
		urlStr := httpctx.GetRequestURL(req)

		var taskInfo *ypb.SingleManualHijackInfoMessage
		task, ok := hijackManger.getTask(httpctx.GetRequestMITMTaskID(req))
		if task != nil && ok {
			taskInfo = task.infoMessage
			defer hijackManger.unRegister(task.taskID)
			defer hijackListFeedback(Hijack_List_Delete, taskInfo)
		}

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("hijack response error: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}

			newHijackRsp := mitmPluginCaller.CallAfterRequestWithCtx(pluginCtx, isHttps, urlStr, httpctx.GetBareRequestBytes(req), httpctx.GetRequestBytes(req), httpctx.GetBareResponseBytes(req), hijackRsp)
			if len(newHijackRsp) > 0 {
				httpctx.SetResponseModified(req, "yaklang.hook(ex) afterRequest")
				httpctx.SetHijackedResponseBytes(req, newHijackRsp)
				hijackRsp = newHijackRsp
			}
		}()
		plainResponse := getPlainResponseBytes(req)
		if len(plainResponse) > 0 {
			httpctx.SetPlainResponseBytes(req, plainResponse)
			rsp = plainResponse
		}

		// use handled request
		plainRequest := getPlainRequestBytes(req)

		plainResponseHash := codec.Sha256(plainResponse)
		handleResponseModified := func(r []byte) bool {
			if codec.Sha256(r) != plainResponseHash {
				return true
			}
			return false
		}

		dropped := utils.NewBool(false)
		mitmPluginCaller.CallHijackResponseExWithCtx(pluginCtx, isHttps, urlStr, func() interface{} {
			return plainRequest
		}, func() interface{} {
			return plainResponse
		}, constClujore(func(i interface{}) {
			if ret := codec.AnyToBytes(i); handleResponseModified(ret) {
				httpctx.SetResponseModified(req, "yaklang.hook(ex)")
				httpctx.SetHijackedResponseBytes(req, ret)
			}
		}), constClujore(func() {
			dropped.Set()
		}))

		// dropped.
		if !dropped.IsSet() {
			// legacy code
			mitmPluginCaller.CallHijackResponseWithCtx(pluginCtx, isHttps, urlStr, func() interface{} {
				if httpctx.GetResponseIsModified(req) {
					return httpctx.GetHijackedResponseBytes(req)
				} else {
					return plainResponse
				}
			}, constClujore(func(i interface{}) {
				if ret := codec.AnyToBytes(i); handleResponseModified(ret) {
					httpctx.SetResponseModified(req, "yaklang.hook")
					httpctx.SetHijackedResponseBytes(req, ret)
				}
			}), constClujore(func() {
				dropped.Set()
			}))
		}

		if dropped.IsSet() {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
			return nil
		}

		if httpctx.GetResponseIsModified(req) {
			rsp = httpctx.GetHijackedResponseBytes(req)
		}

		httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_AutoFoward, true)
		/*
			自动过滤下，不是所有 response 都应该替换
			应该替换的条件是不匹配过滤器的内容
		*/

		// 处理响应规则
		if replacer.HaveHijackingRules() {
			hookStart := time.Now()
			ruleCount := len(replacer.GetHijackingRules())
			urlStr := httpctx.GetRequestURL(req)
			rules, rspHooked, dropped := replacer.Hook(false, true, urlStr, rsp)
			hookDuration := time.Since(hookStart)

			// 监控慢规则 Hook（超过 300ms）
			if hookDuration > 300*time.Millisecond {
				now := time.Now()
				slowHookItem := &yakit.SlowRuleHookDescription{
					Duration:      hookDuration,
					DurationMs:    hookDuration.Milliseconds(),
					DurationStr:   hookDuration.String(),
					Type:          "hook_response",
					RuleCount:     ruleCount,
					URL:           urlStr,
					Timestamp:     now,
					TimestampUnix: now.Unix(),
				}
				yakit.AddSlowRuleHookItem(slowHookItem)
				yakit.TriggerSlowRuleHookCallbackThrottled()
				log.Warnf("MITM HookResponse took too long: %v, rule_count:%d, url:%s", hookDuration, ruleCount, urlStr)
			}

			if dropped {
				httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
				log.Warn("response should be dropped(VIA replacer.hook)")
				return nil
			}
			httpctx.AppendMatchedRule(req, rules...)
			if handleResponseModified(rspHooked) {
				httpctx.SetResponseModified(req, "yakit.rule.hook")
				httpctx.SetHijackedResponseBytes(req, rspHooked)
				rsp = rspHooked
			}
		}

		ptr := fmt.Sprintf("%p", req)
		if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return rsp
		}

		rsp, _, err := lowhttp.FixHTTPResponse(rsp)
		if err != nil {
			log.Errorf("fix http response packet failed: %s", err)
			return rsp
		}

		if taskInfo == nil || task == nil { // task not registered ,or deleted, should not hijack
			return rsp
		}

		var traceInfo *lowhttp.LowhttpTraceInfo
		if i, ok := httpctx.GetResponseTraceInfo(req).(*lowhttp.LowhttpTraceInfo); ok {
			traceInfo = i
		}

		taskInfo.Status = Hijack_Status_Response
		taskInfo.Request = httpctx.GetRequestBytes(req)
		taskInfo.Response = rsp
		taskInfo.TraceInfo = model.ToLowhttpTraceInfoGRPCModel(traceInfo)
		httpctx.SetResponseViewedByUser(req)

		for {
			hijackListFeedback(Hijack_List_Update, taskInfo)
			select {
			case <-streamCtx.Done():
				return rsp
			case controlMessage, ok := <-task.messageChan:
				if !ok {
					return rsp
				}

				if controlMessage.GetUpdateTags() {
					taskInfo.Tags = controlMessage.GetTags()
					httpctx.SetFlowTags(req, controlMessage.GetTags())
				}

				if controlMessage.GetDrop() {
					return nil
				}
				if controlMessage.GetForward() {
					return rsp
				}

				if controlMessage.GetSendPacket() {
					response := controlMessage.GetResponse()
					taskInfo.Response = response
					if handleResponseModified(response) {
						httpctx.SetResponseModified(req, "manual")
						httpctx.SetHijackedResponseBytes(req, response)
					}
					rspModified, _, err := lowhttp.FixHTTPResponse(response)
					if err != nil {
						log.Errorf("fix http response[req:%v] failed: %s", ptr, err.Error())
						return rsp
					}
					if rspModified == nil {
						log.Error("BUG: http response is empty... use origin")
						return rsp
					}
					return rspModified
				}
			}
		}
	}
	handleHijackWsRequest := func(raw []byte, req *http.Request, rsp *http.Response, ts int64) (finalResult []byte) {
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("hijack ws websocket failed: %s", err)
				return
			}
		}()

		if filterWebSocket.IsSet() {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return raw
		}

		_, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		var extName string
		u, _ := url.Parse(urlStr)
		if ret := path.Ext(u.EscapedPath()); ret != "" {
			extName = ret
			if !strings.HasPrefix(extName, ".") {
				extName = "." + extName
			}
		}

		if !filterManager.IsPassed(req.Method, req.Host, urlStr, extName) {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return raw
		}
		wshash := httpctx.GetWebsocketRequestHash(req)
		if wshash == "" {
			wshash = utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		}

		originReqRaw := raw[:]
		finalResult = originReqRaw

		defer func() {
			wsFlow := yakit.BuildWebsocketFlow(false, wshash, requireWsFrameIndexByWSHash(wshash), finalResult[:])
			replacer.HookColorWs(finalResult, wsFlow)
			yakit.SaveWebsocketFlowEx(s.GetProjectDatabase(), wsFlow, func(err error) {
				if err != nil {
					log.Warnf("save websocket flow(from server) failed: %s", err)
				}
			})
		}()
		// 条件劫持
		if hijackFilterManager != nil && !hijackFilterManager.IsEmpty() && hijackFilterManager.IsPassed(req.Method, req.Host, urlStr, extName) {
			log.Infof("[mitm] hijack ws request by hijack filter")
			hijackManger.setCanRegister(true)
		}

		var encode []string
		if utils.IsGzip(raw) {
			encode = append(encode, "gzip")
		}
		feedbackOrigin := &ypb.SingleManualHijackInfoMessage{
			IsHttps:         httpctx.GetRequestHTTPS(req),
			URL:             urlStr,
			RemoteAddr:      httpctx.GetRemoteAddr(req),
			IsWebsocket:     true,
			Payload:         raw,
			Status:          Hijack_Status_WS,
			WebsocketEncode: encode,
			Method:          "WS",
		}

		task := hijackManger.register(feedbackOrigin)
		if task == nil {
			return raw
		}
		hijackListFeedback(Hijack_List_Add, task.infoMessage)

		defer hijackManger.unRegister(task.taskID)
		defer hijackListFeedback(Hijack_List_Delete, task.infoMessage)

		for {
			select {
			case <-streamCtx.Done():
				return raw
			case controlMessage, ok := <-task.messageChan:
				if !ok {
					return raw
				}
				if controlMessage.GetDrop() {
					return nil
				}
				if controlMessage.GetForward() {
					return raw
				}
				if controlMessage.GetSendPacket() {
					task.infoMessage.Payload = controlMessage.GetPayload()
					return task.infoMessage.Payload
				}
			}
		}
	}

	handleHijackRequest := func(isHttps bool, originReqIns *http.Request, req []byte) (hijackReq []byte) {
		setModifiedRequest := func(id string, req []byte) {
			httpctx.SetRequestModified(originReqIns, id)
			httpctx.SetHijackedRequestBytes(originReqIns, req)
			// url from plainRequest
			var reqURL string
			if isHttps {
				reqURL = lowhttp.GetUrlFromHTTPRequest("https", req)
			} else {
				reqURL = lowhttp.GetUrlFromHTTPRequest("http", req)
			}
			httpctx.SetRequestURL(originReqIns, reqURL)
		}

		httpctx.SetResponseContentTypeFiltered(originReqIns, func(t string) bool {
			ret := !filterManager.IsMIMEPassed(t)
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ResponseIsFiltered, ret)
			return ret
		})

		httpctx.SetMatchedRule(originReqIns, make([]*ypb.MITMContentReplacer, 0))
		fixReq := lowhttp.FixHTTPRequest(req)
		fixReqIns, _ := lowhttp.ParseBytesToHttpRequest(fixReq)
		method := originReqIns.Method

		// make it plain
		plainRequest := getPlainRequestBytes(originReqIns)

		// handle rules
		originRequestHash := codec.Sha256(plainRequest)

		// modified ?
		handleRequestModified := func(newReqBytes []byte) bool {
			return codec.Sha256(newReqBytes) != originRequestHash
		}

		defer func() {
			if err := recover(); err != nil {
				log.Warnf("Hijack warning: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		/* 由 MITM Hooks 触发 */
		var (
			dropped  = utils.NewBool(false)
			urlStr   = ""
			hostname = originReqIns.Host
			extName  = ""
		)
		urlRaw, err := lowhttp.ExtractURLFromHTTPRequest(originReqIns, isHttps)
		if err != nil {
			log.Errorf("extract url from request failed: %s", err)
			fmt.Println(string(req))
		}
		if urlRaw != nil {
			urlStr = urlRaw.String()
			hostname = urlRaw.Host
			if ret := path.Ext(urlRaw.EscapedPath()); ret != "" {
				extName = ret
				if !strings.HasPrefix(extName, ".") {
					extName = "." + extName
				}
			}
			if strings.ToUpper(method) == "CONNECT" {
				urlStr = hostname
			}
			httpctx.SetRequestURL(originReqIns, urlStr)
		}

		// 过滤
		if !filterManager.IsPassed(method, hostname, urlStr, extName) {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return req
		}

		pluginCtx := httpctx.GetPluginContext(originReqIns)

		defer func() {
			newHijackReq := mitmPluginCaller.CallBeforeRequestWithCtx(pluginCtx, isHttps, urlStr, httpctx.GetBareRequestBytes(originReqIns), hijackReq)
			if len(newHijackReq) > 0 && handleRequestModified(newHijackReq) {
				hijackReq = newHijackReq
				setModifiedRequest("yaklang.hook beforeRequest", hijackReq)
			}
		}()

		hookStart := time.Now()
		ruleCount := len(replacer.GetHijackingRules())
		urlStr = httpctx.GetRequestURL(originReqIns)
		rules, req1, shouldBeDropped := replacer.Hook(true, false, urlStr, req, isHttps)
		hookDuration := time.Since(hookStart)

		// 监控慢规则 Hook（超过 300ms）
		if hookDuration > 300*time.Millisecond {
			now := time.Now()
			slowHookItem := &yakit.SlowRuleHookDescription{
				Duration:      hookDuration,
				DurationMs:    hookDuration.Milliseconds(),
				DurationStr:   hookDuration.String(),
				Type:          "hook_request",
				RuleCount:     ruleCount,
				URL:           urlStr,
				Timestamp:     now,
				TimestampUnix: now.Unix(),
			}
			yakit.AddSlowRuleHookItem(slowHookItem)
			yakit.TriggerSlowRuleHookCallbackThrottled()
			log.Warnf("MITM HookRequest took too long: %v, rule_count:%d, url:%s", hookDuration, ruleCount, urlStr)
		}

		if shouldBeDropped {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
			log.Warn("MITM: request dropped by hook (VIA replacer.hook)")
			return nil
		}
		httpctx.AppendMatchedRule(originReqIns, rules...)

		modifiedByRule := false
		if handleRequestModified(req1) {
			req = req1
			modifiedByRule = true
			setModifiedRequest("yakit.mitm.replacer", req1)
		}

		mitmPluginCaller.CallMockHTTPRequestWithCtx(pluginCtx, isHttps, urlStr,
			func() interface{} {
				if modifiedByRule {
					return httpctx.GetHijackedRequestBytes(originReqIns)
				}
				return getPlainRequestBytes(originReqIns)
			}, func(rsp interface{}) {
				rspBytes := codec.AnyToBytes(rsp)
				fixedRsp, _, _ := lowhttp.FixHTTPResponse(rspBytes)
				if fixedRsp == nil {
					log.Warnf("failed to fix mock response, using 502 Bad Gateway")
					fixedRsp = []byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
				}
				httpctx.SetMockResponseBytes(originReqIns, fixedRsp)
				httpctx.SetShouldMockResponse(originReqIns, true)
			})

		mitmPluginCaller.CallHijackRequestWithCtx(pluginCtx, isHttps, urlStr,
			func() interface{} {
				if modifiedByRule {
					return httpctx.GetHijackedRequestBytes(originReqIns)
				}
				return getPlainRequestBytes(originReqIns)
			}, constClujore(func(replaced interface{}) {
				if dropped.IsSet() {
					return
				}
				if replaced != nil {
					if ret := codec.AnyToBytes(replaced); handleRequestModified(ret) {
						setModifiedRequest("yaklang.hook", lowhttp.FixHTTPRequest(ret))
					}
				}
			}),
			constClujore(func() {
				httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
				dropped.Set()
			}))

		// 如果丢弃就直接丢！
		if dropped.IsSet() {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
			return nil
		}

		if httpctx.GetRequestIsModified(originReqIns) {
			req = httpctx.GetHijackedRequestBytes(originReqIns)
		}

		// 条件劫持
		if hijackFilterManager != nil && !hijackFilterManager.IsEmpty() && hijackFilterManager.IsPassed(method, hostname, urlStr, extName) {
			log.Infof("[mitm] hijack request by hijack filter")
			hijackManger.setCanRegister(true)
		}

		feedbackOrigin := &ypb.SingleManualHijackInfoMessage{
			Request:    req,
			IsHttps:    isHttps,
			URL:        urlStr,
			RemoteAddr: httpctx.GetRemoteAddr(originReqIns),
			Method:     lowhttp.GetHTTPRequestMethod(req),
			Status:     Hijack_Status_Request,
		}

		task := hijackManger.register(feedbackOrigin)
		if task == nil {
			return req
		}

		taskInfo := task.infoMessage

		httpctx.SetResponseContentTypeFiltered(originReqIns, func(t string) bool { // update callback set resp filter feedback
			ret := !filterManager.IsMIMEPassed(t)
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ResponseIsFiltered, ret)
			if ret && httpctx.GetContextBoolInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
				hijackListFeedback(Hijack_List_Delete, taskInfo)
				hijackManger.unRegister(task.taskID)
			}
			return ret
		})

		hijackListFeedback(Hijack_List_Add, taskInfo)
		httpctx.SetRequestMITMTaskID(originReqIns, task.taskID)
		httpctx.SetResponseViewedByUser(originReqIns)
		defer func() {
			if !taskInfo.HijackResponse {
				hijackListFeedback(Hijack_List_Delete, taskInfo)
				hijackManger.unRegister(task.taskID)
			} else {
				taskInfo.Status = Hijack_Status_Waiting
				hijackListFeedback(Hijack_List_Update, taskInfo)
			}
		}()
		for {
			hijackListFeedback(Hijack_List_Update, taskInfo)
			select {
			case <-streamCtx.Done():
				return req
			case controlReq, ok := <-task.messageChan:
				if !ok {
					return req
				}
				if controlReq.GetHijackResponse() {
					taskInfo.HijackResponse = true
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					log.Infof("the ptr: %p's mitm request is waiting for hijack response", originReqIns)
				}

				if controlReq.GetCancelHijackResponse() {
					taskInfo.HijackResponse = false
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false)
					log.Infof("the ptr: %p's mitm request cancel hijack response", originReqIns)
				}

				if controlReq.GetUpdateTags() {
					taskInfo.Tags = controlReq.GetTags()
					httpctx.SetFlowTags(originReqIns, controlReq.GetTags())
				}

				if controlReq.GetDrop() {
					log.Infof("MITM %v recv drop hijacked request[%v]", addr, task.taskID)
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false) // 设置无需劫持resp
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
					startCreateFlow := time.Now()
					flow, err := yakit.CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps, originReqIns, "mitm", originReqIns.URL.String(), remoteAddr, yakit.CreateHTTPFlowWithRequestIns(fixReqIns))
					if err != nil {
						log.Errorf("save http flow[%v %v] from mitm failed: %s", originReqIns.Method, originReqIns.URL.String(), err)
						return nil
					}
					flow.Hash = flow.CalcHash()
					flow.AddTagToFirst("[被丢弃]")
					flow.Purple()

					log.Debugf("mitmPluginCaller.HijackSaveHTTPFlow for %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
					startCreateFlow = time.Now()

					flow.Hash = flow.CalcHash()
					flow.StatusCode = 200 // 这里先设置成200
					flow.Response = ""
					for i := 0; i < 3; i++ {
						startCreateFlow = time.Now()
						// 用户丢弃请求后，这个flow表现在http history中应该是不包含响应的
						err = yakit.InsertHTTPFlow(s.GetProjectDatabase(), flow)
						log.Debugf("insert http flow %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
						if err != nil {
							log.Errorf("create / save httpflow from mirror error: %s", err)
							time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
							continue
						}
						break
					}
					return nil
				}

				if controlReq.GetForward() {
					return req
				}

				if controlReq.GetSendPacket() {
					// 这里是用户自定义的请求
					current := controlReq.GetRequest()
					taskInfo.Request = current // use for front ,should not render fuzztag
					if bytes.Contains(current, []byte{'{', '{'}) || bytes.Contains(current, []byte{'}', '}'}) {
						// 在这可能包含 fuzztag
						result := mutate.MutateQuick(current)
						if len(result) > 0 {
							current = []byte(result[0])
						}
					}
					if handleRequestModified(current) {
						setModifiedRequest("user", current)
					}
					return current
				}
			}
		}
	}

	handleMirrorResponse := func(isHttps bool, reqUrl string, req *http.Request, rsp *http.Response, remoteAddr string) {
		addCounter()

		// 不符合劫持条件就不劫持
		isFilteredByResponse := httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ResponseIsFiltered)
		isFilteredByRequest := httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered)
		isRequestModified := httpctx.GetRequestIsModified(req)
		isResponseModified := httpctx.GetResponseIsModified(req)
		isResponseDropped := httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped)
		isFiltered := isFilteredByResponse || isFilteredByRequest || (filterWebSocket.IsSet() && httpctx.GetIsWebWebsocketRequest(req))
		isViewed := httpctx.GetRequestViewedByUser(req) || httpctx.GetResponseViewedByUser(req)
		isModified := isRequestModified || isResponseModified

		plainRequest := getPlainRequestBytes(req)
		plainResponse := getPlainResponseBytes(req)
		responseOverSize := false
		if len(plainResponse) > packetLimit {
			responseOverSize = true
		}
		header, body := lowhttp.SplitHTTPPacketFast(plainResponse)
		if responseOverSize {
			plainResponse = []byte(header)
		}

		shouldBeHijacked := !isFiltered

		pluginCtx := httpctx.GetPluginContext(req)
		go func() {
			mitmPluginCaller.MirrorHTTPFlowWithCtx(pluginCtx, isHttps, reqUrl, plainRequest, plainResponse, body, shouldBeHijacked)
		}()
		// 劫持过滤
		if isFiltered {
			return
		}
		saveBarePacketHandler := func(id uint) {
			// 存储KV，将flow ID作为key，bare request和bare response作为value
			if httpctx.GetRequestIsModified(req) {
				bareReq := httpctx.GetPlainRequestBytes(req)
				if len(bareReq) == 0 {
					bareReq = httpctx.GetBareRequestBytes(req)
				}
				log.Debugf("[KV] save bare Request(%d)", id)

				if len(bareReq) > 0 && id > 0 {
					keyStr := strconv.FormatUint(uint64(id), 10) + "_request"
					yakit.SetProjectKeyWithGroup(s.GetProjectDatabase(), keyStr, bareReq, yakit.BARE_REQUEST_GROUP)
				}
			}

			if httpctx.GetResponseIsModified(req) || isResponseDropped {
				bareRsp := httpctx.GetPlainResponseBytes(req)
				if len(bareRsp) == 0 {
					bareRsp = httpctx.GetBareResponseBytes(req)
				}
				log.Debugf("[KV] save bare Response(%d)", id)

				if len(bareRsp) > 0 && id > 0 {
					keyStr := strconv.FormatUint(uint64(id), 10) + "_response"
					yakit.SetProjectKeyWithGroup(s.GetProjectDatabase(), keyStr, bareRsp, yakit.BARE_RESPONSE_GROUP)
				}
			}
		}

		// 保存到数据库
		log.Debugf("start to create httpflow from mitm[%v %v]", req.Method, truncate(reqUrl))
		startCreateFlow := time.Now()
		var flow *schema.HTTPFlow
		if httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_NOLOG) {
			flow, err = yakit.CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps, req, "mitm", reqUrl, remoteAddr)
			flow.StatusCode = 200 // 先设置成200
		} else {
			var duration time.Duration
			if i, ok := httpctx.GetResponseTraceInfo(req).(*lowhttp.LowhttpTraceInfo); ok {
				duration = i.TotalTime
			}
			flow, err = yakit.CreateHTTPFlowFromHTTPWithBodySaved(isHttps, req, rsp, "mitm", reqUrl, remoteAddr, yakit.CreateHTTPFlowWithDuration(duration)) // , !responseOverSize)
		}
		if err != nil {
			log.Errorf("save http flow[%v %v] from mitm failed: %s", req.Method, reqUrl, err)
			return
		}
		log.Debugf("yakit.CreateHTTPFlowFromHTTPWithBodySaved for %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))
		startCreateFlow = time.Now()
		// 额外，获取进程名
		if name := httpctx.GetProcessName(req); name != "" {
			flow.ProcessName = sql.NullString{
				String: filepath.Base(name),
				Valid:  true,
			}
		}

		flow.Hash = flow.CalcHash()
		if isViewed {
			if isModified {
				flow.AddTagToFirst("[手动修改]")
				flow.Orange()
			} else {
				flow.AddTagToFirst("[手动劫持]")
				flow.Yellow()
			}
		}
		if isResponseDropped {
			flow.AddTagToFirst("[响应被丢弃]")
			flow.Purple()
		}

		// 处理ws升级请求包
		if httpctx.GetIsWebWebsocketRequest(req) {
			flow.IsWebsocket = true
			wshash := httpctx.GetWebsocketRequestHash(req)
			flow.WebsocketHash = wshash
			flow.HiddenIndex = wshash
		}
		hijackedFlowMutex := new(sync.Mutex)
		isDroppedSaveFlow := utils.NewBool(false)

		pluginCh := make(chan struct{})
		mitmPluginCaller.HijackSaveHTTPFlowEx(
			pluginCtx,
			flow,
			func() {
				close(pluginCh)
			},
			func(replaced *schema.HTTPFlow) {
				if replaced == nil {
					return
				}
				hijackedFlowMutex.Lock()
				defer hijackedFlowMutex.Unlock()

				*flow = *replaced
			},
			func() {
				isDroppedSaveFlow.Set()
			},
		)
		log.Debugf("mitmPluginCaller.HijackSaveHTTPFlow for %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))

		// storage
		flow.Hash = flow.CalcHash()
		startCreateFlow = time.Now()
		colorCh := make(chan struct{})
		var extracted []*schema.ExtractedData

		// replacer hook color
		if replacer != nil {
			go func() {
				hookStart := time.Now()
				ruleCount := len(replacer.GetMirrorRules())
				urlStr := httpctx.GetRequestURL(req)
				extracted = replacer.HookColor(plainRequest, plainResponse, req, flow)
				hookDuration := time.Since(hookStart)
				close(colorCh)

				// 监控慢规则 Hook（超过 300ms）
				if hookDuration > 300*time.Millisecond {
					now := time.Now()
					slowHookItem := &yakit.SlowRuleHookDescription{
						Duration:      hookDuration,
						DurationMs:    hookDuration.Milliseconds(),
						DurationStr:   hookDuration.String(),
						Type:          "hook_color",
						RuleCount:     ruleCount,
						URL:           urlStr,
						Timestamp:     now,
						TimestampUnix: now.Unix(),
					}
					yakit.AddSlowRuleHookItem(slowHookItem)
					yakit.TriggerSlowRuleHookCallbackThrottled()
					log.Warnf("MITM HookColor took too long: %v, rule_count:%d, url:%s", hookDuration, ruleCount, urlStr)
				}

				for _, e := range extracted {
					err = yakit.CreateOrUpdateExtractedDataEx(-1, e)
					if err != nil {
						log.Errorf("save hookcolor error: %s", err)
					}
				}
			}()
		} else {
			close(colorCh)
		}

		var needUpdate bool
		timeoutCtx, timeCancel := context.WithTimeout(streamCtx, 300*time.Millisecond)
		defer timeCancel()
		select {
		case <-colorCh:
		case <-timeoutCtx.Done(): // wait for max 300ms
			needUpdate = true
		}

		select {
		case <-pluginCh:
		case <-timeoutCtx.Done(): // wait for max 300ms
			needUpdate = true
		}

		if !isDroppedSaveFlow.IsSet() {
			// 额外添加用户手动设置的标签，确保其优先级最高
			userTags := httpctx.GetFlowTags(req)
			if len(userTags) > 0 {
				flow.AddTagToFirst(userTags...)
			}
			tags := flow.Tags
			err := yakit.InsertHTTPFlowEx(flow, false, func() {
				saveBarePacketHandler(flow.ID)
			})
			if err != nil {
				log.Errorf("create / save httpflow from mirror error: %s", err)
			} else if needUpdate {
				go func() {
					<-colorCh
					<-pluginCh
					if tags != flow.Tags {
						err := yakit.UpdateHTTPFlowTagsEx(flow)
						if err != nil {
							log.Errorf("update http flow tags error: %s", err)
						}
					}
				}()
			}

			log.Debugf("insert http flow %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))
		}
	}
	// 核心 MITM 服务器
	var opts []crep.MITMConfig
	for _, cert := range firstReq.GetCertificates() {
		opts = append(opts, crep.MITM_MutualTLSClient(cert.CrtPem, cert.KeyPem, cert.GetCaCertificates()...))
	}

	extraIncome := yaklib.DefaultMitmExtraConnManager.Register(streamCtx, yaklib.DefaultGRPCMitmKey)
	defer yaklib.DefaultMitmExtraConnManager.Unregister(yaklib.DefaultGRPCMitmKey)
	wrapperConnChan := make(chan *minimartian.WrapperedConn, 100)
	publicIP, err := netutil.GetPublicHost()
	if err != nil {
		log.Errorf("get public route error: %s", err)
	}
	go func() {
		for {
			select {
			case conn := <-extraIncome.OutputChannel():
				wrapperConnChan <- minimartian.NewWrapperedConnWithStrongLocalHost(conn, publicIP.String(), nil)
			case <-streamCtx.Done():
				return
			}
		}
	}()

	opts = append(opts,
		crep.MITM_EnableMITMCACertPage(!disableCACertPage),
		crep.MITM_EnableWebsocketCompression(!disableWebsocketCompression),
		crep.MITM_RandomJA3(randomJA3),
		crep.MITM_ProxyAuth(proxyUsername, proxyPassword),
		crep.MITM_SetDisableSystemProxy(disableSystemProxy),
		crep.MITM_SetHijackedMaxContentLength(packetLimit),
		crep.MITM_SetDownstreamProxy(downstreamProxy...),
		crep.MITM_SetDownstreamProxyRoutes(downstreamProxyRoutes),
		crep.MITM_SetHTTPResponseHijackRaw(handleHijackResponse),
		crep.MITM_SetHTTPRequestHijackRaw(handleHijackRequest),
		crep.MITM_SetWebsocketRequestHijackRaw(handleHijackWsRequest),
		crep.MITM_SetWebsocketResponseHijackRaw(handleHijackWsResponse),
		crep.MITM_SetHTTPResponseMirror(handleMirrorResponse),
		crep.MITM_SetWebsocketHijackMode(true),
		crep.MITM_SetHTTP2(firstReq.GetEnableHttp2()),
		crep.MITM_MergeOptions(opts...),
		crep.MITM_SetGM(enableGMTLS),
		crep.MITM_SetGMPrefer(preferGMTLS),
		crep.MITM_SetGMOnly(onlyGMTLS),
		crep.MITM_SetFindProcessName(true),
		crep.MITM_SetDNSServers(dnsServers...),
		crep.MITM_SetHostMapping(hostMapping),
		crep.MITM_SetHTTPForceClose(forceDisableKeepAlive),
		crep.MITM_SetMaxReadWaitTime(time.Duration(firstReq.GetMaxReadWaitTime())*time.Second),
		crep.MITM_SetExtraIncomingConnectionChannel(wrapperConnChan),
	)

	// 如果 mitm 启动时进行设置，优先使用mitm中的设置
	if firstReq.GetMaxContentLength() != 0 && firstReq.GetMaxContentLength() <= 10*1024*1024 {
		opts = append(opts, crep.MITM_SetMaxContentLength(firstReq.GetMaxContentLength()))
	}
	mServer, err = crep.NewMITMServer(opts...)
	if err != nil {
		log.Error(err)
		return err
	}

	// 发送第一个设置状态
	recoverFilterAndReplacerSend()
	// 发送第二个来设置 replacer
	recoverFilterAndReplacerSend()

	log.Infof("start serve mitm server for %s", addr)
	// err = mServer.Run(ctx)
	err = mServer.ServeWithListenedCallback(streamCtx, utils.HostPort(host, port), func() {
		feedbackToUser("MITM 服务器已启动 / starting mitm server")
	})

	if err != nil {
		log.Errorf("close mitm server for %s, reason: %v", addr, err)
		return err
	}

	return nil
}

type manualHijackTask struct {
	taskID      string
	messageChan <-chan *ypb.SingleManualHijackControlMessage
	infoMessage *ypb.SingleManualHijackInfoMessage
}

type manualHijackManager struct {
	hijackTask      *omap.OrderedMap[string, *manualHijackTask]
	messageChan     map[string]chan<- *ypb.SingleManualHijackControlMessage
	manualHijacking bool
	hijackLock      sync.Mutex
}

func newManualHijackManager() *manualHijackManager {
	return &manualHijackManager{
		hijackTask:      omap.NewOrderedMap[string, *manualHijackTask](make(map[string]*manualHijackTask)),
		messageChan:     make(map[string]chan<- *ypb.SingleManualHijackControlMessage),
		manualHijacking: false,
	}
}

func (m *manualHijackManager) setCanRegister(b bool) {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	if !b {
		for _, id := range m.hijackTask.Keys() {
			m.hijackTask.Delete(id)
			if ch, ok := m.messageChan[id]; ok {
				close(ch)
				delete(m.messageChan, id)
			}
		}
	}
	m.manualHijacking = b
}

func (m *manualHijackManager) getHijackingTaskInfo() []*ypb.SingleManualHijackInfoMessage {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	var tasks []*ypb.SingleManualHijackInfoMessage
	m.hijackTask.ForEach(func(key string, value *manualHijackTask) bool {
		tasks = append(tasks, value.infoMessage)
		return true
	})
	return tasks
}

func (m *manualHijackManager) isManual() bool {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	return m.manualHijacking
}

func (m *manualHijackManager) register(resp *ypb.SingleManualHijackInfoMessage) *manualHijackTask {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	if !m.manualHijacking {
		return nil
	}
	id := ksuid.New().String()
	ch := make(chan *ypb.SingleManualHijackControlMessage, 2)

	resp.TaskID = id
	m.messageChan[id] = ch
	task := &manualHijackTask{
		taskID:      id,
		messageChan: ch,
		infoMessage: resp,
	}
	m.hijackTask.Set(id, task)
	return task
}

func (m *manualHijackManager) getTask(taskID string) (*manualHijackTask, bool) {
	return m.hijackTask.Get(taskID)
}

func (m *manualHijackManager) unRegister(id string) {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	m.hijackTask.Delete(id)
	if ch, ok := m.messageChan[id]; ok {
		close(ch)
		delete(m.messageChan, id)
	}
}

func (m *manualHijackManager) unicast(req *ypb.SingleManualHijackControlMessage) {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	if ch, ok := m.messageChan[req.GetTaskID()]; ok {
		ch <- req
	}
}

func (m *manualHijackManager) broadcast(req *ypb.SingleManualHijackControlMessage) {
	m.hijackLock.Lock()
	defer m.hijackLock.Unlock()
	m.broadcastNeedLock(req)
}

func (m *manualHijackManager) broadcastNeedLock(req *ypb.SingleManualHijackControlMessage) {
	for _, ch := range m.messageChan {
		ch <- req
	}
}

// PluginTrace 实现插件执行跟踪的双向流服务
func (s *Server) PluginTrace(stream ypb.Yak_PluginTraceServer) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("plugin trace panic: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	log.Info("插件跟踪流连接建立")

	emptyResp := &ypb.PluginTraceResponse{
		ResponseType: "control_result",
		Success:      false,
	}

	var mitmPluginCaller *yak.MixPluginCaller

	// 检测MixPluginCaller是否可用，不可用则定期检测
	for {
		mitmPluginCaller = mitmPluginCallerGlobal
		if mitmPluginCaller == nil || mitmPluginCallerNotifyChan == nil {
			log.Debug("MITM 插件管理器未初始化，返回空trace列表，等待后端启动MITMv2...")
			_ = stream.Send(emptyResp)
			// 等待1秒后重试
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				log.Info("client close the plugin trace stream during plugin caller check loop")
				return nil
			}
		} else {
			// 检测到可用，进入正常trace推送逻辑
			break
		}
	}

	go func() {
		select {
		case <-mitmPluginCallerNotifyChan:
			log.Info("plugin trace shutdown due to MITM closed")
			cancel()
		}
	}()

	// 进入正常trace推送逻辑
	manager := mitmPluginCaller.GetNativeCaller()

	// 创建trace更新通道，增大容量以处理高并发
	traceUpdateChan := make(chan *yak.PluginExecutionTrace, 1000)

	// 用于跟踪已推送给前端的trace
	pushedTraces := make(map[string]bool) // traceID -> isPushed
	pushedTracesMutex := sync.RWMutex{}

	debouncedLog, _ := lo.NewDebounce(time.Second*5, func() { log.Warn("trace update channel is full, dropping trace update") })

	// 设置callback，推送状态变更（不包括running状态的时间检测）
	_, removeCallback := manager.AddExecutionTraceCallback(func(trace *yak.PluginExecutionTrace) {
		pushedTracesMutex.Lock()
		defer pushedTracesMutex.Unlock()

		wasPushed := pushedTraces[trace.TraceID]
		shouldPush := false

		switch trace.Status {
		case yak.PluginStatusCancelled:
			// cancelled状态总是推送（可能是用户手动取消或系统取消）
			shouldPush = true
		case yak.PluginStatusFailed:
			// failed状态总是推送（可能是长时间运行后失败）
			shouldPush = true
		case yak.PluginStatusCompleted:
			// 如果这个trace之前已经推送过（因为长时间运行），现在完成了，需要推送完成状态
			// 前端收到completed状态后应该清除这条trace
			shouldPush = wasPushed
		case yak.PluginStatusRunning:
			// running状态不在这里处理，由定时检查处理
			shouldPush = false
		}

		if shouldPush {
			// 记录已推送状态
			pushedTraces[trace.TraceID] = true

			select {
			case traceUpdateChan <- trace:
			case <-ctx.Done():
				log.Info("trace won't sent to client due to stream close")
				return
			default:
				debouncedLog()
			}

			if trace.Status != yak.PluginStatusRunning {
				delete(pushedTraces, trace.TraceID)
			}
		}
	})
	if removeCallback != nil {
		defer func() {
			removeCallback()
			log.Info("plugin trace callback removed")
		}()
	}
	// 启动goroutine处理trace批量更新推送和定时stats推送
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("trace update goroutine panic: %v", err)
			}
			log.Info("trace push goroutine exit")
		}()

		// 批量处理的缓冲区
		traceBatch := make([]*yak.PluginExecutionTrace, 0, 50)

		// 定时器：2秒推送一次stats
		statsTicker := time.NewTicker(2 * time.Second)
		defer statsTicker.Stop()

		// 批量处理定时器：500ms收集一批trace进行推送
		batchTicker := time.NewTicker(500 * time.Millisecond)
		defer batchTicker.Stop()

		// 长时间运行trace检测定时器：每1秒检查一次
		longRunningTicker := time.NewTicker(1 * time.Second)
		defer longRunningTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case trace, ok := <-traceUpdateChan:
				if !ok {
					return
				}
				// 收集trace到批处理缓冲区
				traceBatch = append(traceBatch, trace)

				// 如果批次达到上限，立即发送
				if len(traceBatch) >= 50 {
					s.sendTraceBatch(stream, traceBatch)
					traceBatch = traceBatch[:0] // 清空缓冲区
				}

			case <-batchTicker.C:
				// 定时发送批次（即使没有达到上限）
				if len(traceBatch) > 0 {
					s.sendTraceBatch(stream, traceBatch)
					traceBatch = traceBatch[:0] // 清空缓冲区
				}

			case <-statsTicker.C:
				// 定时推送统计信息
				stats := s.getPluginTraceStats(manager)
				resp := &ypb.PluginTraceResponse{
					ResponseType: "stats_update",
					Stats:        stats,
					Success:      true,
				}

				if err := stream.Send(resp); err != nil {
					log.Warnf("发送stats更新失败: %v", err)
					return
				}

			case <-longRunningTicker.C:
				// 检查长时间运行的trace
				s.checkLongRunningTraces(manager, traceUpdateChan, &pushedTraces, &pushedTracesMutex)
			}
		}
	}()

	// 处理客户端请求
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Info("plugin trace stream closed")
				return nil
			}
			log.Warnf("PluginTrace Recv error: %v", err)
			return nil
		}

		response := s.handlePluginTraceRequest(manager, req)
		_ = stream.Send(response)
	}
}

// handlePluginTraceRequest 处理插件跟踪请求
func (s *Server) handlePluginTraceRequest(manager *yak.YakToCallerManager, req *ypb.PluginTraceRequest) *ypb.PluginTraceResponse {
	switch req.ControlMode {
	case "start_stream":
		manager.EnableExecutionTracing(req.EnableTracing)
		return &ypb.PluginTraceResponse{
			ResponseType: "control_result",
			Success:      true,
		}

	case "cancel_trace":
		// 取消特定的跟踪
		if req.TraceID == "" {
			return &ypb.PluginTraceResponse{
				ResponseType: "control_result",
				Success:      false,
				Message:      "TraceID不能为空",
			}
		}

		success := manager.CancelExecutionTrace(req.TraceID)
		message := "取消成功"
		if !success {
			message = "取消失败：找不到指定的跟踪记录"
		}

		return &ypb.PluginTraceResponse{
			ResponseType: "control_result",
			Success:      success,
			Message:      message,
		}

	case "stop_stream":
		// 客户端请求停止流
		manager.EnableExecutionTracing(false)
		return &ypb.PluginTraceResponse{
			ResponseType: "control_result",
			Success:      true,
			Message:      "流已停止",
		}

	case "set_tracing":
		manager.EnableExecutionTracing(req.EnableTracing)
		return &ypb.PluginTraceResponse{
			ResponseType: "control_result",
			Success:      true,
		}

	default:
		return &ypb.PluginTraceResponse{
			ResponseType: "control_result",
			Success:      false,
			Message:      "未知的控制模式: " + req.ControlMode,
		}
	}
}

// checkLongRunningTraces 检查长时间运行的trace并推送给前端
func (s *Server) checkLongRunningTraces(manager *yak.YakToCallerManager, traceUpdateChan chan *yak.PluginExecutionTrace, pushedTraces *map[string]bool, pushedTracesMutex *sync.RWMutex) {
	pushedTracesMutex.Lock()
	defer pushedTracesMutex.Unlock()

	// 获取所有Running状态的trace
	allTraces := manager.GetRunningExecutionTraces()

	for _, trace := range allTraces {
		// 只检查running状态的trace
		if trace.Status != yak.PluginStatusRunning {
			continue
		}

		// 检查运行时间是否超过阈值，使用管理器中的可配置阈值
		if !trace.StartTime.IsZero() {
			elapsed := time.Since(trace.StartTime)
			threshold := manager.GetLongRunningThreshold()
			if elapsed > time.Duration(threshold)*time.Second {
				// 标记为已推送
				(*pushedTraces)[trace.TraceID] = true

				// 推送到通道
				select {
				case traceUpdateChan <- trace:
					log.Debugf("检测到长时间运行的trace: %s, 运行时间: %v, 阈值: %ds", trace.TraceID, elapsed, threshold)
				default:
					log.Warn("trace update channel is full, dropping long running trace update")
				}
			}
		}
	}
}

// sendTraceBatch 批量发送trace更新
// 前端处理逻辑说明：
// - running状态：显示在trace列表中，可提供取消按钮
// - cancelled状态：更新对应trace为已取消状态
// - failed状态：更新对应trace为失败状态，显示错误信息
// - completed状态：从trace列表中移除该trace（因为正常完成不需要用户关注）
func (s *Server) sendTraceBatch(stream ypb.Yak_PluginTraceServer, traces []*yak.PluginExecutionTrace) {
	if len(traces) == 0 {
		return
	}

	pbTraces := make([]*ypb.PluginExecutionTrace, 0, len(traces))
	for _, trace := range traces {
		argsBytes, _ := json.Marshal(trace.Args)
		var durationMS int64
		if trace.EndTime.IsZero() { // trace still running
			durationMS = time.Now().Sub(trace.StartTime).Milliseconds()
		} else {
			durationMS = trace.EndTime.Sub(trace.StartTime).Milliseconds()
		}
		pbTrace := &ypb.PluginExecutionTrace{
			TraceID:       trace.TraceID,
			PluginID:      trace.PluginID,
			HookName:      trace.HookName,
			Status:        string(trace.Status),
			StartTime:     trace.StartTime.Unix(),
			EndTime:       trace.EndTime.Unix(),
			ExecutionArgs: argsBytes,
			ErrorMessage:  trace.Error,
			DurationMs:    durationMS,
			RuntimeId:     "", // 暂时为空
		}
		pbTraces = append(pbTraces, pbTrace)
	}

	resp := &ypb.PluginTraceResponse{
		ResponseType: "trace_update",
		Traces:       pbTraces,
		Success:      true,
	}

	if err := stream.Send(resp); err != nil {
		log.Warnf("发送trace批量更新失败: %v", err)
	}
}

// getPluginTraceStats 获取插件跟踪统计信息
func (s *Server) getPluginTraceStats(manager *yak.YakToCallerManager) *ypb.PluginTraceStats {
	traces := manager.GetAllExecutionTraces()

	stats := &ypb.PluginTraceStats{
		TotalTraces: int64(len(traces)),
	}

	for _, trace := range traces {
		switch trace.Status {
		case yak.PluginStatusRunning:
			stats.RunningTraces++
		case yak.PluginStatusCompleted:
			stats.CompletedTraces++
		case yak.PluginStatusFailed:
			stats.FailedTraces++
		case yak.PluginStatusCancelled:
			stats.CancelledTraces++
		}
	}

	return stats
}
