package yakgrpc

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"sync"
	"time"
)

func (s *Server) hybridScanResume(manager *HybridScanTaskManager, stream HybridScanRequestStream) error {
	task, err := yakit.GetHybridScanByTaskId(s.GetProjectDatabase(), manager.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume HybridScanByID: %v", manager.TaskId())
	}
	quickSave := func() {
		err := yakit.SaveHybridScanTask(consts.GetGormProjectDatabase(), task)
		if err != nil {
			log.Error(err)
		}
	}

	task.Status = yakit.HYBRIDSCAN_EXECUTING
	quickSave()

	// recover scanConfig
	var scanConfig ypb.HybridScanRequest
	err = json.Unmarshal(task.ScanConfig, &scanConfig)
	if err != nil {
		return utils.Wrapf(err, "Resume HybridScanByID: %v", manager.TaskId())
	}

	defer func() {
		if err := recover(); err != nil {
			task.Reason = utils.Wrapf(utils.Error(err), "PANIC from resume").Error()
			task.Status = yakit.HYBRIDSCAN_ERROR
			quickSave()
			return
		}

		if task.Status == yakit.HYBRIDSCAN_PAUSED {
			quickSave()
			return
		}
		task.Status = yakit.HYBRIDSCAN_DONE
		quickSave()
	}()

	var hashMap = make(map[int64]struct{})
	var minIndex int64 = -1
	var maxIndex int64 = 0
	// string to int
	for _, val := range utils.ParseStringToPorts(task.SurvivalTaskIndexes) {
		val := int64(val)
		hashMap[val] = struct{}{}
		if minIndex == -1 {
			minIndex = val
		} else {
			if val < minIndex {
				minIndex = val
			}
		}
		if val > maxIndex {
			maxIndex = val
		}
	}

	var targets []*HybridScanTarget
	var pluginName []string
	err = json.Unmarshal([]byte(task.Targets), &targets)
	if err != nil {
		return utils.Wrapf(err, "Unmarshal HybridScan Targets: %v", task.Targets)
	}
	err = json.Unmarshal([]byte(task.Plugins), &pluginName)
	if err != nil {
		return utils.Wrapf(err, "Unmarshal HybridScan Plugins: %v", task.Plugins)
	}

	statusManager := newHybridScanStatusManager(task.TaskId, len(targets), len(pluginName), task.Status)
	setTaskStatus := func(status string) {
		task.Status = status
		statusManager.Status = status
	}
	statusManager.SetCurrentTaskIndex(minIndex)

	pluginCacheList := list.New()
	feedbackStatus := func() {
		statusManager.Feedback(stream)
	}

	swg := utils.NewSizedWaitGroup(int(scanConfig.Concurrent))                                                                     // 设置并发数
	manager.ctx, manager.cancel = context.WithTimeout(manager.Context(), time.Duration(scanConfig.TotalTimeoutSecond)*time.Second) // 设置总超时
	// init some config
	var resumeFilterManager = filter.NewFilterManager(12, 1<<15, 30)
	var hasUnavailableTarget = false

	countRiskClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		result.RuntimeID = task.TaskId
		currentStatus := statusManager.GetStatus(task)
		currentStatus.ExecResult = result
		return stream.Send(currentStatus)
	})
	s.tickerRiskCountFeedback(manager.Context(), 2*time.Second, task.TaskId, countRiskClient)
	defer s.forceRiskCountFeedback(task.TaskId, countRiskClient)

	matcher, err := fp.NewDefaultFingerprintMatcher(fp.NewConfig(fp.WithDatabaseCache(true), fp.WithCache(true)))
	if err != nil {
		return utils.Wrap(err, "init fingerprint matcher failed")
	}

	go func() { // 监听控制信号
		for {
			rsp, err := stream.Recv()
			if err != nil {
				task.Reason = err.Error()
				return
			}
			if rsp.HybridScanMode == "pause" {
				setTaskStatus(yakit.HYBRIDSCAN_PAUSED)
				feedbackStatus()
				manager.Pause()
				manager.Stop()
				quickSave()
			}
		}
	}()

	// dispatch
	for _, __currentTarget := range targets {
		if manager.IsStop() || manager.IsPaused() { // if cancel, return immediately
			break
		}
		statusManager.DoActiveTarget()
		feedbackStatus()
		targetWg := new(sync.WaitGroup)

		conn, err := netx.DialX(utils.ExtractHostPort(__currentTarget.Url))
		if err != nil {
			log.Errorf("dial target failed: %s", err)
			hasUnavailableTarget = true
			statusManager.DoneTarget()
			feedbackStatus()
			continue
		}
		conn.Close()

		// check can use mitm
		skipMitm := false
		resp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(__currentTarget.Request), lowhttp.WithHttps(__currentTarget.IsHttps), lowhttp.WithRuntimeId(task.TaskId), lowhttp.WithProxy(scanConfig.Proxy))
		if err != nil {
			skipMitm = true
		}
		__currentTarget.Response = resp.RawPacket

		// fingerprint match just once
		var portScanCond = &sync.Cond{L: &sync.Mutex{}}
		var fingerprintMatchOK = false
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

		// load plugins
		pluginChan, err := s.PluginGenerator(pluginCacheList, manager.Context(), &ypb.HybridScanPluginConfig{PluginNames: pluginName})
		if err != nil {
			return utils.Wrapf(err, "PluginGenerator")
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

			taskIndex := statusManager.DoActiveTask(task)
			feedbackStatus()

			if __pluginInstance.Type == "mitm" && skipMitm {
				log.Debugf("skip mitm plugin: %s", __pluginInstance.ScriptName)
				statusManager.DoneTask(taskIndex, task)
				statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
				feedbackStatus()
				targetWg.Done()
				continue
			}

			for !fingerprintMatchOK { // wait for fingerprint match
				portScanCond.L.Lock()
				portScanCond.Wait()
				portScanCond.L.Unlock()
			}

			go func() {
				defer swg.Done()
				defer targetWg.Done()
				defer func() {
					if !manager.IsStop() { // 暂停之后不再更新进度
						statusManager.DoneTask(taskIndex, task)
					}
					statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
					feedbackStatus()
				}()

				// shrink context
				if manager.IsStop() {
					log.Infof("skip task %d via canceled", taskIndex)
					return
				}

				statusManager.PushActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)

				// 过滤执行过的任务
				// 小于最小索引的任务，直接跳过
				// 大于最大索引的任务，直接执行
				// 在最小索引和最大索引之间的任务，如果没有执行过，执行
				if taskIndex < minIndex {
					return
				} else if taskIndex >= minIndex && taskIndex <= maxIndex {
					_, ok := hashMap[taskIndex]
					if !ok {
						return
					}
				}

				callerFilter := resumeFilterManager.DequeueFilter()
				defer resumeFilterManager.EnqueueFilter(callerFilter)

				feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
					// shrink context
					if manager.IsStop() {
						return nil
					}

					result.RuntimeID = task.TaskId
					currentStatus := statusManager.GetStatus(task)
					currentStatus.CurrentPluginName = pluginInstance.ScriptName
					currentStatus.ExecResult = result
					return stream.Send(currentStatus)
				}, task.TaskId)

				err := ScanHybridTargetWithPlugin(task.TaskId, manager.Context(), targetRequestInstance, pluginInstance, scanConfig.Proxy, feedbackClient, callerFilter)
				if err != nil {
					log.Warnf("scan target failed: %s", err)
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
			if manager.IsStop() { //停止之后不再 更新进度
				statusManager.DoneTarget()
				feedbackStatus()
			}
		}()
	}
	swg.Wait()
	statusManager.GetStatus(task)
	if !manager.IsPaused() {
		setTaskStatus(yakit.HYBRIDSCAN_DONE)
	}
	feedbackStatus()
	quickSave()
	if hasUnavailableTarget {
		return utils.Errorf("Has unreachable target")
	}
	return nil
}
