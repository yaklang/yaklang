package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed grpc_execBatchYakScript_batch_exec.yak
var batchExecScripts []byte

func (s *Server) ExecBatchYakScript(req *ypb.ExecBatchYakScriptRequest, stream ypb.Yak_ExecBatchYakScriptServer) error {
	// 用于管理进度保存相关内容
	manager := NewProgressManager(s.GetProjectDatabase())

	ctx, cancel := context.WithTimeout(stream.Context(), time.Duration(req.TotalTimeoutSeconds)*time.Second)
	defer cancel()

	extraParams := req.GetExtraParams()
	/*
		加载可执行的脚本
	*/
	var (
		rsp []*ypb.YakScript
		err error
	)

	proxyCurrent := strings.TrimSpace(req.GetProxy())
	if proxyCurrent != "" {
		extraParams = append(extraParams, &ypb.ExecParamItem{Key: "proxy", Value: proxyCurrent})
	}

	groupSize := req.GetProgressTaskCount()

	// 如果启动了插件过滤器的话
	if req.GetEnablePluginFilter() {
		if req.GetFromRecover() || len(req.GetPluginFilter().GetIncludedScriptNames()) > 200 {
			for _, y := range yakit.QueryYakScriptByNames(s.GetProfileDatabase(), req.GetPluginFilter().GetIncludedScriptNames()...) {
				rsp = append(rsp, y.ToGRPCModel())
			}
		} else {
			var filter = req.GetPluginFilter()
			if filter.GetNoResultReturn() {
				return utils.Errorf("no poc selected")
			}
			if filter.Pagination == nil {
				filter.Pagination = &ypb.Paging{Page: 1, Limit: 100000}
			} else {
				filter.Pagination.Page = 1
				filter.Pagination.Limit = 100000
			}
			_, ret, err := yakit.QueryYakScript(s.GetProfileDatabase(), filter)
			if err != nil {
				return err
			}
			for _, rawModel := range ret {
				rsp = append(rsp, rawModel.ToGRPCModel())
			}
		}
	} else {
		// 不启动过滤器就走原来的流程
		if len(req.GetScriptNames()) > 0 {
			for _, y := range yakit.QueryYakScriptByNames(s.GetProfileDatabase(), req.GetScriptNames()...) {
				rsp = append(rsp, y.ToGRPCModel())
			}
		} else {
			filterReq := &ypb.QueryYakScriptRequest{
				Pagination: &ypb.Paging{
					Page:    1,
					Limit:   req.Limit,
					OrderBy: "updated_at",
					Order:   "desc",
				},
				Type:                  req.Type,
				Keyword:               req.Keyword,
				ExcludeNucleiWorkflow: req.GetDisableNucleiWorkflow(),
				ExcludeScriptNames:    req.GetExcludedYakScript(),
			}
			var rspRaw *ypb.QueryYakScriptResponse
			rspRaw, err = s.QueryYakScript(ctx, filterReq)
			if err != nil {
				return utils.Errorf("fetch current yak module failed: %s", err)
			}
			rsp = rspRaw.Data
		}
	}

	swg := utils.NewSizedWaitGroup(int(req.Concurrent))
	defer swg.Wait()
	totalYakScript := len(rsp)
	if totalYakScript <= 0 {
		return utils.Errorf("ERROR loading Plugins... %v", "empty")
	}

	if groupSize <= 0 {
		groupSize = 5
	}
	yakScriptGroups := funk.Chunk(rsp, int(groupSize)).([][]*ypb.YakScript)
	groupScriptTotal := len(yakScriptGroups)

	/*
		加载目标
	*/
	var targets []string
	var targetRaw = req.GetTarget()
	fileContentRaw, _ := ioutil.ReadFile(req.GetTargetFile())
	if fileContentRaw != nil {
		targetRaw += "\n"
		targetRaw += string(fileContentRaw)
	}
	targets = utils.PrettifyListFromStringSplited(targetRaw, "\n")
	targets = utils.PrettifyListFromStringSplited(strings.Join(targets, ","), ",")
	targets = mutate.QuickMutateSimple(targets...)

	progressTotal := groupScriptTotal * len(targets)
	var progressCount int64
	var progressRunning int64
	var scanTaskExecutingCount int64

	// 开始在这里准备保存结果
	var lastScripts []string
	var lastScriptLock = new(sync.Mutex)
	addLastScript := func(s ...*ypb.YakScript) {
		lastScriptLock.Lock()
		defer lastScriptLock.Unlock()
		for _, i := range s {
			lastScripts = append(lastScripts, i.ScriptName)
		}
	}
	baseProgress := req.GetBaseProgress()
	if !(baseProgress > 0 && baseProgress < 1) {
		baseProgress = 0.1
	}
	getPercent := func() float64 {
		return baseProgress + (float64(progressCount)/float64(progressTotal))*(1-baseProgress)
	}
	var yakScriptOnlineGroup, taskName string
	if req.YakScriptOnlineGroup != "" {
		yakScriptOnlineGroup = req.YakScriptOnlineGroup
	}
	if req.TaskName != "" {
		taskName = req.TaskName
	}

	defer func() {
		// 如果推出的时候，last Script/Targets 都不为空，说明有一些没有完成的任务，
		if len(lastScripts) <= 0 {
			return
		}
		uid := uuid.NewV4().String()
		manager.AddExecBatchTaskToPool(uid, getPercent(), yakScriptOnlineGroup, taskName, &ypb.ExecBatchYakScriptRequest{
			Target:                req.Target,
			ExtraParams:           req.ExtraParams,
			Keyword:               req.Keyword,
			ExcludedYakScript:     req.ExcludedYakScript,
			DisableNucleiWorkflow: req.DisableNucleiWorkflow,
			Limit:                 req.Limit,
			TotalTimeoutSeconds:   req.TotalTimeoutSeconds,
			Type:                  req.Type,
			Concurrent:            req.Concurrent,
			ScriptNames:           lastScripts,
			EnablePluginFilter:    false,
			YakScriptOnlineGroup:  yakScriptOnlineGroup,
			TaskName:              taskName,
		})
	}()

	/*
		执行任务的核心函数在下面，我们在下面内容控制分组与进程的对应关系
	*/
	_ = stream.Send(&ypb.ExecBatchYakScriptResult{
		ProgressMessage: true, ProgressPercent: 0.1,
		ProgressCount: 0, ProgressTotal: int64(progressTotal),
		Timestamp: time.Now().Unix(),
	})
	sendStatus := func() {
		_ = stream.Send(&ypb.ExecBatchYakScriptResult{
			ProgressMessage:        true,
			ProgressPercent:        getPercent(),
			ProgressCount:          progressCount,
			ProgressTotal:          int64(progressTotal),
			ProgressRunning:        progressRunning,
			ScanTaskExecutingCount: scanTaskExecutingCount,
			Timestamp:              time.Now().Unix(),
		})
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				sendStatus()
			}
			time.Sleep(time.Second)
		}
	}()
	for _, scriptGroup := range yakScriptGroups {
		if len(targets) == 0 {
			for _, script := range scriptGroup {
				stream.Send(&ypb.ExecBatchYakScriptResult{
					Id:        script.ScriptName,
					Status:    "waiting",
					PoC:       script,
					Timestamp: time.Now().Unix(),
				})
			}
			continue
		}

		scriptGroup := scriptGroup
		select {
		case <-ctx.Done():
			addLastScript(scriptGroup...)
			continue
		default:
		}

		for _, target := range targets {
			target := target
			scanTaskCount := 1 * len(scriptGroup)
			err := swg.AddWithContext(ctx)
			if err != nil {
				continue
			}

			go func() {
				atomic.AddInt64(&progressRunning, 1)
				atomic.AddInt64(&scanTaskExecutingCount, int64(scanTaskCount))
				defer swg.Done()
				defer func() {
					atomic.AddInt64(&progressCount, 1)
					atomic.AddInt64(&progressRunning, -1)
					atomic.AddInt64(&scanTaskExecutingCount, -int64(scanTaskCount))
					sendStatus()
				}()

				taskId := uuid.NewV4().String() // codec.Sha512(target + script.ScriptName + fmt.Sprint(script.Id))

				/**
				这儿需要小心点处理：

				批量执行将不再建议使用 “进程”

				通过上下文控制即可。

				不同插件的调用方式和目标处理都不一样，一个插件组可能有不同的插件类型，因此需要写一个脚本同时调用 N 种插件
				*/
				var templates []string
				var ordinaries []string
				for _, i := range scriptGroup {
					switch i.GetType() {
					case "nuclei", "nuclei-templates":
						templates = append(templates, i.ScriptName)
					default:
						ordinaries = append(ordinaries, i.ScriptName)
					}
				}

				engine := yak.NewScriptEngine(10)
				subCtx, cancel := context.WithTimeout(ctx, time.Duration(int64(30*time.Second)*int64(len(scriptGroup))))
				defer cancel()

				//params, defers, err := ConvertMultiYakScriptToExecBatchRequest(&ypb.ExecRequest{
				//	Params: append(extraParams, &ypb.ExecParamItem{Key: "target", Value: target}),
				//}, scriptGroup, true)
				//defer func() {
				//	for _, r := range defers {
				//		r()
				//	}
				//}()
				//if err != nil {
				//	log.Errorf("generate exec request params failed: %s", err)
				//	return
				//}

				stream.Send(&ypb.ExecBatchYakScriptResult{
					Status: "data",
					Result: yaklib.NewYakitLogExecResult(
						"info", fmt.Sprintf("正在启动对 %v 的扫描子进程，插件进程数：[%v]", target, len(scriptGroup)),
					),
					Target:     target,
					ExtraParam: extraParams,
					TaskId:     taskId,
					Timestamp:  time.Now().Unix(),
				})
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

				//// 启动一个带上下文引擎的内容
				//err = s.execRequest(params, `general-batch`, subCtx, func(result *ypb.ExecResult, logItem *yaklib.YakitLog) error {
				//	if logItem == nil {
				//		return nil
				//	}
				//
				//	stream.Send(&ypb.ExecBatchYakScriptResult{
				//		Status:     "data",
				//		Result:     result,
				//		Target:     target,
				//		ExtraParam: extraParams,
				//		TaskId:     taskId,
				//		Timestamp:  time.Now().Unix(),
				//	})
				//	return nil
				//}, ioutil.Discard)

				var logToExecResult = func(l *yaklib.YakitLog) *ypb.ExecResult {
					return yaklib.NewYakitLogExecResult(l.Level, l.Data)
				}
				var feedbackClient = yaklib.NewVirtualYakitClient(func(i interface{}) error {
					switch ret := i.(type) {
					case *ypb.ExecResult:
						stream.Send(&ypb.ExecBatchYakScriptResult{
							Status:     "data",
							Result:     ret,
							Target:     target,
							ExtraParam: extraParams,
							TaskId:     taskId,
							Timestamp:  time.Now().Unix(),
						})
					case *yaklib.YakitLog:
						stream.Send(&ypb.ExecBatchYakScriptResult{
							Status:     "data",
							Result:     logToExecResult(ret),
							Target:     target,
							ExtraParam: extraParams,
							TaskId:     taskId,
							Timestamp:  time.Now().Unix(),
						})
					}
					return nil
				})
				engine.HookOsExit()
				engine.RegisterEngineHooksLegacy(func(engine yaklang.YaklangEngine) error {
					switch ret := engine.(type) {
					case *antlr4yak.Engine:
						yaklib.SetEngineClient(ret, feedbackClient)
					}
					return nil
				})
				coreEngine, err := engine.ExecuteExWithContext(subCtx, string(batchExecScripts), map[string]interface{}{
					"target":      target,
					"templates":   templates,
					"ordinary":    ordinaries,
					"ctx":         subCtx,
					"yakitclient": feedbackClient,
				})
				if err != nil {
					log.Errorf("execute batch exec script failed: %s", err)
				}
				_ = coreEngine

				defer func() {
					time.Sleep(time.Duration(rand.Intn(3000)) * time.Millisecond)
					stream.Send(&ypb.ExecBatchYakScriptResult{
						Status:     "end",
						Target:     target,
						ExtraParam: extraParams,
						TaskId:     taskId,
						Timestamp:  time.Now().Unix(),
					})
				}()
				if err != nil {
					log.Errorf("exec for [%v] error: %s", req.Target, err)
					stream.Send(&ypb.ExecBatchYakScriptResult{
						Target:     target,
						ExtraParam: extraParams,
						Timestamp:  time.Now().Unix(),
						TaskId:     taskId,
						Status:     "data",
						Ok:         false,
						Reason:     err.Error(),
						Result:     yaklib.NewYakitLogExecResult("error", err.Error()),
					})
					select {
					case <-ctx.Done():
						addLastScript(scriptGroup...)
					default:
					}
					return
				}
			}()
		}

	}
	return nil
}

func ConvertMultiYakScriptToExecBatchRequest(req *ypb.ExecRequest, script []*ypb.YakScript, batchMode bool) (*ypb.ExecRequest, []func(), error) {
	if len(script) <= 0 {
		return nil, nil, utils.Error("empty yakScripts")
	}
	var defers []func()

	var plugins []string
	script = funk.Filter(script, func(i *ypb.YakScript) bool {
		result := utils.StringSliceContain([]string{
			"mitm", "port-scan", "nuclei", "nasl",
		}, i.Type)
		if result {
			plugins = append(plugins, i.ScriptName)
		}
		return result
	}).([]*ypb.YakScript)

	if len(plugins) <= 0 {
		return nil, nil, utils.Error("no available plugins")
	}

	if len(plugins) == 1 {
		var params = append(req.Params, &ypb.ExecParamItem{Key: "--plugin", Value: plugins[0]})
		return &ypb.ExecRequest{
			Params: params,
			Script: generalBatchExecutor,
		}, defers, nil
	}

	f, err := consts.TempFile("exec-batch-plugin-list-*.txt")
	if err != nil {
		return nil, nil, err
	}
	defers = append(defers, func() {
		os.RemoveAll(f.Name())
	})
	f.WriteString(strings.Join(plugins, "|"))
	f.Close()
	var params = append(req.Params, &ypb.ExecParamItem{Key: "--yakit-plugin-file", Value: f.Name()})
	return &ypb.ExecRequest{Params: params, Script: generalBatchExecutor}, defers, nil
}

func (s *Server) RecoverExecBatchYakScriptUnfinishedTask(req *ypb.RecoverExecBatchYakScriptUnfinishedTaskRequest, stream ypb.Yak_RecoverExecBatchYakScriptUnfinishedTaskServer) error {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqTask, err := manager.GetProgressByUid(req.GetUid(), true)
	if err != nil {
		return utils.Errorf("recover request by uid[%s] failed: %s", req.GetUid(), err)
	}
	return s.ExecBatchYakScript(reqTask, stream)
}
