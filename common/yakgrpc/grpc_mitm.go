package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

func _checker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	if includes == nil {
		if utils.StringGlobArrayContains(excludes, target) {
			return false
		}
		return true
	} else {
		if utils.StringGlobArrayContains(excludes, target) {
			return false
		}
		if utils.StringGlobArrayContains(includes, target) {
			return true
		}
		return false
	}
}

const REQUEST_CONTEXT_KEY_MatchedRules = "MatchedRules"
const REQUEST_CONTEXT_KEY_INFOMAP = "InfoMap"

var enabledHooks = yak.MITMAndPortScanHooks
var saveHTTPFlowMutex = new(sync.Mutex)

var mustAcceptEncodingRegexp = regexp.MustCompile(`(?i)Accept-Encoding: ([^\r\n]*)?`)

func StripHTTPRequestGzip(reqIns *http.Request, req []byte) (*http.Request, []byte) {
	// Accept-Encoding => identity
	var haveAcceptEncoding = false
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req, func(line string) {
		if strings.HasPrefix(
			strings.ToLower(strings.TrimSpace(line)),
			"accept-encoding") {
			haveAcceptEncoding = true
		}
	})
	if haveAcceptEncoding {
		indexes := mustAcceptEncodingRegexp.FindStringSubmatchIndex(header)
		if len(indexes) >= 4 {
			start, end := indexes[2], indexes[3]
			header = header[:start] + "identity" + header[end:]
		}
	} else {
		header = header[:len(header)-2]
		header += "Accept-Encoding: identity\r\n\r\n"
	}

	var buffer = bytes.NewBufferString(header)
	buffer.Write(body)
	return reqIns, buffer.Bytes()
}

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
		host, port, err := utils.ParseStringToHostPort(proxyUrl.Host)
		if err != nil {
			feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "parse host to host:port failed "+err.Error()))
			return utils.Errorf("parse proxy host failed: %s", proxyUrl.Host)
		}
		if port <= 0 {
			feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "缺乏端口（Miss Port）"))
			return utils.Errorf("proxy miss port. [%v]", proxyUrl.Host)
		}
		conn, err := net.DialTimeout("tcp", utils.HostPort(host, port), 5*time.Second)
		if err != nil {
			feedbackToUser(fmt.Sprintf("下游代理检测失败 / downstream proxy failed:[%v] %v", downstreamProxy, "代理不通（Proxy Cannot be connected）"))
			return utils.Errorf("proxy cannot be connected: %v", proxyUrl.String())
		}
		conn.Close()
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
	var packetLimit = 10 * 1000 * 1000
	//var includeHostname []string
	//var excludeHostname = []string{
	//	"google.com", "*google.com", "*google*.com",
	//	//"baidu.com", "*baidu.com",
	//}
	//var excludeSuffix = []string{
	//	".css",
	//	".jpg", ".jpeg", ".png",
	//	".mp3", ".mp4", ".ico", ".bmp",
	//	".flv", ".aac", ".ogg", "avi",
	//	".svg", ".gif", ".woff", ".woff2",
	//	".doc", ".docx", ".pptx",
	//	".ppt", ".pdf"}
	//var includeSuffix []string
	//var excludeMethods = []string{"OPTIONS", "CONNECT"}
	//var excludeMIME = []string{
	//	// https://www.runoob.com/http/http-content-type.html
	//	"image/*",
	//	"audio/*", "video/*",
	//	"application/ogg", "application/pdf", "application/msword",
	//	"application/x-ppt", "video/avi", "application/x-ico",
	//}

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

	var shouldBeHijacked = func(r []byte, isHttps bool) bool {
		reqInstance, err := lowhttp.ParseBytesToHttpRequest(r)
		if err != nil {
			log.Errorf("mitm filter error: parse http request failed: %s", err)
			return false
		}

		urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(r, isHttps)
		if err != nil {
			log.Errorf("mitm filter error: parse req to url failed: %s", err)
			return false
		}

		host, _, err := utils.ParseStringToHostPort(urlIns.String())
		if err != nil {
			return false
		}

		// 后缀过滤
		ext := filepath.Ext(urlIns.Path)
		return _checker(filterManager.IncludeSuffix, filterManager.ExcludeSuffix, strings.ToLower(ext)) &&
			_checker(filterManager.IncludeHostnames, filterManager.ExcludeHostnames, strings.ToLower(host)) &&
			_checker(nil, filterManager.ExcludeMethods, strings.ToUpper(reqInstance.Method)) &&
			_checker(nil, filterManager.ExcludeMIME, strings.ToLower(reqInstance.Header.Get("Content-Type"))) &&
			_checker(filterManager.IncludeUri, filterManager.ExcludeUri, strings.ToLower(reqInstance.RequestURI))
	}

	var responseShouldBeHijacked = func(rsp *http.Response, isHttps bool) bool {
		//l, _ := strconv.Atoi(rsp.Header.Get("Content-Length"))
		//if l > packetLimit {
		//	return false
		//}
		return _checker(nil, filterManager.ExcludeMIME, strings.ToLower(rsp.Header.Get("Content-Type")))
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

			go func() {
				defer func() {
					recover()
				}()
				select {
				case messageChan <- reqInstance:
				case <-stream.Context().Done():
					return
				}
			}()
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
	var mServer *crep.MITMServer
	var websocketHashCache = new(sync.Map)
	var wshashFrameIndex = make(map[string]int)
	var requireWsFrameIndexByWSHash = func(i string) int {
		/*这个函数目前用在 Hijack 里面，不太需要加锁，因为 mitmLock 已经一般生效了*/
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
				log.Errorf("hijack response error: %s", err)
			}
		}()
		mitmLock.Lock()
		defer mitmLock.Unlock()

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

		if !GetContextBoolInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return raw
		}
		//_, ok := hijackedResponseByRequestPTRMap.Load(wshash)
		//if !ok {
		//	return raw
		//}

		_, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		host, port, _ := utils.ParseStringToHostPort(urlStr)
		remoteAddr := mServer.GetRemoteAddrRaw(utils.HostPort(host, port))
		responseCounter := time.Now().UnixNano()
		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Response:    raw,
			Url:         urlStr,
			ResponseId:  responseCounter,
			RemoteAddr:  remoteAddr,
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
	handleHijackResponse := func(isHttps bool, req *http.Request, rsp []byte, remoteAddr string) []byte {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("hijack response error: %s", err)
			}
		}()
		mitmLock.Lock()
		defer mitmLock.Unlock()

		/*
		   这里是调用 hijackHTTPResponse 的问题
		*/
		originRspRaw := rsp[:]
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseBytes, string(originRspRaw))
		var urlStr = GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_Url)

		var handled = utils.NewBool(false)
		var dropped = utils.NewBool(false)
		var modifiedResponse []byte

		var requestRaw = []byte(GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestBytes))
		if len(requestRaw) <= 0 {
			requestRaw, _ = utils.HttpDumpWithBody(req, true)
		}

		// 响应，带请求
		mitmPluginCaller.CallHijackResponseEx(isHttps, urlStr, func() interface{} {
			return requestRaw
		}, func() interface{} {
			var fixedResponse, _, _ = lowhttp.FixHTTPResponse(originRspRaw[:])
			if len(fixedResponse) > 0 {
				return fixedResponse
			} else {
				return originRspRaw[:]
			}
		}, constClujore(func(i interface{}) {
			handled.Set()
			modifiedResponse = utils.InterfaceToBytes(i)
		}), constClujore(func() {
			handled.Set()
			dropped.Set()
		}))

		// 响应
		mitmPluginCaller.CallHijackResponse(isHttps, urlStr, func() interface{} {
			var fixedResponse, _, _ = lowhttp.FixHTTPResponse(originRspRaw[:])
			if len(fixedResponse) > 0 {
				return fixedResponse
			} else {
				return originRspRaw[:]
			}
		}, constClujore(func(i interface{}) {
			handled.Set()
			modifiedResponse = utils.InterfaceToBytes(i)
		}), constClujore(func() {
			handled.Set()
			dropped.Set()
		}))
		if handled.IsSet() {
			if dropped.IsSet() {
				return nil
			} else {
				rsp = modifiedResponse[:]
			}
		}

		// 自动转发与否
		if autoForward.IsSet() {
			SetContextValueInfoFromRequest(req, RESPONSE_CONTEXT_KEY_AutoFoward, true)
			/*
				自动过滤下，不是所有 response 都应该替换
				应该替换的条件是不匹配过滤器的内容
			*/

			// 这个来过滤一些媒体文件
			rspIns, _ := lowhttp.ParseBytesToHTTPResponse(rsp)
			if rspIns != nil {
				if !responseShouldBeHijacked(rspIns, isHttps) {
					SetContextValueInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ResponseIsFiltered, true)
					return rsp
				}
			}

			// 处理响应规则
			if replacer.haveHijackingRules() {
				rules, rspHooked, dropped := replacer.hook(false, true, rsp)
				if dropped {
					SetContextValueInfoFromRequest(req, RESPONSE_CONTEXT_KEY_IsDropped, true)
					log.Warn("response should be dropped(VIA replacer.hook)")
					return nil
				}
				if v := req.Context().Value(REQUEST_CONTEXT_KEY_MatchedRules); v != nil {
					if v1, ok := v.(*[]*ypb.MITMContentReplacer); ok {
						*v1 = append(*v1, rules...)
					}
				}
				return rspHooked
			}
			return rsp
		}

		rules, rsp1, shouldBeDropped := replacer.hook(false, true, rsp)
		if shouldBeDropped {
			log.Warn("response should be dropped(VIA replacer.hook)")
			SetContextValueInfoFromRequest(req, RESPONSE_CONTEXT_KEY_IsDropped, true)
			return nil
		}
		rsp = rsp1
		if v := req.Context().Value(REQUEST_CONTEXT_KEY_MatchedRules); v != nil {
			if v1, ok := v.(*[]*ypb.MITMContentReplacer); ok {
				*v1 = append(*v1, rules...)
			}
		}
		responseCounter := time.Now().UnixNano()

		ptr := fmt.Sprintf("%p", req)
		if !GetContextBoolInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest) {
			return rsp
		}
		//_, ok := hijackedResponseByRequestPTRMap.Load(ptr)
		//if !ok {
		//	// 不在响应的劫持列表，返回
		//	return rsp
		//}

		// 确定要劫持该响应
		// hijackedResponseByRequestPTRMap.Delete(ptr)

		rsp, _, err := lowhttp.FixHTTPResponse(rsp)
		if err != nil {
			log.Errorf("fix http response packet failed: %s", err)
			return originRspRaw
		}
		feedbackRspIns := &ypb.MITMResponse{
			ForResponse: true,
			Response:    rsp,
			Request:     requestRaw,
			ResponseId:  responseCounter,
			RemoteAddr:  remoteAddr,
		}
		err = send(feedbackRspIns)
		if err != nil {
			log.Errorf("send response failed: %s", err)
			return rsp
		}

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
		mitmLock.Lock()
		defer mitmLock.Unlock()
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("hijack ws websocket failed: %s", err)
				return
			}
		}()

		isTls, urlStr := lowhttp.ExtractWebsocketURLFromHTTPRequest(req)
		host, port, _ := utils.ParseStringToHostPort(urlStr)
		remoteAddr := mServer.GetRemoteAddrRaw(utils.HostPort(host, port))

		wshash := utils.CalcSha1(fmt.Sprintf("%p", req), fmt.Sprintf("%p", rsp), ts)
		_, ok := websocketHashCache.Load(wshash)
		if !ok {
			// 证明这是新的 wshash
			// 在这儿可以给数据库增加一个记录了
			websocketHashCache.Store(wshash, true)

			flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySaved(
				s.GetProjectDatabase(), isTls, req, rsp, "mitm", urlStr, remoteAddr, true, true,
			)
			if err != nil {
				log.Errorf("httpflow failed: %s", err)
			}
			if flow != nil {
				flow.IsWebsocket = true
				flow.WebsocketHash = wshash
				flow.HiddenIndex = wshash
				flow.Hash = flow.CalcHash()
				err = yakit.InsertHTTPFlow(s.GetProjectDatabase(), flow.Hash, flow)
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
					RemoteAddr:          remoteAddr,
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
					return originReqRaw
				}

				if reqInstance.GetHijackResponse() {
					log.Infof("the ws hash: %s's mitm ws response is wait for hijacked", wshash)
					//hijackedResponseByRequestPTRMap.Store(hijackedPtr, req)
					SetContextValueInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					continue
				}

				// 把修改后的请求放回去
				requestModified := reqInstance.GetRequest()
				return requestModified
			}
		}
	}
	handleHijackRequest := func(isHttps bool, originReqIns *http.Request, req []byte) []byte {
		var matchedRules []*ypb.MITMContentReplacer
		matchedRulesP := &matchedRules
		ctx := context.WithValue(originReqIns.Context(), REQUEST_CONTEXT_KEY_MatchedRules, matchedRulesP)
		*originReqIns = *originReqIns.WithContext(ctx)
		var originReqRaw = req[:]
		SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsHttps, true)
		SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_RequestBytes, string(originReqRaw))
		// 保证始终只有一个 Goroutine 在处理请求
		mitmLock.Lock()
		defer mitmLock.Unlock()
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("Hijack warning: %v", err)
				return
			}
		}()

		// 触发劫持修改内容
		rules, req1, shouldBeDropped := replacer.hook(true, false, req, isHttps)
		if shouldBeDropped {
			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsDropped, true)
			log.Warn("MITM: request dropped by hook (VIA replacer.hook)")
			return nil
		}

		req = req1
		*matchedRulesP = append(matchedRules, rules...)
		/* 由 MITM Hooks 触发 */
		var (
			dropped          = utils.NewBool(false)
			hijackedByHook   = new(sync.Mutex)
			hijackedReqStore = map[string][]byte{
				"request": lowhttp.FixHTTPRequestOut(req),
			}
			urlStr = ""
		)
		urlRaw, _ := lowhttp.ExtractURLFromHTTPRequestRaw(hijackedReqStore["request"], isHttps)
		if urlRaw != nil {
			urlStr = urlRaw.String()
			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_Url, urlStr)
		}
		mitmPluginCaller.CallHijackRequest(isHttps, urlStr,
			func() interface{} {
				req, ok := hijackedReqStore["request"]
				if !ok {
					return make([]byte, 3)
				}
				return req
			}, constClujore(func(replaced interface{}) {
				if dropped.IsSet() {
					return
				}

				hijackedByHook.Lock()
				defer hijackedByHook.Unlock()

				if replaced != nil {
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsModified, true)
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_ModifiedBy, "hook")
					after := utils.InterfaceToBytes(replaced)
					hijackedReqStore["request"] = after
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_Modified, string(after))
				}
			}),
			constClujore(func() {
				SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsDropped, true)
				dropped.Set()
			}))

		// 如果丢弃就直接丢！
		if dropped.IsSet() {
			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsDropped, true)
			return nil
		}

		req, _ = hijackedReqStore["request"]
		req = lowhttp.FixHTTPRequestOut(req)
		if req == nil {
			req = originReqRaw
		}

		// 过滤
		if !shouldBeHijacked(req, isHttps) {
			log.Infof("req: %s is filtered", urlStr)
			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_RequestIsFiltered, true)
			return req
		}

		// MITM 手动劫持放行
		if autoForward.IsSet() {
			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_AutoFoward, true)
			return req
		}

		// 处理 gzip
		_, req = StripHTTPRequestGzip(nil, req)
		if req == nil {
			req = originReqRaw[:]
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

			SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_RequestIsHijacked, true)
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
					RemoteAddr:          mServer.GetRemoteAddr(isHttps, urlStr),
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

				// 如果 ID 对不上，返回来的是旧的，已经不需要处理的 ID，则重新接受等待新的
				if reqInstance.GetId() < id {
					log.Warnf("MITM %v recv old hijacked request[%v]", addr, reqInstance.GetId())
					goto RECV
				}

				// 直接丢包
				if reqInstance.GetDrop() {
					log.Infof("MITM %v recv drop hijacked request[%v]", addr, reqInstance.GetId())
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsDropped, true)
					return nil
				}

				// 原封不动转发
				if reqInstance.GetForward() {
					return originReqRaw
				}

				if reqInstance.GetHijackResponse() {
					// 设置将会当读劫持
					SetContextValueInfoFromRequest(originReqIns, RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
					hijackedPtr := fmt.Sprintf("%p", originReqIns)
					log.Infof("the ptr: %v's mitm request is waiting for hijacked", hijackedPtr)
					//hijackedResponseByRequestPTRMap.Store(hijackedPtr, originReqIns)
					continue
				}

				md5orig := codec.Md5(bytes.TrimSpace(feedbackOrigin.GetRequest()))
				current := reqInstance.GetRequest()
				if bytes.Contains(current, []byte{'{', '{'}) || bytes.Contains(current, []byte{'}', '}'}) {
					// 在这可能包含 fuzztag
					result := mutate.MutateQuick(current)
					if len(result) > 0 {
						current = []byte(result[0])
					}
				}
				md5after := codec.Md5(bytes.TrimSpace(current))
				if md5orig != md5after {
					log.Infof("MITM %v recv hijacked request[%v] changed", addr, reqInstance.GetId())
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_IsModified, true)
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_ModifiedBy, "user")
					SetContextValueInfoFromRequest(originReqIns, REQUEST_CONTEXT_KEY_Modified, string(current))
					ctx = context.WithValue(originReqIns.Context(), "Modified", 1)
					*originReqIns = *originReqIns.WithContext(ctx)
				} else {
					ctx = context.WithValue(originReqIns.Context(), "Viewed", 1)
					*originReqIns = *originReqIns.WithContext(ctx)
				}

				// 把能获取到的请求 / 修改好的请求放回去
				return current
			}
		}
	}

	handleMirrorResponse := func(isHttps bool, reqUrl string, req *http.Request, rsp *http.Response, remoteAddr string) {
		addCounter()

		// 不符合劫持条件就不劫持
		var isFilteredByResponse = GetContextBoolInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ResponseIsFiltered)
		var isFilteredByRequest = GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsFiltered)
		var isFilter = isFilteredByResponse || isFilteredByRequest
		var requestRaw = []byte(GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestBytes))
		if len(requestRaw) <= 0 {
			requestRaw, err = utils.HttpDumpWithBody(req, true)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
				return
			}
		}
		var modified = GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_IsModified)
		var viewed = GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsHijacked)

		// 处理 gzip
		var newRequest *http.Request
		newRequest, requestRaw = StripHTTPRequestGzip(req, requestRaw)
		if newRequest != nil {
			req = newRequest
		}

		responseRaw, _ := utils.HttpDumpWithBody(rsp, !utils.HTTPPacketIsLargerThanMaxContentLength(rsp, packetLimit))
		noGzippedResponse, body, _ := lowhttp.FixHTTPResponse(responseRaw)
		if noGzippedResponse != nil {
			responseRaw = noGzippedResponse
		}

		shouldeBeHijacked := !isFilter
		go func() {
			mitmPluginCaller.MirrorHTTPFlow(isHttps, reqUrl, requestRaw, responseRaw, body, shouldeBeHijacked)
		}()
		// 劫持过滤
		if isFilter {
			return
		}

		// 保存到数据库
		flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySaved(s.GetProjectDatabase(), isHttps, req, rsp, "mitm", reqUrl, remoteAddr, true, !utils.HTTPPacketIsLargerThanMaxContentLength(rsp, packetLimit))
		if err != nil {
			log.Errorf("save http flow[%v %v] from mitm failed: %s", req.Method, reqUrl, err)
			return
		}
		// Hidden Index 用来标注 MITM 劫持的顺序
		flow.HiddenIndex = getPacketIndex()

		flow.Hash = flow.CalcHash()
		if modified {
			flow.AddTagToFirst("[被修改]")
			flow.Red()
		}
		if viewed {
			flow.AddTagToFirst("[被劫持]")
			flow.Orange()
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

		// HOOK 存储过程
		if flow != nil && !dropped.IsSet() {
			flow.Hash = flow.CalcHash()
			flow := flow
			go func() {
				saveHTTPFlowMutex.Lock()
				defer saveHTTPFlowMutex.Unlock()

				defer func() {
					if err := recover(); err != nil {
						log.Error("panic from save httpflow to database! " + fmt.Sprint(err) + " current url: " + flow.Url)
					}
				}()

				//log.Infof("start to do sth with tag")
				if replacer != nil {
					replacer.hookColor(requestRaw, responseRaw, req, flow)
				}

				//if flow.Tags != "" {
				//	log.Infof("save with tag: %v", flow.Tags)
				//}
				for i := 0; i < 3; i++ {
					err = yakit.CreateOrUpdateHTTPFlow(s.GetProjectDatabase(), flow.CalcHash(), flow)
					if err != nil {
						log.Errorf("create / save httpflow from mirror error: %s", err)
						time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
						continue
					}
					return
				}
			}()
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
