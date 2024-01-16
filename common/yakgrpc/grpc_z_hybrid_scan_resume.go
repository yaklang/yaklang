package yakgrpc

import (
	"container/list"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
		if consts.GetGormProjectDatabase().Save(task).Error != nil {
			log.Error(err)
		}
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

	statusManager := newHybridScanStatusManager(task.TaskId, len(targets), len(pluginName))
	statusManager.SetCurrentTaskIndex(minIndex)

	pluginCacheList := list.New()
	feedbackStatus := func() {
		statusManager.Feedback(stream)
	}

	swg := utils.NewSizedWaitGroup(20)

	// dispatch
	for _, __currentTarget := range targets {
		statusManager.DoActiveTarget()
		feedbackStatus()

		// load plugins
		pluginChan, err := s.PluginGenerator(pluginCacheList, manager.Context(), &ypb.HybridScanPluginConfig{PluginNames: pluginName})
		if err != nil {
			return utils.Wrapf(err, "PluginGenerator")
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
				task.SurvivalTaskIndexes = utils.ConcatPorts(statusManager.GetCurrentActiveTaskIndexes())
				feedbackStatus()
				task.Status = yakit.HYBRIDSCAN_PAUSED
				quickSave()
			})

			taskIndex := statusManager.DoActiveTask()

			feedbackStatus()

			go func() {
				defer swg.Done()

				defer targetWg.Done()
				defer func() {
					statusManager.DoneTask(taskIndex)
					feedbackStatus()
					statusManager.RemoveActiveTask(taskIndex, targetRequestInstance, pluginInstance.ScriptName, stream)
				}()

				// shrink context
				select {
				case <-manager.Context().Done():
					log.Infof("skip task %d via canceled", taskIndex)
					return
				default:
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

				err := s.ScanTargetWithPlugin(task.TaskId, manager.Context(), targetRequestInstance, pluginInstance, func(result *ypb.ExecResult) {
					// shrink context
					select {
					case <-manager.Context().Done():
						return
					default:
					}

					result.RuntimeID = task.TaskId
					status := statusManager.GetStatus()
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
			return utils.Error("task manager canceled")
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
		task.Status = yakit.HYBRIDSCAN_DONE
	}
	return nil
}
