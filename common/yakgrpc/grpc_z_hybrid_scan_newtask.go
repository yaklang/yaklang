package yakgrpc

import (
	"container/list"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) hybridScanNewTask(manager *HybridScanTaskManager, stream HybridScanRequestStream, firstRequest *ypb.HybridScanRequest) error {
	defer manager.Stop()
	taskId := manager.TaskId()
	taskRecorder := &schema.HybridScanTask{
		TaskId:               taskId,
		Status:               yakit.HYBRIDSCAN_EXECUTING,
		HybridScanTaskSource: firstRequest.GetHybridScanTaskSource(),
	}
	err := yakit.SaveHybridScanTask(consts.GetGormProjectDatabase(), taskRecorder)
	if err != nil {
		return utils.Errorf("save task failed: %s", err)
	}

	quickSave := func() {
		err := yakit.SaveHybridScanTask(consts.GetGormProjectDatabase(), taskRecorder)
		if err != nil {
			log.Error(err)
		}
	}

	defer func() {
		if err := recover(); err != nil {
			taskRecorder.Reason = fmt.Errorf("%v", err).Error()
			taskRecorder.Status = yakit.HYBRIDSCAN_ERROR
			quickSave()
			return
		}
		if taskRecorder.Status == yakit.HYBRIDSCAN_PAUSED {
			quickSave()
			return
		}
		taskRecorder.Status = yakit.HYBRIDSCAN_DONE
		quickSave()
	}()

	// read targets and plugin
	var target *ypb.HybridScanInputTarget
	var plugin *ypb.HybridScanPluginConfig
	var rsp *ypb.HybridScanRequest
	concurrent := 20 // 默认值
	var totalTimeout float32 = 72000
	var proxy string
	log.Infof("waiting for recv input and plugin config: %v", taskId)
	for plugin == nil || target == nil {
		rsp, err = stream.Recv()
		if err != nil {
			taskRecorder.Reason = err.Error()
			return err
		}
		if target == nil {
			target = rsp.GetTargets()
		}
		if plugin == nil {
			plugin = rsp.GetPlugin()
		}
		if rsp.GetConcurrent() > 0 {
			concurrent = int(rsp.GetConcurrent())
		}
		if rsp.GetTotalTimeoutSecond() > 0 {
			totalTimeout = rsp.GetTotalTimeoutSecond()
		}
		if rsp.GetProxy() != "" {
			proxy = rsp.GetProxy()
		}
	}
	taskRecorder.ScanConfig, _ = json.Marshal(rsp)
	quickSave()

	// 设置并发
	swg := utils.NewSizedWaitGroup(concurrent)
	// 设置总超时
	manager.ctx, manager.cancel = context.WithTimeout(manager.Context(), time.Duration(totalTimeout)*time.Second)

	// targetChan 的大小如何估算？目标数量（百万为单位） * 目标大小字节数为 M 数
	// 即，100w 个目标，每个目标占用大小为 100 字节，那么都在内存中，开销大约为 100M
	// 这个开销在内存中处理绰绰有余，但是在网络传输中，这个开销就很大了
	//generate target list
	targetChan, err := TargetGenerator(manager.Context(), s.GetProjectDatabase(), target)
	if err != nil {
		taskRecorder.Reason = err.Error()
		return utils.Errorf("TargetGenerator failed: %s", err)
	}
	// save targets
	var targetCached []*HybridScanTarget
	for targetInput := range targetChan {
		targetCached = append(targetCached, targetInput)
	}
	targetsBytes, err := json.Marshal(targetCached)
	if err != nil {
		return utils.Errorf("marshal targets failed: %s", err)
	}
	taskRecorder.Targets = string(targetsBytes)
	// generate plugin list
	pluginCache := list.New()
	pluginChan, err := s.PluginGenerator(pluginCache, manager.Context(), plugin)
	if err != nil {
		taskRecorder.Reason = err.Error()
		return utils.Errorf("load plugin generator failed: %s", err)
	}
	// save plugin list
	var pluginNames []string
	for r := range pluginChan {
		pluginNames = append(pluginNames, r.ScriptName)
	}
	if len(pluginNames) == 0 {
		taskRecorder.Reason = "no plugin loaded"
		return utils.Error("no plugin loaded")
	}
	pluginBytes, err := json.Marshal(pluginNames)
	if err != nil {
		return utils.Errorf("marshal plugin failed: %s", err)
	}
	taskRecorder.Plugins = string(pluginBytes)

	statusManager := newHybridScanStatusManager(taskId, len(targetCached), len(pluginNames), taskRecorder.Status)
	setTaskStatus := func(status string) { // change status , should set manager status ,need send to front end.
		taskRecorder.Status = status
		statusManager.Status = status
	}
	feedbackStatus := func() {
		statusManager.Feedback(stream)
	}
	globalFeedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(
		func(result *ypb.ExecResult) error {
			if manager.IsStop() || manager.IsPaused() {
				return nil
			}
			result.RuntimeID = taskId
			status := &ypb.HybridScanResponse{
				ExecResult: result,
			}
			return stream.Send(status)
		}, taskId)

	// Send RuntimeID immediately
	currentStatus := statusManager.GetStatus(taskRecorder)
	currentStatus.ExecResult = &ypb.ExecResult{RuntimeID: taskId}
	stream.Send(currentStatus)

	// init some config
	unreachableTargets := make([]string, 0)
	scanFilterManager := filter.NewFilterManager(12, 1<<15, 30)

	countRiskClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		result.RuntimeID = taskId
		currentStatus = statusManager.GetStatus(taskRecorder)
		currentStatus.ExecResult = result
		return stream.Send(currentStatus)
	})
	s.tickerRiskCountFeedback(manager.Context(), 2*time.Second, taskId, countRiskClient)
	defer s.forceRiskCountFeedback(taskId, countRiskClient)

	// build match
	matcher, err := fp.NewDefaultFingerprintMatcher(fp.NewConfig(fp.WithDatabaseCache(true), fp.WithCache(true)))
	if err != nil {
		return utils.Wrap(err, "init fingerprint matcher failed")
	}

	go func() { // 监听控制信号
		for {
			rsp, err = stream.Recv()
			if err != nil {
				taskRecorder.Reason = err.Error()
				return
			}
			if rsp.GetHybridScanMode() == "pause" {
				setTaskStatus(yakit.HYBRIDSCAN_PAUSED)
				feedbackStatus()
				manager.Pause()
				manager.Stop()
				quickSave()
			}
		}
	}()

	quickSave()
	// start dispatch tasks
	for _, __currentTarget := range targetCached {
		if manager.IsStop() || manager.IsPaused() { // if stop or pause, break immediately
			break // need send status to front end, can't return
		}
		// load targets
		statusManager.DoActiveTarget()
		targetWg := new(sync.WaitGroup)

		targetHostPort := utils.ExtractHostPort(__currentTarget.Url)
		conn, err := netx.DialX(targetHostPort)
		if err != nil {
			log.Errorf("dial target[%s] failed: %s", targetHostPort, err)
			globalFeedbackClient.YakitError("dial target[%s] failed: %s", targetHostPort, err)
			unreachableTargets = append(unreachableTargets, targetHostPort)
			statusManager.DoneFailureTarget()
			feedbackStatus()
			continue
		}
		conn.Close()

		// check can use mitm
		skipMitm := false
		resp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(__currentTarget.Request), lowhttp.WithHttps(__currentTarget.IsHttps), lowhttp.WithRuntimeId(taskId), lowhttp.WithProxy(proxy))
		if err != nil {
			skipMitm = true
		}
		__currentTarget.Response = resp.RawPacket

		// fingerprint match just once
		portScanCond := &sync.Cond{L: &sync.Mutex{}}
		fingerprintMatchOK := false
		go func() {
			host, port, _ := utils.ParseStringToHostPort(__currentTarget.Url)
			_, err = matcher.Match(host, port)
			if err != nil {
				log.Errorf("match fingerprint failed: %s", err)
			}
			portScanCond.L.Lock()
			defer portScanCond.L.Unlock()
			portScanCond.Broadcast()
			fingerprintMatchOK = true
		}()

		pluginChan, err := s.PluginGenerator(pluginCache, manager.Context(), plugin)
		if err != nil {
			return utils.Errorf("generate plugin queue failed: %s", err)
		}
		for __pluginInstance := range pluginChan {
			targetRequestInstance := __currentTarget
			pluginInstance := __pluginInstance
			if swgErr := swg.AddWithContext(manager.Context()); swgErr != nil {
				if errors.Is(swgErr, context.Canceled) {
					break
				}
				continue
			}
			targetWg.Add(1)

			taskIndex := statusManager.DoActiveTask(taskRecorder)
			feedbackStatus()

			if __pluginInstance.Type == "mitm" && skipMitm {
				log.Debugf("skip mitm plugin: %s", __pluginInstance.ScriptName)
				statusManager.DoneTask(taskIndex, taskRecorder)
				statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
				feedbackStatus()
				targetWg.Done()
				continue
			}

			for __pluginInstance.Type == "port-scan" && !fingerprintMatchOK { // wait for fingerprint match
				portScanCond.L.Lock()
				portScanCond.Wait()
				portScanCond.L.Unlock()
			}

			go func() {
				defer swg.Done()
				defer targetWg.Done()
				defer func() {
					if !manager.IsStop() { // 停止之后不再更新进度
						statusManager.DoneTask(taskIndex, taskRecorder)
					}
					statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
					feedbackStatus()
				}()
				// shrink context
				if manager.IsStop() {
					log.Infof("skip task %d via canceled", taskIndex)
					globalFeedbackClient.YakitInfo("skip task %d via canceled", taskIndex)
					return
				}
				statusManager.PushActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
				callerFilter := scanFilterManager.DequeueFilter()
				defer scanFilterManager.EnqueueFilter(callerFilter)
				feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(
					func(result *ypb.ExecResult) error {
						if manager.IsStop() || manager.IsPaused() {
							return nil
						}
						result.RuntimeID = taskId
						currentStatus = statusManager.GetStatus(taskRecorder)
						currentStatus.CurrentPluginName = pluginInstance.ScriptName
						currentStatus.ExecResult = result
						return stream.Send(currentStatus)
					}, taskId)
				err := ScanHybridTargetWithPlugin(taskId, manager.Context(), targetRequestInstance, pluginInstance, proxy, feedbackClient, callerFilter)
				if err != nil {
					log.Errorf("scan target[%s] failed: %s", targetHostPort, err)
					globalFeedbackClient.YakitError("scan target[%s] failed: %s", targetHostPort, err)
				}
				time.Sleep(time.Duration(300+rand.Int63n(700)) * time.Millisecond)
			}()

		}
		// shrink context
		if manager.IsStop() {
			break
		}
		go func() {
			// shrink context
			if manager.IsStop() {
				return
			}
			targetWg.Wait()
			if !manager.IsStop() { // 停止之后不再更新进度
				statusManager.DoneTarget()
				feedbackStatus()
			}
		}()
	}

	swg.Wait()
	statusManager.GetStatus(taskRecorder)
	if !manager.IsPaused() { // before return , should set status, if not paused,should set done
		setTaskStatus(yakit.HYBRIDSCAN_DONE)
	}
	feedbackStatus()
	quickSave()
	if len(unreachableTargets) > 0 {
		return utils.Errorf("Has un-reachable targets: %v", strings.Join(unreachableTargets, ", "))
	}
	return nil
}

//go:embed grpc_z_hybrid_scan.yak
var execTargetWithPluginScript string

func ScanHybridTargetWithPlugin(
	runtimeId string, ctx context.Context, target *HybridScanTarget, plugin *schema.YakScript, proxy string, feedbackClient *yaklib.YakitClient, callerFilter filter.Filterable,
) error {
	ctx, cancel := context.WithCancel(ctx)
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		yak.BindYakitPluginContextToEngine(engine, yak.CreateYakitPluginContext(
			runtimeId,
		).WithPluginName(
			plugin.ScriptName,
		).WithProxy(
			proxy,
		).WithContext(ctx).WithContextCancel(
			cancel,
		).WithYakitClient(feedbackClient))
		vars := map[string]any{
			"RUNTIME_ID":    runtimeId,
			"REQUEST":       target.Request,
			"RESPONSE":      target.Response,
			"HTTPS":         target.IsHttps,
			"PLUGIN":        plugin,
			"CTX":           ctx,
			"CALLER_FILTER": callerFilter,
			"PROXY":         proxy,
		}
		if target != nil && target.Vars != nil {
			if injected, ok := target.Vars["INJECTED_VARS"]; ok {
				vars["INJECTED_VARS"] = injected
			}
		}
		engine.SetVars(vars)
		return nil
	})
	err := engine.ExecuteWithContext(ctx, execTargetWithPluginScript)
	if err != nil {
		return utils.Errorf("execute script failed: %s", err)
	}
	return nil
}

func (s *Server) PluginGenerator(l *list.List, ctx context.Context, plugin *ypb.HybridScanPluginConfig) (chan *schema.YakScript, error) {
	if l.Len() > 0 {
		// use cache
		front := l.Front()
		outC := make(chan *schema.YakScript)
		go func() {
			defer func() {
				close(outC)
			}()
			for {
				if front == nil {
					break
				}
				outC <- front.Value.(*schema.YakScript)
				front = front.Next()
			}
		}()
		return outC, nil
	}
	outC := make(chan *schema.YakScript)
	go func() {
		defer close(outC)

		for _, i := range plugin.GetPluginNames() {
			script, err := yakit.GetYakScriptByName(s.GetProfileDatabase().Model(&schema.YakScript{}), i)
			if err != nil {
				continue
			}
			l.PushBack(script)
			outC <- script
		}
		if plugin.GetFilter() != nil {
			for pluginInstance := range yakit.YieldYakScripts(yakit.FilterYakScript(
				s.GetProfileDatabase().Model(&schema.YakScript{}), plugin.GetFilter(),
			), ctx) {
				l.PushBack(pluginInstance)
				outC <- pluginInstance
			}
		}
	}()
	return outC, nil
}

type HybridScanTarget struct {
	IsHttps  bool
	Request  []byte
	Response []byte
	Url      string
	Vars     map[string]any
}

func TargetGenerator(ctx context.Context, db *gorm.DB, targetConfig *ypb.HybridScanInputTarget) (chan *HybridScanTarget, error) {
	// handle target
	outTarget := make(chan *HybridScanTarget)
	inputTarget := targetConfig.GetInput()
	inputTargetFile := targetConfig.GetInputFile()
	if len(inputTargetFile) != 0 {
		inputTarget += "\n" + strings.Join(
			lo.FilterMap(inputTargetFile, func(file string, _ int) (string, bool) {
				fileContent, err := os.ReadFile(file)
				if err != nil {
					return "", false
				}
				return strings.ReplaceAll(string(fileContent), "\r", ""), true
			}), "\n",
		)
	}
	inputTarget = strings.TrimSpace(inputTarget)
	buildRes, err := BuildHttpRequestPacket(db, targetConfig.GetHTTPRequestTemplate(), inputTarget)
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(outTarget)
		for target := range buildRes {
			select {
			case <-ctx.Done():
				return
			case outTarget <- &HybridScanTarget{
				IsHttps: target.IsHttps,
				Request: target.Request,
				Url:     target.Url,
				Vars: map[string]any{
					"INJECTED_VARS": map[string]any{},
				},
			}:
				continue
			}
		}
	}()
	return outTarget, nil

	//outTarget := make(chan *HybridScanTarget)
	//baseBuilderParams := targetConfig.GetHTTPRequestTemplate()
	//if baseBuilderParams != nil && baseBuilderParams.IsRawHTTPRequest {
	//	reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(baseBuilderParams.RawHTTPRequest, baseBuilderParams.IsHttps)
	//	if err != nil {
	//		return nil, err
	//	}
	//	go func() {
	//		outTarget <- &HybridScanTarget{
	//			IsHttps: baseBuilderParams.IsHttps,
	//			Request: baseBuilderParams.RawHTTPRequest,
	//			Url:     reqUrl.String(),
	//		}
	//	}()
	//	//return outTarget, nil
	//}
	//
	//baseTemplates := []byte("GET {{Path}} HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")
	//requestTemplates, err := s.HTTPRequestBuilder(targetConfig.GetHTTPRequestTemplate())
	//if err != nil {
	//	log.Warn(err)
	//}
	//var templates = requestTemplates.GetResults()
	//if len(templates) == 0 {
	//	templates = append(templates, &ypb.HTTPRequestBuilderResult{
	//		HTTPRequest: []byte("GET / HTTP/1.1\r\nHost: {{Hostname}}"),
	//	})
	//}
	//
	//var target = targetConfig.GetInput()
	//var files = targetConfig.GetInputFile()
	//
	//outC := make(chan string)
	//go func() {
	//	defer close(outC)
	//	if target != "" {
	//		for _, line := range utils.PrettifyListFromStringSplitEx(target, "\n", "|", ",") {
	//			outC <- line
	//		}
	//	}
	//	if len(files) > 0 {
	//		fr, err := mfreader.NewMultiFileLineReader(files...)
	//		if err != nil {
	//			log.Errorf("failed to read file: %v", err)
	//			return
	//		}
	//		for fr.Next() {
	//			line := fr.Text()
	//			if err != nil {
	//				break
	//			}
	//			outC <- line
	//		}
	//	}
	//}()
	//
	//go func() {
	//	defer func() {
	//		close(outTarget)
	//	}()
	//	for target := range outC {
	//		target = strings.TrimSpace(target)
	//		if target == "" {
	//			continue
	//		}
	//		var urlIns *url.URL
	//		if utils.IsValidHost(target) { // 处理没有单独一个host情况 不含port
	//			urlIns = &url.URL{Host: target, Path: "/"}
	//		} else {
	//			urlIns = utils.ParseStringToUrl(target)
	//			if urlIns.Host == "" {
	//				continue
	//			}
	//
	//			host, port, _ := utils.ParseStringToHostPort(urlIns.Host) // 处理包含 port 的情况
	//			if !utils.IsValidHost(host) {                             // host不合规情况 比如 a:80
	//				continue
	//			}
	//
	//			if port > 0 && urlIns.Scheme == "" { // fix https
	//				if port == 443 {
	//					urlIns.Scheme = "https"
	//				}
	//			}
	//			if urlIns.Path == "" {
	//				urlIns.Path = "/"
	//			}
	//		}
	//		builderParams := mergeBuildParams(baseBuilderParams, urlIns)
	//		if builderParams == nil {
	//			continue
	//		}
	//		builderResponse, err := s.HTTPRequestBuilder(builderParams)
	//		if err != nil {
	//			log.Errorf("failed to build http request: %v", err)
	//		}
	//		results := builderResponse.GetResults()
	//		if len(results) <= 0 {
	//			packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(urlIns.Host))
	//			packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(urlIns.Path))
	//			outTarget <- &HybridScanTarget{
	//				IsHttps: urlIns.Scheme == "https",
	//				Request: packet,
	//				Url:     urlIns.String(),
	//			}
	//		} else {
	//			for _, result := range results {
	//				packet := bytes.ReplaceAll(result.HTTPRequest, []byte(`{{Hostname}}`), []byte(urlIns.Host))
	//				tUrl, _ := lowhttp.ExtractURLFromHTTPRequestRaw(packet, result.IsHttps)
	//				var uStr = urlIns.String()
	//				if tUrl != nil {
	//					uStr = tUrl.String()
	//				}
	//				outTarget <- &HybridScanTarget{
	//					IsHttps: result.IsHttps,
	//					Request: packet,
	//					Url:     uStr,
	//				}
	//			}
	//		}
	//	}
	//
	//	//for target := range outC {
	//	//	for _, template := range templates {
	//	//		https, hostname := utils.ParseStringToHttpsAndHostname(target)
	//	//		if hostname == "" {
	//	//			log.Infof("skip invalid target: %v", target)
	//	//			continue
	//	//		}
	//	//		reqBytes := bytes.ReplaceAll(template.GetHTTPRequest(), []byte("{{Hostname}}"), []byte(hostname))
	//	//		uIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, https)
	//	//		var uStr = target
	//	//		if uIns != nil {
	//	//			uStr = uIns.String()
	//	//		}
	//	//		outTarget <- &HybridScanTarget{
	//	//			IsHttps: https,
	//	//			Request: reqBytes,
	//	//			Url:     uStr,
	//	//		}
	//	//	}
	//	//}
	//}()
	//return outTarget, nil
}
