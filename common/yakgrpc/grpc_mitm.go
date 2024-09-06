package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/netx"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type hijackStatusCode int8

var (
	waitHijack   hijackStatusCode = -1
	hijack       hijackStatusCode = 0
	finishHijack hijackStatusCode = 1
	autoFoward   hijackStatusCode = 2
)

var enabledHooks = yak.MITMAndPortScanHooks

var mitmSaveToDBLock = new(sync.Mutex)

func feedbackFactory(db *gorm.DB, caller func(result *ypb.ExecResult) error, saveToDb bool, yakScriptName string) func(i interface{}) {
	return func(i interface{}) {
		if caller == nil {
			return
		}

		t, msg := yaklib.MarshalYakitOutput(i)
		if t == "" {
			return
		}
		ylog := &yaklib.YakitLog{
			Level:     t,
			Data:      msg,
			Timestamp: time.Now().Unix(),
		}
		raw, err := yaklib.YakitMessageGenerator(ylog)
		if err != nil {
			return
		}

		result := &ypb.ExecResult{
			IsMessage: true,
			Message:   raw,
		}
		if saveToDb {
			mitmSaveToDBLock.Lock()
			yakit.SaveExecResult(db, yakScriptName, result)
			mitmSaveToDBLock.Unlock()
		}

		caller(result)
		if err != nil {
			return
		}
		return
	}
}

var constClujore = func(i interface{}) func() interface{} {
	return func() interface{} {
		return i
	}
}

const (
	MITMReplacerKeyRecords = "R1oHf8xca6CobwVg2_MITMReplacerKeyRecords"
	MITMFilterKeyRecords   = "uWokegBnCQdnxezJtMVo_MITMFilterKeyRecords"
)

func (s *Server) MITM(stream ypb.Yak_MITMServer) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("mitm panic... %v", err)
		}
	}()

	var mServer *crep.MITMServer

	feedbacker := yak.YakitCallerIf(func(result *ypb.ExecResult) error {
		return stream.Send(&ypb.MITMResponse{Message: result, HaveMessage: true})
	})

	feedbackToUser := feedbackFactory(s.GetProjectDatabase(), feedbacker, false, "")
	send := func(rsp *ypb.MITMResponse) (sendError error) {
		defer func() {
			if err := recover(); err != nil {
				rspRaw, _ := json.Marshal(rsp)
				spew.Dump(rspRaw)
				sendError = utils.Errorf("send error: %s", err)
			}
		}()

		sendError = stream.Send(rsp)
		return
	}
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
	hostMapping := make(map[string]string)
	getDownstreamProxy := func(request *ypb.MITMRequest) (string, error) {
		downstreamProxy := strings.TrimSpace(request.GetDownstreamProxy())
		// 容错处理一下代理
		downstreamProxy = strings.Trim(downstreamProxy, `":`)
		if downstreamProxy == "0" {
			downstreamProxy = ""
		}
		if downstreamProxy != "" {
			feedbackToUser(fmt.Sprintf("启用下游代理为 / downstream proxy:[%v]", downstreamProxy))
			proxyUrl, err := url.Parse(downstreamProxy)
			if err != nil {
				feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, err))
				return "", utils.Errorf("cannot use proxy[%v]", err)
			}
			_, port, err := utils.ParseStringToHostPort(proxyUrl.Host)
			if err != nil {
				feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "parse host to host:port failed "+err.Error()))
				return "", utils.Errorf("parse proxy host failed: %s", proxyUrl.Host)
			}
			if port <= 0 {
				feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "缺乏端口（Miss Port）"))
				return "", utils.Errorf("proxy miss port. [%v]", proxyUrl.Host)
			}
			conn, err := netx.ProxyCheck(downstreamProxy, 5*time.Second) // 代理检查只做log记录，不在阻止MITM启动
			if err != nil {
				errInfo := "代理不通（Proxy Cannot be connected）"
				if errors.Is(err, netx.ErrorProxyAuthFailed) {
					errInfo = "认证失败（Proxy Auth Fail）"
				}
				feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, errInfo))
			}
			if conn != nil {
				conn.Close()
			}
		}
		return downstreamProxy, nil
	}
	var (
		host                  string = "127.0.0.1"
		port                  int    = 8089
		enableGMTLS                  = firstReq.GetEnableGMTLS()
		preferGMTLS                  = firstReq.GetPreferGMTLS()
		onlyGMTLS                    = firstReq.GetOnlyEnableGMTLS()
		proxyUsername                = firstReq.GetProxyUsername()
		proxyPassword                = firstReq.GetProxyPassword()
		dnsServers                   = firstReq.GetDnsServers()
		forceDisableKeepAlive        = firstReq.GetForceDisableKeepAlive()
		disableCACertPage            = firstReq.GetDisableCACertPage()
		randomJA3                    = firstReq.GetRandomJA3()
	)
	downstreamProxy, err := getDownstreamProxy(firstReq)
	if err != nil {
		return err
	}
	for _, pair := range firstReq.Hosts {
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

	if firstReq.GetHost() != "" {
		host = firstReq.GetHost()
	}
	if firstReq.GetPort() > 0 {
		port = int(firstReq.GetPort())
	}

	addr := utils.HostPort(host, port)
	log.Infof("start to listening mitm for %v", addr)

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	log.Infof("start to create mitm server instance for %v", addr)

	// 创建一个劫持流用来控制流程
	hijackingStream := make(chan int64, 1)
	defer func() {
		close(hijackingStream)
	}()

	feedbackToUser("初始化劫持流... / initializing hijacking stream")

	// 设置过滤器
	// 10M - 10 * 1000 * 1000
	packetLimit := 8 * 10 * 1000 * 1000 // 80M

	/*
		设置过滤器
	*/
	filterManager := GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())

	/*
		设置内容替换模块，通过正则驱动
	*/
	replacer := NewMITMReplacer(func() []*ypb.MITMContentReplacer {
		result := yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
		if result != "" {
			var rules []*ypb.MITMContentReplacer
			_ = json.Unmarshal([]byte(result), &rules)
			if len(rules) > 0 {
				return rules
			}
		}
		return nil
	})
	replacer.AutoSaveCallback(func(items ...*MITMReplaceRule) {
		if len(items) <= 0 {
			yakit.SetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords, "[]")
			return
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return
		}
		yakit.SetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords, string(raw))
	})

	recoverSend := func() {
		send(&ypb.MITMResponse{
			JustFilter:          true,
			IncludeHostname:     filterManager.IncludeHostnames,
			ExcludeHostname:     filterManager.ExcludeHostnames,
			ExcludeSuffix:       filterManager.ExcludeSuffix,
			IncludeSuffix:       filterManager.IncludeSuffix,
			ExcludeMethod:       filterManager.ExcludeMethods,
			ExcludeContentTypes: filterManager.ExcludeMIME,
			ExcludeUri:          filterManager.ExcludeUri,
			IncludeUri:          filterManager.IncludeUri,
			JustContentReplacer: true,
			Replacers:           replacer.GetRules(),
		})
	}

	feedbackToUser("初始化过滤器... / initializing filters")

	// 开始准备劫持
	// var hijackedResponseByRequestPTRMap = new(sync.Map)

	// callers := yak.NewYakToCallerManager()
	mitmPluginCaller, err := yak.NewMixPluginCaller()
	if err != nil {
		return utils.Errorf("create mitm plugin manager failed: %s", err)
	}
	mitmPluginCaller.SetFeedback(feedbacker)
	mitmPluginCaller.SetDividedContext(true)
	mitmPluginCaller.SetConcurrent(20)
	mitmPluginCaller.SetLoadPluginTimeout(10)
	mitmPluginCaller.SetCallPluginTimeout(60)
	if downstreamProxy != "" {
		mitmPluginCaller.SetProxy(downstreamProxy)
	}

	// int beforeRequest afterRequest
	var beforeRequest func(https bool, originReq []byte, req []byte) []byte = nil
	var afterRequest func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte = nil

	cacheThrottle := utils.NewDebounce(1)

	clearPluginHTTPFlowCache := func() {
		if mitmPluginCaller != nil {
			mitmPluginCaller.ResetFilter()
		}

		cacheThrottle(func() {
			stream.Send(&ypb.MITMResponse{
				HaveNotification:    true,
				NotificationContent: []byte("MITM 插件去重缓存已重置"),
			})
		})
	}

	controller := &hijackTaskController{
		taskStatusMap:  make(map[string]*taskStatus),
		canDequeueCond: sync.NewCond(&sync.Mutex{}),
		queueMux:       &sync.Mutex{},
		statusMapMux:   &sync.Mutex{},
		currentTask:    "",
	}

	waitNewHijackTask := func() { // 等待队列有可劫持任务
		controller.canDequeueCond.L.Lock()
		defer controller.canDequeueCond.L.Unlock()
		for !controller.canDequeue.IsSet() {
			controller.canDequeueCond.Wait()
		}
	}

	waitHijackFinish := func(currentStatus *taskStatus) {
		currentStatus.statusChangeCond.L.Lock()
		defer currentStatus.statusChangeCond.L.Unlock()
		for currentStatus.status < 1 { // 完成或者放行
			currentStatus.statusChangeCond.Wait()
		}
	}
	go func() {
		for {
			waitNewHijackTask()
			controller.currentTask = controller.nextTask()
			if controller.currentTask == "" {
				continue
			}
			currentStatus := controller.getStatus(controller.currentTask)
			currentStatus.setStatus(hijack)
			waitHijackFinish(currentStatus)
		}
	}()

	autoForward := utils.NewBool(true)
	autoForwardCh := make(chan struct{}, 1)

	filterWebSocket := utils.NewBool(firstReq.GetFilterWebsocket())

	go func() {
		defer close(autoForwardCh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-autoForwardCh:
			}
			controller.clear()
		}
	}()

	// 消息循环
	messageChan := make(chan *ypb.MITMRequest, 10000)
	go func() {
		defer close(messageChan)

		for {
			reqInstance, err := stream.Recv()
			if err != nil {
				log.Errorf("stream recv error: %v", err)
				return
			}

			if reqInstance.GetSetResetFilter() {
				filterManager.Recover()
				send(&ypb.MITMResponse{
					JustFilter:          true,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					IncludeUri:          filterManager.IncludeUri,
					ExcludeUri:          filterManager.ExcludeUri,
				})
				clearPluginHTTPFlowCache()
				continue
			}

			if reqInstance.SetContentReplacers {
				log.Infof("recv mitm content-replacers[%v]", len(reqInstance.Replacers))
				if len(reqInstance.Replacers) > 0 {
					replacer.SetRules(reqInstance.Replacers...)
				} else {
					log.Infof("remove all content-replacer")
					replacer.SetRules()
				}
				recoverSend()
				clearPluginHTTPFlowCache()
				continue
			}

			// 自动加载所有 MITM 插件（基础插件）
			if reqInstance.SetPluginMode {
				clearPluginHTTPFlowCache()
				if len(reqInstance.GetInitPluginNames()) > 0 {
					var plugins []string
					if len(reqInstance.GetInitPluginNames()) > 200 && false {
						plugins = reqInstance.GetInitPluginNames()[:200]
						stream.Send(&ypb.MITMResponse{HaveNotification: true, NotificationContent: []byte(
							"批量加载插件受限，最多一次性加载200个插件",
						)})
					} else {
						plugins = reqInstance.GetInitPluginNames()
					}
					var failedPlugins []string // 失败插件
					var loadedPlugins []string
					stream.Send(&ypb.MITMResponse{HaveLoadingSetter: true, LoadingFlag: true})
					swg := utils.NewSizedWaitGroup(50)
					wg := &sync.WaitGroup{}
					successScriptNameChan := make(chan string)
					failedScriptNameChan := make(chan string)
					wg.Add(2)
					go func() {
						defer wg.Done()
						for i := range successScriptNameChan {
							loadedPlugins = append(loadedPlugins, i)
						}
					}()
					go func() {
						defer wg.Done()
						for i := range failedScriptNameChan {
							failedPlugins = append(failedPlugins, i)
						}
					}()
					startTime := time.Now()
					for _, script := range yakit.QueryYakScriptByNames(s.GetProfileDatabase(), plugins...) {
						swg.Add()
						script := script
						go func() {
							defer swg.Done()
							err := mitmPluginCaller.LoadPluginEx(
								ctx,
								script, reqInstance.GetYakScriptParams()...,
							)
							if err != nil {
								failedScriptNameChan <- script.ScriptName
								log.Errorf("load %v failed: %s", script.ScriptName, err)
							} else {
								successScriptNameChan <- script.ScriptName
							}
						}()
					}
					swg.Wait()
					close(successScriptNameChan)
					close(failedScriptNameChan)
					wg.Wait()
					duration := time.Now().Sub(startTime).Seconds()
					stream.Send(&ypb.MITMResponse{HaveLoadingSetter: true, LoadingFlag: false})
					stream.Send(&ypb.MITMResponse{HaveNotification: true, NotificationContent: []byte(fmt.Sprintf(
						"初始化加载插件完成，加载成功【%v】个，失败【%v】个, 共耗时 %f 秒。", len(loadedPlugins), len(failedPlugins), duration,
					))})
				}
				clearPluginHTTPFlowCache()
				continue
			}

			// 清除 MITM 缓存
			if reqInstance.SetClearMITMPluginContext {
				clearPluginHTTPFlowCache()
				continue
			}

			// 清除 hook (会执行 clear 来清除垃圾)
			if reqInstance.RemoveHook {
				clearPluginHTTPFlowCache()
				mitmPluginCaller.GetNativeCaller().Remove(reqInstance.GetRemoveHookParams())
				_ = stream.Send(&ypb.MITMResponse{
					GetCurrentHook: true,
					Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
				})
				continue
			}

			// 设置自动转发
			if reqInstance.GetSetAutoForward() {
				clearPluginHTTPFlowCache()
				beforeAuto := autoForward.IsSet() // 存当前状态
				log.Debugf("mitm-auto-forward: %v", reqInstance.GetAutoForwardValue())
				autoForward.SetTo(reqInstance.GetAutoForwardValue())
				if !beforeAuto && autoForward.IsSet() { // 当 f -> t 时发送信号
					autoForwardCh <- struct{}{}
				}
			}

			// 设置中间人插件
			if reqInstance.SetYakScript {
				clearPluginHTTPFlowCache()
				script, _ := yakit.GetYakScript(s.GetProfileDatabase(), reqInstance.GetYakScriptID())
				if script != nil && (script.Type == "mitm" || script.Type == "port-scan") {
					log.Infof("start to load yakScript[%v]: %v 's capabilities", script.ID, script.ScriptName)
					// appendCallers(script.Content, script.ScriptName, reqInstance.YakScriptParams)
					ctx := stream.Context()
					err = mitmPluginCaller.LoadPluginEx(ctx, script, reqInstance.GetYakScriptParams()...)
					if err != nil {
						//_ = stream.Send(&ypb.MITMResponse{
						//	HaveNotification:    true,
						//	NotificationContent: []byte(fmt.Sprintf("加载失败[%v]：%v", script.ScriptName, err)),
						//})

						if len(script.GetParams()) > 0 {
							yakit.BroadcastData(yakit.ServerPushType_Error, fmt.Sprintf("加载失败[%v]：%v", script.ScriptName, err))
						}
						log.Error(err)
					}
					_ = stream.Send(&ypb.MITMResponse{
						GetCurrentHook: true,
						Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
					})
					continue
				}

				if script == nil && reqInstance.GetYakScriptContent() != "" {
					hotPatchScript := reqInstance.GetYakScriptContent()
					beforeRequest, afterRequest, _ = yak.MutateHookCaller(stream.Context(), hotPatchScript, feedbacker, reqInstance.GetYakScriptParams()...)

					log.Info("start to load yakScriptContent content")
					err := mitmPluginCaller.LoadHotPatch(stream.Context(), reqInstance.GetYakScriptParams(), hotPatchScript)
					_ = stream.Send(&ypb.MITMResponse{
						GetCurrentHook: true,
						Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
					})

					if err != nil {
						if strings.Contains(err.Error(), "YakVM Panic:") {
							splitErr := strings.SplitN(err.Error(), "YakVM Panic:", 2)
							err = utils.Error(splitErr[1])
						}
						yakit.BroadcastData(yakit.ServerPushType_Error, fmt.Sprintf("mitm load hotpatch script error:%v", err))
					}

					continue
				}
				continue
			}

			// 获取当前已经启用的插件
			if reqInstance.GetCurrentHook {
				_ = stream.Send(&ypb.MITMResponse{
					GetCurrentHook: true,
					Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
				})
				continue
			}

			// 更新过滤器
			if reqInstance.UpdateFilter {
				clearPluginHTTPFlowCache()
				filterManager.IncludeSuffix = reqInstance.IncludeSuffix
				filterManager.ExcludeSuffix = reqInstance.ExcludeSuffix
				filterManager.IncludeHostnames = reqInstance.IncludeHostname
				filterManager.ExcludeHostnames = reqInstance.ExcludeHostname
				filterManager.ExcludeMethods = reqInstance.ExcludeMethod
				filterManager.ExcludeMIME = reqInstance.ExcludeContentTypes
				filterManager.ExcludeUri = reqInstance.ExcludeUri
				filterManager.IncludeUri = reqInstance.IncludeUri
				filterManager.Save()
				send(&ypb.MITMResponse{
					JustFilter:          true,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeUri:          filterManager.ExcludeUri,
					IncludeUri:          filterManager.IncludeUri,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
				})
				continue
			}

			// 是否过滤ws
			if reqInstance.GetUpdateFilterWebsocket() {
				filterWebSocket.SetTo(reqInstance.GetFilterWebsocket())
				continue
			}

			// 运行时更新代理
			if reqInstance.GetSetDownstreamProxy() {
				downstreamProxy, err := getDownstreamProxy(reqInstance)
				if err == nil && mServer != nil {
					err = mServer.Configure(crep.MITM_SetDownstreamProxy(downstreamProxy))
					if err != nil {
						feedbackToUser(fmt.Sprintf("设置下游代理失败 / set downstream proxy failed: %v", err))
						log.Errorf("set downstream proxy failed: %s", err)
					}
					mitmPluginCaller.SetProxy(downstreamProxy)
					feedbackToUser(fmt.Sprintf("设置下游代理成功 / set downstream proxy successful: %v", downstreamProxy))
				}
			}
			messageChan <- reqInstance
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

		// 保存Websocket Flow
		_, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		wshash := httpctx.GetWebsocketRequestHash(req)
		if wshash == "" {
			wshash = utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		}

		yakit.SaveWebsocketFlowEx(s.GetProjectDatabase(), true, wshash, requireWsFrameIndexByWSHash(wshash), raw[:], func(err error) {
			if err != nil {
				log.Warnf("save websocket flow(from server) failed: %s", err)
			}
		})

		if autoForward.IsSet() {
			// 自动转发的内容，按理说这儿应该接入内部规则
			return originRspRaw
		}

		if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return raw
		}
		//_, ok := hijackedResponseByRequestPTRMap.Load(wshash)
		//if !ok {
		// return raw
		//}
		taskID := utils.CalcSha1(fmt.Sprintf("ws:%p[%s]", req, time.Now().String())) // ws特殊处理 由于一对多的模式 所以不需要请求劫持响应一一对应 响应和请求同级
		controller.Register(taskID)
		if controller.waitHijack(taskID) == autoFoward {
			return raw
		}
		defer func() {
			controller.finishHijack(taskID)
		}()

		wsReq := getPlainRequestBytes(req)
		responseCounter := time.Now().UnixNano()
		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Payload:     raw,
			Url:         urlStr,
			ResponseId:  responseCounter,
			Request:     wsReq,
			RemoteAddr:  httpctx.GetRemoteAddr(req),
			IsWebsocket: true,
		}

		err = send(feedbackRspIns)
		if err != nil {
			log.Errorf("send response failed: %s", err)
			return raw
		}

		for {
			reqInstance, ok := <-messageChan
			if !ok {
				return raw
			}

			// 如果出现了问题，丢失上下文，可以通过 recover 来恢复
			if reqInstance.GetRecover() {
				log.Infof("retry recover mitm session")
				send(feedbackRspIns)
				send(&ypb.MITMResponse{
					JustFilter:          true,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					ExcludeUri:          filterManager.ExcludeUri,
					IncludeUri:          filterManager.IncludeUri,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
				})
				continue
			}

			if reqInstance.GetResponseId() < responseCounter {
				continue
			}

			if reqInstance.GetDrop() {
				return nil
			}

			if reqInstance.GetForward() {
				return originRspRaw
			}

			return reqInstance.GetResponse()
		}
	}

	handleHijackResponse := func(isHttps bool, req *http.Request, rspInstance *http.Response, rsp []byte, remoteAddr string) (hijackRsp []byte) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("hijack response error: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
			if afterRequest != nil {
				hijackRsp = afterRequest(isHttps, httpctx.GetBareRequestBytes(req), httpctx.GetRequestBytes(req), httpctx.GetBareResponseBytes(req), httpctx.GetResponseBytes(req))
				httpctx.SetResponseModified(req, "yaklang.hook(ex) afterRequest")
				httpctx.SetHijackedResponseBytes(req, hijackRsp)
			}
		}()
		originRspRaw := rsp[:]
		plainResponse := getPlainResponseBytes(req)
		if len(plainResponse) > 0 {
			httpctx.SetPlainResponseBytes(req, plainResponse)
			rsp = plainResponse
		}

		urlStr := httpctx.GetRequestURL(req)

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
		mitmPluginCaller.CallHijackResponseEx(isHttps, urlStr, func() interface{} {
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
			mitmPluginCaller.CallHijackResponse(isHttps, urlStr, func() interface{} {
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

		// 自动转发与否
		if autoForward.IsSet() { // 如果其请求是自动转发的，响应也不应该劫持
			httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_AutoFoward, true)
			/*
				自动过滤下，不是所有 response 都应该替换
				应该替换的条件是不匹配过滤器的内容
			*/

			// 处理响应规则
			if replacer.haveHijackingRules() {
				rules, rspHooked, dropped := replacer.hook(false, true, rsp)
				if dropped {
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
					log.Warn("response should be dropped(VIA replacer.hook)")
					return nil
				}
				httpctx.AppendMatchedRule(req, rules...)
				if handleResponseModified(rspHooked) {
					httpctx.SetResponseModified(req, "yakit.rule.hook")
					httpctx.SetHijackedResponseBytes(req, rspHooked)
				}
				return rspHooked
			}
			return rsp
		}

		// 非自动转发的情况下处理替换器
		rules, rsp1, shouldBeDropped := replacer.hook(false, true, rsp)
		if shouldBeDropped {
			log.Warn("response should be dropped(VIA replacer.hook)")
			httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
			return nil
		}
		if handleResponseModified(rsp1) {
			rsp = rsp1
		}
		httpctx.AppendMatchedRule(req, rules...)
		responseCounter := time.Now().UnixNano()

		ptr := fmt.Sprintf("%p", req)
		if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return rsp
		}

		rsp, _, err := lowhttp.FixHTTPResponse(rsp)
		if err != nil {
			log.Errorf("fix http response packet failed: %s", err)
			return originRspRaw
		}

		taskID := utils.CalcSha1(fmt.Sprintf("%p", req))
		if taskID != controller.currentTask { //  只劫持当前请求对应的响应，避免超时请求导致的错误响应劫持
			log.Debugf("not want resp, auto forward")
			return originRspRaw
		}

		defer func() {
			controller.finishHijack(taskID)
		}()

		var traceInfo *lowhttp.LowhttpTraceInfo
		if i, ok := httpctx.GetResponseTraceInfo(req).(*lowhttp.LowhttpTraceInfo); ok {
			traceInfo = i
		}

		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Response:    rsp,
			Request:     plainRequest,
			ResponseId:  responseCounter,
			RemoteAddr:  remoteAddr,
			TraceInfo:   model.ToLowhttpTraceInfoGRPCModel(traceInfo),
		}
		err = send(feedbackRspIns)
		if err != nil {
			log.Errorf("send response failed: %s", err)
			return rsp
		}

		httpctx.SetResponseViewedByUser(req)
		for {
			if autoForward.IsSet() {
				return rsp
			}
			reqInstance, ok := <-messageChan
			if !ok {
				return rsp
			}

			// 如果出现了问题，丢失上下文，可以通过 recover 来恢复
			if reqInstance.GetRecover() {
				log.Infof("retry recover mitm session")
				send(feedbackRspIns)
				send(&ypb.MITMResponse{
					JustFilter:          true,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					ExcludeUri:          filterManager.ExcludeUri,
					IncludeUri:          filterManager.IncludeUri,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
				})
				continue
			}

			if reqInstance.GetResponseId() < responseCounter {
				continue
			}

			if reqInstance.GetDrop() {
				return nil
			}

			if reqInstance.GetForward() {
				return originRspRaw
			}

			response := reqInstance.GetResponse()
			if handleResponseModified(response) {
				httpctx.SetResponseModified(req, "manual")
				httpctx.SetHijackedResponseBytes(req, response)
			}

			rspModified, _, err := lowhttp.FixHTTPResponse(response)
			if err != nil {
				log.Errorf("fix http response[req:%v] failed: %s", ptr, err.Error())
				return originRspRaw
			}

			if rspModified == nil {
				log.Error("BUG: http response is empty... use origin")
				return originRspRaw
			}
			return rspModified
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

		isTls, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		var extName string
		u, _ := url.Parse(urlStr)
		if ret := path.Ext(u.EscapedPath()); ret != "" {
			extName = ret
			if !strings.HasPrefix(extName, ".") {
				extName = "." + extName
			}
		}
		if !filterManager.IsPassed(req.Method, req.Host, urlStr, extName, isTls) {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return raw
		}
		wshash := httpctx.GetWebsocketRequestHash(req)
		if wshash == "" {
			wshash = utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		}

		originReqRaw := raw[:]
		finalResult = originReqRaw

		// 保存 Websocket Flow
		yakit.SaveWebsocketFlowEx(s.GetProjectDatabase(), false, wshash, requireWsFrameIndexByWSHash(wshash), raw[:], func(err error) {
			if err != nil {
				log.Warnf("save to websocket flow failed: %s", err)
			}
		})

		// MITM 自动转发
		if autoForward.IsSet() {
			return raw
		}

		var encode []string
		if utils.IsGzip(raw) {
			encode = append(encode, "gzip")
		}
		//if utils.IsProtobuf() {
		//	encode = append(encode, "protobuf")
		//}

		taskID := utils.CalcSha1(fmt.Sprintf("ws:%p[%s]", req, time.Now().String())) // ws特殊处理 由于一对多的模式 所以不需要请求劫持响应一一对应。
		controller.Register(taskID)
		if controller.waitHijack(taskID) == autoFoward { // 自动放行
			return raw
		}

		defer func() {
			controller.finishHijack(taskID)
		}()

		wsReq := getPlainRequestBytes(req)
		counter := time.Now().UnixNano()
		select {
		case hijackingStream <- counter:
		case <-ctx.Done():
			return raw
		}

		select {
		case <-stream.Context().Done():
			return raw
		case id, ok := <-hijackingStream:
			if !ok {
				return raw
			}

			for {
				feedbackOrigin := &ypb.MITMResponse{
					Request:             wsReq,
					Payload:             raw,
					IsHttps:             false,
					Url:                 urlStr,
					Id:                  id,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					ExcludeUri:          filterManager.ExcludeUri,
					IncludeUri:          filterManager.IncludeUri,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
					IsWebsocket:         true,
					RemoteAddr:          httpctx.GetRemoteAddr(req),
				}
				err = send(feedbackOrigin)
				if err != nil {
					log.Errorf("send ws to mitm client failed: %s", err)
					return raw
				}

			RECV:
				reqInstance, ok := <-messageChan
				if !ok {
					cancel()
					return raw
				}

				if reqInstance.GetRecover() {
					send(feedbackOrigin)
				}

				// 如果 ID 对不上，返回来的是旧的，已经不需要处理的 ID，则重新接受等待新的
				if reqInstance.GetId() < id {
					log.Warnf("MITM %v recv old hijacked request[%v]", addr, reqInstance.GetId())
					goto RECV
				}

				// 直接丢包
				if reqInstance.GetDrop() {
					return nil
				}

				// 原封不动转发
				if reqInstance.GetForward() {
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false)
					return originReqRaw
				}

				if reqInstance.GetHijackResponse() {
					// 设置需要劫持resp
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					log.Infof("the ws hash: %s's mitm ws response is waiting for hijack response", wshash)
					continue // 需要重新回到recv
				}
				if reqInstance.GetCancelhijackResponse() {
					// 设置不需要劫持resp
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false)
					log.Infof("the ws hash: %s's mitm ws response cancel hijack response", wshash)
					continue
				}

				// 把修改后的请求放回去
				requestModified := reqInstance.GetRequest()
				return requestModified
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
		originReqRaw := req[:]
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

		// 保证始终只有一个 Goroutine 在处理请求
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
		if !filterManager.IsPassed(method, hostname, urlStr, extName, isHttps) {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return req
		}

		defer func() {
			if beforeRequest != nil {
				newHijackReq := beforeRequest(isHttps, httpctx.GetBareRequestBytes(originReqIns), hijackReq)
				if handleRequestModified(newHijackReq) {
					hijackReq = newHijackReq
					setModifiedRequest("yaklang.hook beforeRequest", hijackReq)
				}
			}
		}()

		rules, req1, shouldBeDropped := replacer.hook(true, false, req, isHttps)
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

		mitmPluginCaller.CallHijackRequest(isHttps, urlStr,
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

		// MITM 手动劫持放行
		if autoForward.IsSet() {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_AutoFoward, true)
			return req
		}

		taskID := utils.CalcSha1(fmt.Sprintf("%p", originReqIns))
		controller.Register(taskID)                      // 加入队列
		if controller.waitHijack(taskID) == autoFoward { // 等待劫持
			return req
		}

		defer func() {
			if !httpctx.GetContextBoolInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) { // 如果不需要劫持请求则可以直接设置此次hijack已完成
				controller.finishHijack(taskID)
			}
		}()

		// 开始劫持
		counter := time.Now().UnixNano()
		select {
		case hijackingStream <- counter:
		case <-ctx.Done():
			return req
		}

		select {
		case <-stream.Context().Done():
			return req
		case id, ok := <-hijackingStream:
			// channel 可以保证线程安全
			if !ok {
				return req
			}
			httpctx.SetRequestViewedByUser(originReqIns)
			for {
				feedbackOrigin := &ypb.MITMResponse{
					Request:             req,
					Refresh:             false,
					IsHttps:             isHttps,
					Url:                 urlStr,
					Id:                  id,
					IncludeHostname:     filterManager.IncludeHostnames,
					ExcludeHostname:     filterManager.ExcludeHostnames,
					ExcludeSuffix:       filterManager.ExcludeSuffix,
					IncludeSuffix:       filterManager.IncludeSuffix,
					ExcludeMethod:       filterManager.ExcludeMethods,
					ExcludeContentTypes: filterManager.ExcludeMIME,
					ExcludeUri:          filterManager.ExcludeUri,
					IncludeUri:          filterManager.IncludeUri,
					JustContentReplacer: true,
					Replacers:           replacer.GetRules(),
					RemoteAddr:          httpctx.GetRemoteAddr(originReqIns),
				}

				isMultipartData := lowhttp.IsMultipartFormDataRequest(fixReq)
				if isMultipartData {
					feedbackOrigin.Request = lowhttp.ConvertHTTPRequestToFuzzTag(fixReq)
				}

				err = send(feedbackOrigin)
				if err != nil {
					log.Errorf("send to mitm client failed: %s", err)
					return req
				}

			RECV:
				reqInstance, ok := <-messageChan
				if !ok {
					cancel()
					return req
				}

				// 如果出现了问题，丢失上下文，可以通过 recover 来恢复
				if reqInstance.GetRecover() {
					log.Infof("retry recover mitm session")
					send(feedbackOrigin)
					continue
				}

				tags := reqInstance.GetTags()
				if len(tags) > 0 {
					httpctx.SetFlowTags(originReqIns, tags)
				}

				if reqInstance.GetHijackResponse() {
					// 设置需要劫持resp
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					hijackedPtr := fmt.Sprintf("%p", originReqIns)
					log.Infof("the ptr: %v's mitm request is waiting for hijack response", hijackedPtr)
					continue
				}
				if reqInstance.GetCancelhijackResponse() {
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false)
					hijackedPtr := fmt.Sprintf("%p", originReqIns)
					log.Infof("the ptr: %v's mitm request cancel hijack response", hijackedPtr)
					continue
				}
				// 如果 ID 对不上，返回来的是旧的，已经不需要处理的 ID，则重新接受等待新的
				if reqInstance.GetId() < id {
					log.Warnf("MITM %v recv old hijacked request[%v]", addr, reqInstance.GetId())
					goto RECV
				}

				// 直接丢包
				if reqInstance.GetDrop() {
					log.Infof("MITM %v recv drop hijacked request[%v]", addr, reqInstance.GetId())
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false) // 设置无需劫持resp
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
					// 保存到数据库
					log.Debugf("start to create httpflow from mitm[%v %v]", originReqIns.Method, truncate(originReqIns.URL.String()))
					startCreateFlow := time.Now()
					flow, err := yakit.CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps, originReqIns, "mitm", originReqIns.URL.String(), remoteAddr, yakit.CreateHTTPFlowWithRequestIns(fixReqIns))
					if err != nil {
						log.Errorf("save http flow[%v %v] from mitm failed: %s", originReqIns.Method, originReqIns.URL.String(), err)
						return nil
					}
					log.Debugf("yakit.CreateHTTPFlowFromHTTPWithBodySaved for %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
					// Hidden Index 用来标注 MITM 劫持的顺序
					flow.Hash = flow.CalcHash()
					flow.AddTagToFirst("[被丢弃]")
					flow.Purple()

					log.Debugf("mitmPluginCaller.HijackSaveHTTPFlow for %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
					startCreateFlow = time.Now()

					// HOOK 存储过程
					flow.Hash = flow.CalcHash()
					flow.StatusCode = 200 // 这里先设置成200
					flow.Response = ""
					// log.Infof("start to do sth with tag")
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

				// 原封不动转发
				if reqInstance.GetForward() {
					return originReqRaw
				}

				current := reqInstance.GetRequest()
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
		// 劫持过滤
		if isFiltered {
			return
		}
		go func() {
			mitmPluginCaller.MirrorHTTPFlow(isHttps, reqUrl, plainRequest, plainResponse, body, shouldBeHijacked)
		}()

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
		// 额外添加用户手动设置的标签
		tags := httpctx.GetFlowTags(req)
		if len(tags) > 0 {
			flow.AddTag(tags...)
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

		pluginFinishCh := make(chan struct{})
		mitmPluginCaller.HijackSaveHTTPFlowWithCallback(
			flow,
			func() {
				close(pluginFinishCh)
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
		colorOK := make(chan struct{})

		var extracted []*schema.ExtractedData
		// replacer hook color
		if replacer != nil {
			go func() {
				extracted = replacer.hookColor(plainRequest, plainResponse, req, flow)
				close(colorOK)
				for _, e := range extracted {
					err = yakit.CreateOrUpdateExtractedDataEx(-1, e)
					if err != nil {
						log.Errorf("save hookcolor error: %s", err)
					}
				}
			}()
		} else {
			close(colorOK)
		}

		var needUpdate bool
		timeoutCtx, timeCancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer timeCancel()
		select {
		case <-colorOK:
		case <-timeoutCtx.Done(): // wait for max 300ms
			needUpdate = true
		}

		select {
		case <-pluginFinishCh:
		case <-timeoutCtx.Done(): // wait for max 300ms
			needUpdate = true
		}

		if !isDroppedSaveFlow.IsSet() {
			err := yakit.InsertHTTPFlowEx(flow, false, func() {
				saveBarePacketHandler(flow.ID)
			})
			if err != nil {
				log.Errorf("create / save httpflow from mirror error: %s", err)
			} else if needUpdate {
				go func() {
					err := yakit.UpdateHTTPFlowTagsEx(flow)
					if err != nil {
						log.Errorf("update http flow tags error: %s", err)
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
	mServer, err = crep.NewMITMServer(
		crep.MITM_EnableMITMCACertPage(!disableCACertPage),
		crep.MITM_RandomJA3(randomJA3),
		crep.MITM_ProxyAuth(proxyUsername, proxyPassword),
		crep.MITM_SetHijackedMaxContentLength(packetLimit),
		crep.MITM_SetDownstreamProxy(downstreamProxy),
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
		crep.MITM_SetDNSServers(dnsServers...),
		crep.MITM_SetHostMapping(hostMapping),
		crep.MITM_SetHTTPForceClose(forceDisableKeepAlive),
		crep.MITM_SetMaxContentLength(firstReq.GetMaxContentLength()),
	)
	if err != nil {
		log.Error(err)
		return err
	}

	// 发送第一个设置状态
	recoverSend()
	// 发送第二个来设置 replacer
	recoverSend()

	log.Infof("start serve mitm server for %s", addr)
	// err = mServer.Run(ctx)
	err = mServer.ServeWithListenedCallback(ctx, utils.HostPort(host, port), func() {
		feedbackToUser("MITM 服务器已启动 / starting mitm server")
	})
	if err != nil {
		log.Errorf("close mitm server for %s, reason: %v", addr, err)
		return err
	}

	return nil
}

func (s *Server) DownloadMITMCert(ctx context.Context, _ *ypb.Empty) (*ypb.MITMCert, error) {
	crep.InitMITMCert()
	ca, _, err := crep.GetDefaultCaAndKey()
	if err != nil {
		return nil, utils.Errorf("fetch default ca/key failed: %s", err)
	}
	return &ypb.MITMCert{CaCerts: ca, LocalFile: crep.GetDefaultCaFilePath()}, nil
}

func (s *Server) ExportMITMReplacerRules(ctx context.Context, _ *ypb.Empty) (*ypb.ExportMITMReplacerRulesResponse, error) {
	result := yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
	if result != "" {
		var replacers []*ypb.MITMContentReplacer
		err := json.Unmarshal([]byte(result), &replacers)
		if err != nil {
			return nil, err
		}
		raw, err := json.MarshalIndent(replacers, "", "    ")
		if err != nil {
			return nil, err
		}
		return &ypb.ExportMITMReplacerRulesResponse{JsonRaw: raw}, nil
	}
	return nil, utils.Errorf("no existed key records")
}

func (s *Server) ImportMITMReplacerRules(ctx context.Context, req *ypb.ImportMITMReplacerRulesRequest) (*ypb.Empty, error) {
	replace := req.GetReplaceAll()

	var newRules []*ypb.MITMContentReplacer
	var newRule ypb.MITMContentReplacer
	_ = json.Unmarshal(req.GetJsonRaw(), &newRules)
	if len(newRules) < 0 {
		_ = json.Unmarshal(req.GetJsonRaw(), &newRule)
		if newRule.Rule == "" {
			return nil, utils.Error("cannot identify rule (json)")
		}
		newRules = append(newRules, &newRule)
	}

	if len(newRules) <= 0 {
		return nil, utils.Error("规则解析失败(没有新规则导入): no new rules found")
	}

	if replace {
		err := yakit.SetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords, string(req.GetJsonRaw()))
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}

	var existed []*ypb.MITMContentReplacer
	result := yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
	if result != "" {
		_ = json.Unmarshal([]byte(result), &existed)
	}

	funk.ForEach(newRules, func(r *ypb.MITMContentReplacer) {
		r.Index += int32(len(existed))
	})
	existed = append(existed, newRules...)
	raw, err := json.Marshal(existed)
	if err != nil {
		return nil, err
	}
	_ = yakit.SetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords, string(raw))
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) SetCurrentRules(c context.Context, req *ypb.MITMContentReplacers) (*ypb.Empty, error) {
	raw, err := json.Marshal(req.GetRules())
	if err != nil {
		return nil, err
	}
	err = yakit.SetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords, string(raw))
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetCurrentRules(c context.Context, req *ypb.Empty) (*ypb.MITMContentReplacers, error) {
	result := yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
	var rules []*ypb.MITMContentReplacer
	_ = json.Unmarshal([]byte(result), &rules)
	return &ypb.MITMContentReplacers{Rules: rules}, nil
}

func truncate(u string) string {
	if len(u) > 64 {
		return u[:64] + "..."
	}
	return u
}

func (s *Server) GenerateURL(ctx context.Context, req *ypb.GenerateURLRequest) (*ypb.GenerateURLResponse, error) {
	var userInfo *url.Userinfo
	var host string
	if req.GetUsername() != "" {
		if req.GetPassword() != "" {
			userInfo = url.UserPassword(req.GetUsername(), req.GetPassword())
		} else {
			userInfo = url.User(req.GetUsername())
		}
	}
	if req.GetScheme() == "http" && req.GetPort() == 80 {
		host = req.GetHost()
	} else if req.GetScheme() == "https" && req.GetPort() == 443 {
		host = req.GetHost()
	} else {
		host = fmt.Sprintf("%s:%d", req.GetHost(), req.GetPort())
	}
	u := url.URL{
		Scheme: req.GetScheme(),
		User:   userInfo,
		Host:   host,
	}

	return &ypb.GenerateURLResponse{
		URL: u.String(),
	}, nil
}

type hijackTaskController struct {
	taskQueue []string
	queueMux  *sync.Mutex

	taskStatusMap map[string]*taskStatus
	statusMapMux  *sync.Mutex

	currentTask string

	canDequeue     utils.AtomicBool
	canDequeueCond *sync.Cond
}

type taskStatus struct {
	status           hijackStatusCode
	statusChangeCond *sync.Cond
}

func (h *hijackTaskController) Register(taskID string) {
	h.enqueue(taskID)
	if h.queueSize() == 1 {
		h.canDequeueCond.L.Lock() // 第一个元素进入队列的时候，需要唤醒等待的线程
		h.canDequeue.Set()
		h.canDequeueCond.Broadcast()
		h.canDequeueCond.L.Unlock()
	}
}

func (h *hijackTaskController) waitHijack(taskID string) hijackStatusCode { // mitm 任务等待劫持 任务调用
	thisStatus := h.getStatus(taskID)
	if thisStatus == nil { // 如果没有查到状态则自动放行
		return autoFoward
	}
	thisStatus.statusChangeCond.L.Lock()
	defer thisStatus.statusChangeCond.L.Unlock()
	for thisStatus.status == waitHijack { // 状态不为等待即可放行
		thisStatus.statusChangeCond.Wait()
	}
	return thisStatus.status
}

func (h *hijackTaskController) finishHijack(taskID string) { // mitm 任务结束 任务调用
	thisStatus := h.getStatus(taskID)
	if thisStatus == nil { // 如果没有查到状态则自动放行
		return
	}
	thisStatus.setStatus(finishHijack)
}

func (h *hijackTaskController) nextTask() string {
	return h.dequeue()
}

func (h *hijackTaskController) clear() {
	h.queueMux.Lock()
	h.statusMapMux.Lock()
	defer func() {
		h.queueMux.Unlock()
		h.statusMapMux.Unlock()
	}()

	for _, t := range h.taskStatusMap {
		t.setStatus(autoFoward)
	}
	h.currentTask = ""
	h.taskQueue = h.taskQueue[:0]
	h.taskStatusMap = make(map[string]*taskStatus)
	h.canDequeue.UnSet()
}

func (h *hijackTaskController) enqueue(s string) {
	h.queueMux.Lock()
	defer h.queueMux.Unlock()
	h.taskQueue = append(h.taskQueue, s)
	h.setStatus(s, &taskStatus{
		status:           waitHijack,
		statusChangeCond: sync.NewCond(&sync.Mutex{}),
	})
}

func (h *hijackTaskController) dequeue() string {
	h.queueMux.Lock()
	defer h.queueMux.Unlock()
	if len(h.taskQueue) == 0 {
		return ""
	}
	item := h.taskQueue[0]
	h.taskQueue = h.taskQueue[1:]
	if len(h.taskQueue) == 0 { // 队列空 则停止下次请求
		h.canDequeue.UnSet()
	}
	return item
}

func (h *hijackTaskController) queueSize() int {
	return len(h.taskQueue)
}

func (h *hijackTaskController) getStatus(s string) *taskStatus {
	h.statusMapMux.Lock()
	defer h.statusMapMux.Unlock()
	return h.taskStatusMap[s]
}

func (h *hijackTaskController) setStatus(r string, s *taskStatus) {
	h.statusMapMux.Lock()
	defer h.statusMapMux.Unlock()
	h.taskStatusMap[r] = s
}

func (t *taskStatus) setStatus(s hijackStatusCode) {
	t.statusChangeCond.L.Lock()
	t.status = s
	t.statusChangeCond.Broadcast()
	t.statusChangeCond.L.Unlock()
}
