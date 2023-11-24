package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

func _exactChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	if includes == nil {
		if utils.StringSliceContain(excludes, target) {
			return false
		}
		return true
	} else {
		if utils.StringSliceContain(excludes, target) {
			return false
		}
		if utils.StringSliceContain(includes, target) {
			return true
		}
		return false
	}
}

func _checker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	if includes == nil {
		return !utils.StringGlobArrayContains(excludes, target)
	}

	if utils.StringGlobArrayContains(excludes, target) {
		return false
	}
	if utils.StringGlobArrayContains(includes, target) {
		return true
	}
	return false
}

func fixSuffix(suf string) string {
	if strings.HasPrefix(suf, ".") {
		return suf
	} else {
		return "." + suf
	}
}

func _suffixChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	for _, suffix := range excludes {
		if strings.HasSuffix(target, fixSuffix(suffix)) {
			return false
		}
	}

	if includes == nil {
		return true
	}

	for _, suffix := range includes {
		if strings.HasSuffix(target, fixSuffix(suffix)) {
			return true
		}
	}
	return false
}
func mimeCheckGlobRule(rule string, target string) bool {
	if strings.Contains(rule, "/") && strings.Contains(target, "/") { // 如果两个都包含/，则进行分割匹配
		ruleType := strings.SplitN(rule, "/", 2)
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(ruleType[i], "*") {
				rule, err := glob.Compile(ruleType[i])
				if err != nil || !rule.Match(targetType[i]) {
					return false //任意部分匹配失败则 false,包括glob编译失败
				}
			} else {
				if ruleType[i] != targetType[i] {
					return false //任意部分匹配失败则 false
				}
			}
		}
		return true // 全部通过 true
	}

	if !strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 如果都不包含 /
		if strings.Contains(rule, "*") { // 尝试glob 匹配
			rule, err := glob.Compile(rule)
			if err == nil && rule.Match(target) {
				return true
			}
		} else { // 直接 contains
			if utils.IContains(target, rule) {
				return true
			}
		}
		return false
	}

	if strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 仅rule不包含 /
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(rule, "*") {
				rule, err := glob.Compile(rule)
				if err != nil {
					continue
				}
				if rule.Match(targetType[i]) {
					return true //任意部分匹配成功 则true
				}
			} else {
				if rule == targetType[i] {
					return true //任意部分匹配成功 则true
				}
			}
		}
		return false // 全部失败 则false
	}

	return false // 仅 rule 有 / 则直接返回 false
}

func _mimeChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	if includes == nil {
		for _, rule := range excludes {
			if mimeCheckGlobRule(rule, target) {
				return false //如果命中 excludes 则 false 即过滤
			}
		}
		return true
	}

	for _, rule := range excludes {
		if mimeCheckGlobRule(rule, target) {
			return false //如果命中 excludes 则 false 即过滤
		}
	}

	for _, rule := range includes {
		if mimeCheckGlobRule(rule, target) {
			return true //如果命中 includes 则 true 即放行
		}
	}
	return false
}

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

const MITMReplacerKeyRecords = "R1oHf8xca6CobwVg2_MITMReplacerKeyRecords"
const MITMFilterKeyRecords = "uWokegBnCQdnxezJtMVo_MITMFilterKeyRecords"

func (s *Server) MITM(stream ypb.Yak_MITMServer) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("mitm panic... %v", err)
		}
	}()

	var (
		mitmLock = new(sync.Mutex)
	)

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

	firstReq, err := stream.Recv()
	if err != nil {
		return utils.Errorf("recv first req failed: %s", err)
	}
	feedbackToUser("接收到 MITM 启动参数 / receive mitm config request")
	hostMapping := make(map[string]string)
	var (
		host            string = "127.0.0.1"
		port            int    = 8089
		downstreamProxy        = strings.TrimSpace(firstReq.GetDownstreamProxy())
		enableGMTLS            = firstReq.GetEnableGMTLS()
		preferGMTLS            = firstReq.GetPreferGMTLS()
		onlyGMTLS              = firstReq.GetOnlyEnableGMTLS()
		proxyUsername          = firstReq.GetProxyUsername()
		proxyPassword          = firstReq.GetProxyPassword()
		dnsServers             = firstReq.GetDnsServers()
	)
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
			return utils.Errorf("cannot use proxy[%v]", err)
		}
		_, port, err := utils.ParseStringToHostPort(proxyUrl.Host)
		if err != nil {
			feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "parse host to host:port failed "+err.Error()))
			return utils.Errorf("parse proxy host failed: %s", proxyUrl.Host)
		}
		if port <= 0 {
			feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "缺乏端口（Miss Port）"))
			return utils.Errorf("proxy miss port. [%v]", proxyUrl.Host)
		}
		conn, err := netx.ProxyCheck(downstreamProxy, 5*time.Second) //代理检查只做log记录，不在阻止MITM启动
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
	var packetLimit = 8 * 10 * 1000 * 1000 // 80M

	/*
		设置过滤器
	*/
	var filterManager = NewMITMFilterManager(s.GetProfileDatabase())

	/*
		设置内容替换模块，通过正则驱动
	*/
	var replacer = NewMITMReplacer(func() []*ypb.MITMContentReplacer {
		var result = yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
		if result != "" {
			var rules []*ypb.MITMContentReplacer
			_ = json.Unmarshal([]byte(result), &rules)
			if len(rules) > 0 {
				return rules
			}
		}
		return nil
	})
	replacer.AutoSaveCallback(func(items ...*ypb.MITMContentReplacer) {
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

	var recoverSend = func() {
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
	//var hijackedResponseByRequestPTRMap = new(sync.Map)

	//callers := yak.NewYakToCallerManager()
	mitmPluginCaller, err := yak.NewMixPluginCaller()
	if err != nil {
		return utils.Errorf("create mitm plugin manager failed: %s", err)
	}
	mitmPluginCaller.SetFeedback(feedbacker)
	mitmPluginCaller.SetDividedContext(true)
	mitmPluginCaller.SetConcurrent(20)
	mitmPluginCaller.SetLoadPluginTimeout(10)
	if downstreamProxy != "" {
		mitmPluginCaller.SetProxy(downstreamProxy)
	}

	clearPluginHTTPFlowCache := func() {
		if mitmPluginCaller != nil {
			mitmPluginCaller.ResetFilter()
		}
		stream.Send(&ypb.MITMResponse{
			HaveNotification:    true,
			NotificationContent: []byte("MITM 插件去重缓存已重置"),
		})
	}

	var autoForward = utils.NewBool(true)

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
							err := mitmPluginCaller.LoadPluginByName(
								ctx,
								script.ScriptName, nil, script.Content,
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
				log.Infof("mitm-auto-forward: %v", reqInstance.GetAutoForwardValue())
				autoForward.SetTo(reqInstance.GetAutoForwardValue())
			}

			// 设置中间人插件
			if reqInstance.SetYakScript {
				clearPluginHTTPFlowCache()
				script, _ := yakit.GetYakScript(s.GetProfileDatabase(), reqInstance.GetYakScriptID())
				if script != nil && (script.Type == "mitm" || script.Type == "port-scan") {
					log.Infof("start to load yakScript[%v]: %v 's capabilities", script.ID, script.ScriptName)
					//appendCallers(script.Content, script.ScriptName, reqInstance.YakScriptParams)
					ctx := stream.Context()
					err = mitmPluginCaller.LoadPluginByName(ctx, script.ScriptName, nil, script.Content)
					if err != nil {
						_ = stream.Send(&ypb.MITMResponse{
							HaveNotification:    true,
							NotificationContent: []byte(fmt.Sprintf("加载失败[%v]：%v", script.ScriptName, err)),
						})
						log.Error(err)
					}
					_ = stream.Send(&ypb.MITMResponse{
						GetCurrentHook: true,
						Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
					})
					continue
				}

				if script == nil && reqInstance.GetYakScriptContent() != "" {
					log.Info("start to load yakScriptContent content")
					err = mitmPluginCaller.LoadHotPatch(stream.Context(), reqInstance.GetYakScriptContent())
					_ = stream.Send(&ypb.MITMResponse{
						GetCurrentHook: true,
						Hooks:          mitmPluginCaller.GetNativeCaller().GetCurrentHooksGRPCModel(),
					})
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
			//defer func() {
			//	recover()
			//}()
			messageChan <- reqInstance
		}
	}()

	feedbackToUser("创建 MITM 服务器 / creating mitm server")

	/*
		设置数据包计数器
	*/
	var offset = time.Now().UnixNano()
	var count = 0
	var packetCountLock = new(sync.Mutex)
	addCounter := func() {
		packetCountLock.Lock()
		defer packetCountLock.Unlock()
		count++
	}
	getPacketIndex := func() string {
		packetCountLock.Lock()
		defer packetCountLock.Unlock()
		return fmt.Sprintf("%v_%v", offset, count)
	}

	// 缓存 Websocket ID (当前程序的指针，一般不太会有问题)
	/*
		真正开始劫持的函数，以下内容分别针对
		1. 劫持 Websocket 的请求和响应
		2. 劫持普通 HTTP 的请求和响应
		3. 镜像 HTTP 请求和响应
	*/
	var wshashFrameIndexLock sync.Mutex
	var mServer *crep.MITMServer
	var websocketHashCache = new(sync.Map)
	var wshashFrameIndex = make(map[string]int)
	var requireWsFrameIndexByWSHash = func(i string) int {
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
		//如果需要劫持响应则需要处理来自req的锁
		if httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			defer mitmLock.Unlock()
		}

		/* 这儿比单纯劫持响应要简单的多了 */
		originRspRaw := raw[:]
		finalResult = originRspRaw

		// 保存到数据库
		wshash := utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		err := yakit.SaveFromServerWebsocketFlow(s.GetProjectDatabase(), wshash, requireWsFrameIndexByWSHash(wshash), raw[:])
		if err != nil {
			log.Warnf("save websocket flow(from server) failed: %s", err)
		}

		if autoForward.IsSet() {
			// 自动转发的内容，按理说这儿应该接入内部规则
			return originRspRaw
		}

		if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return raw
		}
		//_, ok := hijackedResponseByRequestPTRMap.Load(wshash)
		//if !ok {
		//	return raw
		//}

		_, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		responseCounter := time.Now().UnixNano()
		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Response:    raw,
			Url:         urlStr,
			ResponseId:  responseCounter,
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

	handleHijackResponse := func(isHttps bool, req *http.Request, rspInstance *http.Response, rsp []byte, remoteAddr string) []byte {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("hijack response error: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		// 如果劫持响应就需要释放来自req的锁
		if httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			defer mitmLock.Unlock()
		}
		/*
		   这里是调用 hijackHTTPResponse 的问题
		*/
		originRspRaw := rsp[:]

		plainResponse := httpctx.GetPlainResponseBytes(req)
		if len(plainResponse) <= 0 {
			plainResponse = lowhttp.DeletePacketEncoding(httpctx.GetBareResponseBytes(req))
			httpctx.SetPlainResponseBytes(req, plainResponse)
		}
		rsp = plainResponse

		var urlStr = httpctx.GetRequestURL(req)

		// use handled request
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

		plainResponseHash := codec.Sha256(plainResponse)
		handleResponseModified := func(r []byte) bool {
			if codec.Sha256(r) != plainResponseHash {
				return true
			}
			return false
		}

		var dropped = utils.NewBool(false)
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

			if dropped.IsSet() {
				httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
				return nil
			}
		}

		if httpctx.GetResponseIsModified(req) {
			rsp = httpctx.GetHijackedResponseBytes(req)
		}

		// 自动转发与否
		if autoForward.IsSet() {
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
		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Response:    rsp,
			Request:     plainRequest,
			ResponseId:  responseCounter,
			RemoteAddr:  remoteAddr,
		}
		err = send(feedbackRspIns)
		if err != nil {
			log.Errorf("send response failed: %s", err)
			return rsp
		}

		httpctx.SetResponseViewedByUser(req)
		for {
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
		var shouldHijackResponse bool
		mitmLock.Lock()
		defer func() {
			// 根据是否劫持响应来判断是否释放锁
			if !shouldHijackResponse {
				mitmLock.Unlock()
			}
		}()
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("hijack ws websocket failed: %s", err)
				return
			}
		}()

		isTls, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		wshash := utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		_, ok := websocketHashCache.Load(wshash)
		if !ok {
			// 证明这是新的 wshash
			// 在这儿可以给数据库增加一个记录了
			websocketHashCache.Store(wshash, true)

			flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySaved(
				isTls, req, rsp, "mitm", urlStr, httpctx.GetRemoteAddr(req),
			)
			if err != nil {
				log.Errorf("httpflow failed: %s", err)
			}
			if flow != nil {
				flow.IsWebsocket = true
				flow.WebsocketHash = wshash
				flow.HiddenIndex = wshash
				flow.Hash = flow.CalcHash()
				err = yakit.InsertHTTPFlow(s.GetProjectDatabase(), flow)
				if err != nil {
					log.Errorf("create / save httpflow(websocket) error: %s", err)
				}
			}
		}

		originReqRaw := raw[:]
		finalResult = originReqRaw

		// 保存每一个请求
		err = yakit.SaveToServerWebsocketFlow(s.GetProjectDatabase(), wshash, requireWsFrameIndexByWSHash(wshash), raw[:])
		if err != nil {
			log.Warnf("save to websocket flow failed: %s", err)
		}

		// MITM 手动劫持放行
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
					Request:             raw,
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
					shouldHijackResponse = false
					return nil
				}

				// 原封不动转发
				if reqInstance.GetForward() {
					return originReqRaw
				}

				if reqInstance.GetHijackResponse() {
					// 设置需要劫持resp
					shouldHijackResponse = true // 设置不释放锁
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					log.Infof("the ws hash: %s's mitm ws response is waiting for hijack response", wshash)
				}
				if reqInstance.GetCancelhijackResponse() {
					// 设置不需要劫持resp
					shouldHijackResponse = false
					httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, false)
					log.Infof("the ws hash: %s's mitm ws response cancel hijack response", wshash)
				}

				// 把修改后的请求放回去
				requestModified := reqInstance.GetRequest()
				return requestModified
			}
		}
	}
	handleHijackRequest := func(isHttps bool, originReqIns *http.Request, req []byte) []byte {
		httpctx.SetResponseContentTypeFiltered(originReqIns, func(t string) bool {
			ret := !filterManager.IsMIMEPassed(t)
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ResponseIsFiltered, ret)
			return ret
		})

		var shouldHijackResponse bool
		mitmLock.Lock()
		defer func() {
			// 根据是否劫持响应来判断是否释放锁
			if !shouldHijackResponse {
				mitmLock.Unlock()
			}
		}()
		// 保证始终只有一个 Goroutine 在处理请求
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("Hijack warning: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		httpctx.SetMatchedRule(originReqIns, make([]*ypb.MITMContentReplacer, 0))
		var originReqRaw = req[:]
		var (
			method = originReqIns.Method
		)

		// make it plain
		plainRequest := httpctx.GetPlainRequestBytes(originReqIns)
		if plainRequest == nil || len(plainRequest) == 0 {
			plainRequest = lowhttp.DeletePacketEncoding(httpctx.GetBareRequestBytes(originReqIns))
			httpctx.SetPlainRequestBytes(originReqIns, plainRequest)
		}

		// handle rules
		var originRequestHash = codec.Sha256(plainRequest)

		rules, req1, shouldBeDropped := replacer.hook(true, false, req, isHttps)
		if shouldBeDropped {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
			log.Warn("MITM: request dropped by hook (VIA replacer.hook)")
			return nil
		}
		httpctx.AppendMatchedRule(originReqIns, rules...)

		// modified ?
		handleRequestModified := func(newReqBytes []byte) bool {
			return codec.Sha256(newReqBytes) != originRequestHash
		}
		var modifiedByRule = false
		if handleRequestModified(req1) {
			req = req1
			modifiedByRule = true
			httpctx.SetHijackedRequestBytes(originReqIns, req1)
			httpctx.SetRequestModified(originReqIns, "yakit.mitm.replacer")
		}

		/* 由 MITM Hooks 触发 */
		var (
			dropped  = utils.NewBool(false)
			urlStr   = ""
			hostname = originReqIns.Host
			extName  = ""
		)
		urlRaw, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps)
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
		mitmPluginCaller.CallHijackRequest(isHttps, urlStr,
			func() interface{} {
				if modifiedByRule {
					return httpctx.GetHijackedRequestBytes(originReqIns)
				}
				return plainRequest
			}, constClujore(func(replaced interface{}) {
				if dropped.IsSet() {
					return
				}
				if replaced != nil {
					if ret := codec.AnyToBytes(replaced); handleRequestModified(ret) {
						httpctx.SetRequestModified(originReqIns, "yaklang.hook")
						httpctx.SetHijackedRequestBytes(originReqIns, lowhttp.FixHTTPRequest(ret))
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

		// 过滤
		if !filterManager.IsPassed(method, hostname, urlStr, extName, isHttps) {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return req
		}

		// MITM 手动劫持放行
		if autoForward.IsSet() {
			httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_AutoFoward, true)
			return req
		}

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

				var isMultipartData = lowhttp.IsMultipartFormDataRequest(req)
				if isMultipartData {
					feedbackOrigin.Request = lowhttp.ConvertHTTPRequestToFuzzTag(req)
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

				if reqInstance.GetHijackResponse() {
					// 设置需要劫持resp
					shouldHijackResponse = true // 设置不释放锁
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					hijackedPtr := fmt.Sprintf("%p", originReqIns)
					log.Infof("the ptr: %v's mitm request is waiting for hijack response", hijackedPtr)
					continue
				}
				if reqInstance.GetCancelhijackResponse() {
					shouldHijackResponse = false
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
					shouldHijackResponse = false
					log.Infof("MITM %v recv drop hijacked request[%v]", addr, reqInstance.GetId())
					httpctx.SetContextValueInfoFromRequest(originReqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
					// 保存到数据库
					log.Debugf("start to create httpflow from mitm[%v %v]", originReqIns.Method, truncate(originReqIns.URL.String()))
					var startCreateFlow = time.Now()
					flow, err := yakit.CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps, originReqIns, "mitm", originReqIns.URL.String(), remoteAddr)
					if err != nil {
						log.Errorf("save http flow[%v %v] from mitm failed: %s", originReqIns.Method, originReqIns.URL.String(), err)
						return nil
					}
					log.Debugf("yakit.CreateHTTPFlowFromHTTPWithBodySaved for %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
					// Hidden Index 用来标注 MITM 劫持的顺序
					flow.HiddenIndex = getPacketIndex()
					flow.Hash = flow.CalcHash()
					flow.AddTagToFirst("[被丢弃]")
					flow.Purple()

					log.Debugf("mitmPluginCaller.HijackSaveHTTPFlow for %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
					startCreateFlow = time.Now()

					// HOOK 存储过程
					if flow != nil {
						flow.Hash = flow.CalcHash()
						flow := flow
						flow.StatusCode = 200 // 这里先设置成200
						//log.Infof("start to do sth with tag")
						for i := 0; i < 3; i++ {
							startCreateFlow = time.Now()
							// 用户丢弃请求后，这个flow表现在http history中应该是不包含响应的
							flow.Response = ""
							err = yakit.InsertHTTPFlow(s.GetProjectDatabase(), flow)
							log.Debugf("insert http flow %v cost: %s", truncate(originReqIns.URL.String()), time.Now().Sub(startCreateFlow))
							if err != nil {
								log.Errorf("create / save httpflow from mirror error: %s", err)
								time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
								continue
							}
							break
						}
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
					httpctx.SetRequestModified(originReqIns, "user")
					httpctx.SetHijackedRequestBytes(originReqIns, current)
				}
				return current
			}
		}
	}

	handleMirrorResponse := func(isHttps bool, reqUrl string, req *http.Request, rsp *http.Response, remoteAddr string) {
		addCounter()

		// 不符合劫持条件就不劫持
		var isFilteredByResponse = httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ResponseIsFiltered)
		var isFilteredByRequest = httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsFiltered)
		var isFiltered = isFilteredByResponse || isFilteredByRequest
		var modified = httpctx.GetRequestIsModified(req) || httpctx.GetResponseIsModified(req)
		var viewed = httpctx.GetRequestViewedByUser(req) || httpctx.GetResponseViewedByUser(req)

		var plainRequest []byte
		if httpctx.GetRequestIsModified(req) {
			plainRequest = httpctx.GetHijackedRequestBytes(req)
		} else {
			plainRequest = httpctx.GetPlainRequestBytes(req)
			if len(plainRequest) <= 0 {
				plainRequest = lowhttp.DeletePacketEncoding(httpctx.GetBareRequestBytes(req))
			}
		}

		var responseOverSize = false
		var plainResponse []byte
		if httpctx.GetResponseIsModified(req) {
			plainResponse = httpctx.GetHijackedResponseBytes(req)
		} else {
			plainResponse = httpctx.GetPlainResponseBytes(req)
			if len(plainResponse) <= 0 {
				plainResponse = lowhttp.DeletePacketEncoding(httpctx.GetBareResponseBytes(req))
			}
		}
		if len(plainResponse) > packetLimit {
			responseOverSize = true
		}
		header, body := lowhttp.SplitHTTPPacketFast(plainResponse)
		if responseOverSize {
			plainResponse = []byte(header)
		}

		shouldBeHijacked := !isFiltered
		go func() {
			mitmPluginCaller.MirrorHTTPFlow(isHttps, reqUrl, plainRequest, plainResponse, body, shouldBeHijacked)
		}()

		// 劫持过滤
		if isFiltered {
			return
		}

		// 保存到数据库
		log.Debugf("start to create httpflow from mitm[%v %v]", req.Method, truncate(reqUrl))
		var startCreateFlow = time.Now()
		var flow *yakit.HTTPFlow
		if httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_NOLOG) {
			flow, err = yakit.CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps, req, "mitm", reqUrl, remoteAddr)
			flow.StatusCode = 200 //先设置成200
		} else {
			flow, err = yakit.CreateHTTPFlowFromHTTPWithBodySaved(isHttps, req, rsp, "mitm", reqUrl, remoteAddr) // , !responseOverSize)
		}
		if err != nil {
			log.Errorf("save http flow[%v %v] from mitm failed: %s", req.Method, reqUrl, err)
			return
		}
		log.Debugf("yakit.CreateHTTPFlowFromHTTPWithBodySaved for %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))
		startCreateFlow = time.Now()

		flow.FitHTTPRequest(req)

		// Hidden Index 用来标注 MITM 劫持的顺序
		flow.HiddenIndex = getPacketIndex()
		flow.Hash = flow.CalcHash()
		if viewed {
			if modified {
				flow.AddTagToFirst("[手动修改]")
				flow.Red()
			} else {
				flow.AddTagToFirst("[手动劫持]")
				flow.Orange()
			}
		}

		var hijackedFlowMutex = new(sync.Mutex)
		var dropped = utils.NewBool(false)
		mitmPluginCaller.HijackSaveHTTPFlow(
			flow,
			func(replaced *yakit.HTTPFlow) {
				if replaced == nil {
					return
				}
				hijackedFlowMutex.Lock()
				defer hijackedFlowMutex.Unlock()

				*flow = *replaced
			},
			func() {
				dropped.IsSet()
			},
		)
		log.Debugf("mitmPluginCaller.HijackSaveHTTPFlow for %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))

		// storage
		if flow != nil && !dropped.IsSet() {
			flow.Hash = flow.CalcHash()
			flow := flow
			startCreateFlow = time.Now()

			var colorOK = make(chan struct{})
			var extracted []*yakit.ExtractedData

			if replacer != nil {
				go func() {
					extracted = replacer.hookColor(plainRequest, plainResponse, req, flow)
					log.Debugf("replacer.hookColor(requestRaw, responseRaw, req, flow); for %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))
					close(colorOK)
				}()
			} else {
				close(colorOK)
			}

			// wait for max 300ms
			select {
			case <-colorOK:
			case <-time.NewTimer(time.Millisecond * 300).C:
			}

			var GoColor bool
			if err := utils.RetryWithExpBackOff(func() (err error) {
				select {
				case <-colorOK:
					GoColor = false
				default:
					GoColor = true
				}

				tx := s.GetProjectDatabase().Begin()
				defer func() {
					if err != nil {
						tx.Rollback()
					}
				}()

				if err = yakit.InsertHTTPFlow(tx, flow); err != nil {
					return err
				}

				// for color hook
				if !GoColor {
					if len(extracted) != 0 {
						for _, e := range extracted {
							if err := yakit.CreateOrUpdateExtractedData(tx, -1, e); err != nil {
								return err
							}
						}
					}
					if err = yakit.UpdateHTTPFlowTags(tx, flow); err != nil {
						return err
					}
				}

				return tx.Commit().Error
			}); err != nil {
				log.Errorf("create / save httpflow from mirror error: %s", err)
				flow.HiddenIndex = uuid.NewV4().String() + "_" + flow.HiddenIndex
			}

			if GoColor {
				go func() {
					<-colorOK
					if err := utils.RetryWithExpBackOff(func() (err error) {
						tx := consts.GetGormProjectDatabase().Begin()
						defer func() {
							if err != nil {
								tx.Rollback()
							}
						}()

						if len(extracted) != 0 {
							for _, e := range extracted {
								if err := yakit.CreateOrUpdateExtractedData(tx, -1, e); err != nil {
									return err
								}
							}
						}
						if err = yakit.UpdateHTTPFlowTags(tx, flow); err != nil {
							return err
						}

						return tx.Commit().Error
					}); err != nil {
						log.Errorf("hookcolor error: %s", err)
					}
				}()
			}

			log.Debugf("insert http flow %v cost: %s", truncate(reqUrl), time.Now().Sub(startCreateFlow))
		}

		// 存储KV，将flow ID作为key，bare request和bare response作为value
		if httpctx.GetRequestIsModified(req) {
			bareReq := httpctx.GetPlainRequestBytes(req)
			if len(bareReq) == 0 {
				bareReq = httpctx.GetBareRequestBytes(req)
			}
			log.Debugf("[KV] save bare Response(%d)", flow.ID)

			if len(bareReq) > 0 && flow.ID > 0 {
				keyStr := strconv.FormatUint(uint64(flow.ID), 10) + "_request"
				yakit.SetProjectKeyWithGroup(s.GetProjectDatabase(), keyStr, bareReq, yakit.BARE_REQUEST_GROUP)
			}
		}

		if httpctx.GetResponseIsModified(req) {
			bareRsp := httpctx.GetPlainResponseBytes(req)
			if len(bareRsp) == 0 {
				bareRsp = httpctx.GetBareResponseBytes(req)
			}
			log.Debugf("[KV] save bare Response(%d)", flow.ID)

			if len(bareRsp) > 0 && flow.ID > 0 {
				keyStr := strconv.FormatUint(uint64(flow.ID), 10) + "_response"
				yakit.SetProjectKeyWithGroup(s.GetProjectDatabase(), keyStr, bareRsp, yakit.BARE_RESPONSE_GROUP)
			}
		}

	}
	// 核心 MITM 服务器
	var opts []crep.MITMConfig
	for _, cert := range firstReq.GetCertificates() {
		opts = append(opts, crep.MITM_MutualTLSClient(cert.CrtPem, cert.KeyPem, cert.GetCaCertificates()...))
	}

	mServer, err = crep.NewMITMServer(
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

	feedbackToUser("MITM 服务器已启动 / starting mitm server")
	log.Infof("start serve mitm server for %s", addr)
	//err = mServer.Run(ctx)
	err = mServer.Serve(ctx, utils.HostPort(host, port))
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
	var result = yakit.GetKey(s.GetProfileDatabase(), MITMReplacerKeyRecords)
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
