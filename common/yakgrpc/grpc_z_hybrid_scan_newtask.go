package yakgrpc

import (
	"bytes"
	"container/list"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/mfreader"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"
)

func (s *Server) hybridScanNewTask(manager *HybridScanTaskManager, stream HybridScanRequestStream, firstRequest *ypb.HybridScanRequest) error {
	var (
		concurrent = firstRequest.GetConcurrent()
	)
	if concurrent <= 0 {
		concurrent = 20
	}
	swg := utils.NewSizedWaitGroup(int(concurrent))

	taskId := manager.TaskId()

	taskRecorder := &yakit.HybridScanTask{
		TaskId: taskId,
		Status: yakit.HYBRIDSCAN_EXECUTING,
	}
	err := yakit.SaveHybridScanTask(consts.GetGormProjectDatabase(), taskRecorder)
	if err != nil {
		return utils.Errorf("save task failed: %s", err)
	}

	quickSave := func() {
		yakit.SaveHybridScanTask(consts.GetGormProjectDatabase(), taskRecorder)
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
	log.Infof("waiting for recv input and plugin config: %v", taskId)
	for plugin == nil || target == nil {
		rsp, err := stream.Recv()
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
	}

	targetChan, err := s.TargetGenerator(manager.Context(), target)
	if err != nil {
		taskRecorder.Reason = err.Error()
		return utils.Errorf("TargetGenerator failed: %s", err)
	}

	pluginCache := list.New()

	/*
		统计状态
	*/
	//var totalTarget = int64(len(utils.ParseStringToLines(target.String())))
	//var targetFinished int64 = 0
	//var taskFinished int64 = 0
	//var totalPlugin int64 = 0
	//var getTotalTasks = func() int64 {
	//	return totalTarget * totalPlugin
	//}
	//var activeTask int64
	//var activeTarget int64
	//var taskCount int64

	pluginChan, err := s.PluginGenerator(pluginCache, manager.Context(), plugin)
	if err != nil {
		taskRecorder.Reason = err.Error()
		return utils.Errorf("load plugin generator failed: %s", err)
	}
	var pluginNames []string
	for r := range pluginChan {
		pluginNames = append(pluginNames, r.ScriptName)
	}
	if len(pluginNames) == 0 {
		taskRecorder.Reason = "no plugin loaded"
		return utils.Error("no plugin loaded")
	}

	// targetChan 的大小如何估算？目标数量（百万为单位） * 目标大小字节数为 M 数
	// 即，100w 个目标，每个目标占用大小为 100 字节，那么都在内存中，开销大约为 100M
	// 这个开销在内存中处理绰绰有余，但是在网络传输中，这个开销就很大了
	var targetCached []*HybridScanTarget
	for targetInput := range targetChan {
		targetCached = append(targetCached, targetInput)
	}

	statusManager := newHybridScanStatusManager(taskId, len(targetCached), len(pluginNames))

	statusMutex := new(sync.Mutex)
	getStatus := func() *ypb.HybridScanResponse {
		statusMutex.Lock()
		defer statusMutex.Unlock()
		return statusManager.GetStatus(taskRecorder)
	}
	feedbackStatus := func() {
		statusManager.Feedback(stream)
	}

	// start dispatch tasks
	for _, __currentTarget := range targetCached {
		// load targets
		statusManager.DoActiveTarget()

		pluginChan, err := s.PluginGenerator(pluginCache, manager.Context(), plugin)
		if err != nil {
			return utils.Errorf("generate plugin queue failed: %s", err)
		}
		targetWg := new(sync.WaitGroup)

		for __pluginInstance := range pluginChan {
			targetRequestInstance := __currentTarget
			pluginInstance := __pluginInstance
			if swgErr := swg.AddWithContext(manager.Context()); swgErr != nil {
				continue
			}
			targetWg.Add(1)

			manager.Checkpoint(func() {
				// 如果出现了暂停，立即保存进度
				taskRecorder.SurvivalTaskIndexes = utils.ConcatPorts(statusManager.GetCurrentActiveTaskIndexes())
				names, _ := json.Marshal(pluginNames)
				taskRecorder.Plugins = string(names)
				targetBytes, _ := json.Marshal(targetCached)
				taskRecorder.Targets = string(targetBytes)
				feedbackStatus()
				taskRecorder.Status = yakit.HYBRIDSCAN_PAUSED
				quickSave()
			})

			taskIndex := statusManager.DoActiveTask()
			feedbackStatus()

			go func() {
				defer swg.Done()

				defer targetWg.Done()
				defer func() {
					statusManager.DoneTask(taskIndex)
					statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
				}()

				statusManager.PushActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)

				// shrink context
				select {
				case <-manager.Context().Done():
					log.Infof("skip task %d via canceled", taskIndex)
					return
				default:
				}

				err := s.ScanTargetWithPlugin(taskId, manager.Context(), targetRequestInstance, pluginInstance, func(result *ypb.ExecResult) {
					// shrink context
					select {
					case <-manager.Context().Done():
						return
					default:
					}

					result.RuntimeID = taskId
					status := getStatus()
					status.CurrentPluginName = pluginInstance.ScriptName
					status.ExecResult = result
					stream.Send(status)
				})
				if err != nil {
					log.Warnf("scan target failed: %s", err)
				}
				time.Sleep(time.Duration(300+rand.Int63n(700)) * time.Millisecond)
			}()
		}
		// shrink context
		select {
		case <-manager.Context().Done():
			return utils.Error("task manager cancled")
		default:
		}

		go func() {
			// shrink context
			select {
			case <-manager.Context().Done():
				return
			default:
			}

			targetWg.Wait()
			statusManager.DoneTarget()
			feedbackStatus()
		}()
	}
	swg.Wait()
	feedbackStatus()
	if !manager.IsPaused() {
		taskRecorder.Status = yakit.HYBRIDSCAN_DONE
	}
	return nil
}

//go:embed grpc_z_hybrid_scan.yak
var execTargetWithPluginScript string

func (s *Server) ScanTargetWithPlugin(
	taskId string, ctx context.Context, target *HybridScanTarget, plugin *yakit.YakScript, callback func(result *ypb.ExecResult),
) error {
	feedbackClient := yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		callback(i)
		return nil
	})
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVar("RUNTIME_ID", taskId)
		yak.BindYakitPluginContextToEngine(engine, &yak.YakitPluginContext{
			PluginName: plugin.ScriptName,
			RuntimeId:  taskId,
			Proxy:      "",
		})
		engine.SetVar("REQUEST", target.Request)
		engine.SetVar("HTTPS", target.IsHttps)
		engine.SetVar("PLUGIN", plugin)
		engine.SetVar("CTX", ctx)
		return nil
	})
	err := engine.ExecuteWithContext(ctx, execTargetWithPluginScript)
	if err != nil {
		return utils.Errorf("execute script failed: %s", err)
	}
	return nil
}

func (s *Server) PluginGenerator(l *list.List, ctx context.Context, plugin *ypb.HybridScanPluginConfig) (chan *yakit.YakScript, error) {
	if l.Len() > 0 {
		// use cache
		front := l.Front()
		outC := make(chan *yakit.YakScript)
		go func() {
			defer func() {
				close(outC)
			}()
			for {
				if front == nil {
					break
				}
				outC <- front.Value.(*yakit.YakScript)
				front = front.Next()
			}
		}()
		return outC, nil
	}
	var outC = make(chan *yakit.YakScript)
	go func() {
		defer close(outC)

		for _, i := range plugin.GetPluginNames() {
			script, err := yakit.GetYakScriptByName(s.GetProfileDatabase().Model(&yakit.YakScript{}), i)
			if err != nil {
				continue
			}
			l.PushBack(script)
			outC <- script
		}
		if plugin.GetFilter() != nil {
			for pluginInstance := range yakit.YieldYakScripts(yakit.FilterYakScript(
				s.GetProfileDatabase().Model(&yakit.YakScript{}), plugin.GetFilter(),
			), ctx) {
				l.InsertAfter(pluginInstance, l.Back())
				outC <- pluginInstance
			}
		}
	}()
	return outC, nil

}

type HybridScanTarget struct {
	IsHttps bool
	Request []byte
	Url     string
}

func (s *Server) TargetGenerator(ctx context.Context, targetConfig *ypb.HybridScanInputTarget) (chan *HybridScanTarget, error) {
	// handle target
	baseBuilderParams := targetConfig.GetHTTPRequestTemplate()
	baseTemplates := []byte("GET {{Path}} HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")
	requestTemplates, err := s.HTTPRequestBuilder(ctx, targetConfig.GetHTTPRequestTemplate())
	if err != nil {
		log.Warn(err)
	}
	var templates = requestTemplates.GetResults()
	if len(templates) == 0 {
		templates = append(templates, &ypb.HTTPRequestBuilderResult{
			HTTPRequest: []byte("GET / HTTP/1.1\r\nHost: {{Hostname}}"),
		})
	}

	var target = targetConfig.GetInput()
	var files = targetConfig.GetInputFile()

	outC := make(chan string)
	go func() {
		defer close(outC)
		if target != "" {
			for _, line := range utils.ParseStringToLines(target) {
				outC <- line
			}
		}
		if len(files) > 0 {
			fr, err := mfreader.NewMultiFileLineReader(files...)
			if err != nil {
				log.Errorf("failed to read file: %v", err)
				return
			}
			for fr.Next() {
				line := fr.Text()
				if err != nil {
					break
				}
				outC <- line
			}
		}
	}()

	outTarget := make(chan *HybridScanTarget)
	go func() {
		defer func() {
			close(outTarget)
		}()
		for target := range outC {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			var urlIns *url.URL
			if utils.IsValidHost(target) { // 处理没有单独一个host情况 不含port
				urlIns = &url.URL{Host: target, Path: "/"}
			} else {
				urlIns = utils.ParseStringToUrl(target)
				if urlIns.Host == "" {
					continue
				}

				host, port, _ := utils.ParseStringToHostPort(urlIns.Host) // 处理包含 port 的情况
				if !utils.IsValidHost(host) {                             // host不合规情况 比如 a:80
					continue
				}

				if port > 0 && urlIns.Scheme == "" { // fix https
					if port == 443 {
						urlIns.Scheme = "https"
					}
				}
				if urlIns.Path == "" {
					urlIns.Path = "/"
				}
			}
			builderParams := mergeBuildParams(baseBuilderParams, urlIns)
			if builderParams == nil {
				continue
			}
			builderResponse, err := s.HTTPRequestBuilder(ctx, builderParams)
			if err != nil {
				log.Errorf("failed to build http request: %v", err)
			}
			results := builderResponse.GetResults()
			if len(results) <= 0 {
				packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(urlIns.Host))
				packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(urlIns.Path))
				outTarget <- &HybridScanTarget{
					IsHttps: urlIns.Scheme == "https",
					Request: packet,
					Url:     urlIns.String(),
				}
			} else {
				for _, result := range results {
					packet := bytes.ReplaceAll(result.HTTPRequest, []byte(`{{Hostname}}`), []byte(urlIns.Host))
					tUrl, _ := lowhttp.ExtractURLFromHTTPRequestRaw(packet, result.IsHttps)
					var uStr = urlIns.String()
					if tUrl != nil {
						uStr = tUrl.String()
					}
					outTarget <- &HybridScanTarget{
						IsHttps: result.IsHttps,
						Request: packet,
						Url:     uStr,
					}
				}
			}
		}

		//for target := range outC {
		//	for _, template := range templates {
		//		https, hostname := utils.ParseStringToHttpsAndHostname(target)
		//		if hostname == "" {
		//			log.Infof("skip invalid target: %v", target)
		//			continue
		//		}
		//		reqBytes := bytes.ReplaceAll(template.GetHTTPRequest(), []byte("{{Hostname}}"), []byte(hostname))
		//		uIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, https)
		//		var uStr = target
		//		if uIns != nil {
		//			uStr = uIns.String()
		//		}
		//		outTarget <- &HybridScanTarget{
		//			IsHttps: https,
		//			Request: reqBytes,
		//			Url:     uStr,
		//		}
		//	}
		//}
	}()
	return outTarget, nil
}
