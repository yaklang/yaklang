package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/scannode/scanrpc"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type TaskManager struct {
	tasks    *sync.Map
	recentMu sync.Mutex
	recent   []*taskRecentRecord
}

type taskRecentRecord struct {
	FinishedTimestamp int64
	WaitMs            int64
	ExecMs            int64
}

func GetPalmHomeDir() string {
	return filepath.Join(utils.GetHomeDirDefault("/tmp/"), ".palm-desktop")
}

func (t *TaskManager) Add(taskId string, task *Task) {
	now := time.Now().Unix()
	task.mu.Lock()
	task.StartTimestamp = now
	ddl, ok := task.Ctx.Deadline()
	if ok {
		task.DeadlineTimestamp = ddl.Unix()
	}
	if task.Status == "" {
		task.Status = "running"
	}
	if task.Status == "running" && task.RunningTimestamp == 0 {
		task.RunningTimestamp = now
	}
	task.mu.Unlock()
	t.tasks.Store(taskId, task)
}

func (t *TaskManager) Remove(taskId string) {
	if raw, ok := t.tasks.Load(taskId); ok {
		task := raw.(*Task)
		now := time.Now().Unix()
		task.mu.RLock()
		waitMs := task.WaitMs
		runningTs := task.RunningTimestamp
		task.mu.RUnlock()
		execMs := int64(0)
		if runningTs > 0 {
			execMs = now*1000 - runningTs*1000
		}
		t.recordRecent(&taskRecentRecord{
			FinishedTimestamp: now,
			WaitMs:            waitMs,
			ExecMs:            execMs,
		})
	}
	t.tasks.Delete(taskId)
}

func (t *TaskManager) All() []*Task {
	var tasks []*Task
	t.tasks.Range(func(key, value interface{}) bool {
		tasks = append(tasks, value.(*Task))
		return true
	})
	return tasks
}

func (t *TaskManager) MarkRunning(taskId string, waitMs int64) {
	ins, ok := t.tasks.Load(taskId)
	if !ok {
		return
	}
	task := ins.(*Task)
	task.mu.Lock()
	task.Status = "running"
	task.WaitMs = waitMs
	task.RunningTimestamp = time.Now().Unix()
	task.mu.Unlock()
}

func (t *TaskManager) Snapshot(capacity int) ([]*scanrpc.Task, int, int, int64, int64, int64) {
	var (
		activeCount int
		queueCount  int
		ret         []*scanrpc.Task
	)
	t.tasks.Range(func(_, value interface{}) bool {
		task := value.(*Task)
		snap := task.snapshotRPC()
		ret = append(ret, snap)
		switch snap.Status {
		case "queued":
			queueCount++
		default:
			activeCount++
		}
		return true
	})
	recentAvgWaitMs, recentAvgExecMs, recentCompletedCount := t.recentStats(15 * time.Minute)
	return ret, activeCount, queueCount, recentAvgWaitMs, recentAvgExecMs, recentCompletedCount
}

func (t *TaskManager) recordRecent(record *taskRecentRecord) {
	if record == nil {
		return
	}
	t.recentMu.Lock()
	defer t.recentMu.Unlock()
	t.recent = append(t.recent, record)
	cutoff := time.Now().Add(-30 * time.Minute).Unix()
	trimIndex := 0
	for trimIndex < len(t.recent) && t.recent[trimIndex].FinishedTimestamp < cutoff {
		trimIndex++
	}
	if trimIndex > 0 {
		t.recent = append([]*taskRecentRecord(nil), t.recent[trimIndex:]...)
	}
}

func (t *TaskManager) recentStats(window time.Duration) (int64, int64, int64) {
	t.recentMu.Lock()
	defer t.recentMu.Unlock()
	if len(t.recent) == 0 {
		return 0, 0, 0
	}
	cutoff := time.Now().Add(-window).Unix()
	var (
		totalWait int64
		totalExec int64
		count     int64
	)
	for _, item := range t.recent {
		if item == nil || item.FinishedTimestamp < cutoff {
			continue
		}
		totalWait += item.WaitMs
		totalExec += item.ExecMs
		count++
	}
	if count == 0 {
		return 0, 0, 0
	}
	return totalWait / count, totalExec / count, count
}

func (t *TaskManager) GetTaskById(taskId string) (*Task, error) {
	ins, ok := t.tasks.Load(taskId)
	if ok {
		return ins.(*Task), nil
	}
	return nil, utils.Errorf("no existed task: %s", taskId)
}

type Task struct {
	mu                sync.RWMutex
	TaskType          string
	TaskId            string
	RootTaskID        string
	SubTaskID         string
	RuntimeID         string
	Status            string
	WaitMs            int64
	Ctx               context.Context
	Cancel            context.CancelFunc
	StartTimestamp    int64
	RunningTimestamp  int64
	DeadlineTimestamp int64
}

func (t *Task) snapshotRPC() *scanrpc.Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return &scanrpc.Task{
		TaskID:            t.TaskId,
		RootTaskID:        t.RootTaskID,
		SubTaskID:         t.SubTaskID,
		RuntimeID:         t.RuntimeID,
		TaskType:          t.TaskType,
		Status:            t.Status,
		WaitMs:            t.WaitMs,
		StartTimestamp:    t.StartTimestamp,
		RunningTimestamp:  t.RunningTimestamp,
		DeadlineTimestamp: t.DeadlineTimestamp,
	}
}

func (s *ScanNode) initScanRPC() {
	scanHelper := scanrpc.NewSCANServerHelper()
	s.helper = scanHelper

	manager := &TaskManager{tasks: new(sync.Map)}
	s.manager = manager

	scanHelper.DoSCAN_ScanFingerprint = func(ctx context.Context, node string, req *scanrpc.SCAN_ScanFingerprintRequest, broker *mq.Broker) (*scanrpc.SCAN_ScanFingerprintResponse, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		taskId := fmt.Sprintf("scan-fingerprint-[H:%v P:%v]-[%v]", req.Hosts, req.Ports, uuid.New().String())
		manager.Add(taskId, &Task{
			TaskType: "scan-fingerprint",
			TaskId:   taskId,
			Ctx:      ctx,
			Cancel:   cancel,
		})
		defer manager.Remove(taskId)
		log.Infof("create scan-fingerprint task: %s", taskId)

		matcher, err := fp.NewDefaultFingerprintMatcher(nil)
		if err != nil {
			return nil, err
		}

		swg := utils.NewSizedWaitGroup(req.Concurrent)

		var closedMutex = new(sync.Mutex)
		var closedPorts = map[string]string{}
		var closedCount int
		var haveResult = utils.NewBool(false)

		var extraOptions []fp.ConfigOption
		if req.IsUDP {
			extraOptions = append(extraOptions, fp.WithTransportProtos(fp.UDP))
		} else {
			extraOptions = append(extraOptions, fp.WithTransportProtos(fp.TCP))
		}
		extraOptions = append(extraOptions, fp.WithProbeTimeout(time.Duration(req.TimeoutSeconds)*time.Second))

		for _, host := range utils.ParseStringToHosts(req.Hosts) {
			for _, port := range utils.ParseStringToPorts(req.Ports) {
				err := swg.AddWithContext(ctx)
				if err != nil {
					return nil, utils.Errorf("context done")
				}

				host := host
				port := port

				go func() {
					defer swg.Done()

					log.Infof("scan host: %v, port: %v", host, port)
					result, err := matcher.Match(host, port, extraOptions...)
					if err != nil {
						log.Errorf("match result failed: %s", err)
						return
					}
					switch result.State {
					case fp.OPEN:
						haveResult.Set()
						result, err := spec.NewScanFingerprintResult(result)
						if err != nil {
							return
						}
						s.feedback(result)
					default:
						closedMutex.Lock()
						closedCount++
						if len(closedPorts) <= 10 {
							closedPorts[fmt.Sprintf("%v://%v:%v", "tcp", result.Target, result.Port)] = result.Reason
						}
						closedMutex.Unlock()
					}
				}()
			}
		}
		swg.Wait()

		if closedCount > 0 && !haveResult.IsSet() {
			if len(closedPorts) <= 5 {
				var msg []string
				for key, reason := range closedPorts {
					msg = append(msg, fmt.Sprintf("%v: %s", key, reason))
				}
				return nil, utils.Errorf("closed ports count: %v\n%v\n", closedCount, msg)
			}
			return nil, utils.Errorf("closed ports count: %v", closedCount)
		}

		return &scanrpc.SCAN_ScanFingerprintResponse{}, nil
	}
	scanHelper.DoSCAN_ProxyCollector = func(ctx context.Context, node string, req *scanrpc.SCAN_ProxyCollectorRequest, broker *mq.Broker) (*scanrpc.SCAN_ProxyCollectorResponse, error) {
		ctx, cancel := context.WithCancel(ctx)
		taskId := fmt.Sprintf("proxy-collector-[Port:%v]-[%v]", req.Port, uuid.New().String())
		manager.Add(taskId, &Task{
			TaskType: "proxy-collector",
			TaskId:   taskId,
			Ctx:      ctx,
			Cancel:   cancel,
		})
		defer manager.Remove(taskId)

		server, err := crep.NewMITMServer(
			crep.MITM_SetHTTPResponseMirror(s.feedbackHttpFlow),
		)
		if err != nil {
			return nil, err
		}
		err = server.Serve(ctx, utils.HostPort("0.0.0.0", req.Port))
		if err != nil {
			return nil, err
		}
		return &scanrpc.SCAN_ProxyCollectorResponse{}, nil
	}
	// task api
	scanHelper.DoSCAN_GetRunningTasks = func(ctx context.Context, node string, req *scanrpc.SCAN_GetRunningTasksRequest, broker *mq.Broker) (*scanrpc.SCAN_GetRunningTasksResponse, error) {
		ret, activeCount, queueCount, recentAvgWaitMs, recentAvgExecMs, recentCompletedCount := manager.Snapshot(s.invokeLimiter.capacity())
		return &scanrpc.SCAN_GetRunningTasksResponse{
			Tasks:                ret,
			ActiveCount:          activeCount,
			QueueCount:           queueCount,
			Capacity:             s.invokeLimiter.capacity(),
			RecentAvgWaitMs:      recentAvgWaitMs,
			RecentAvgExecMs:      recentAvgExecMs,
			RecentCompletedCount: recentCompletedCount,
		}, nil
	}
	scanHelper.DoSCAN_StopTask = func(ctx context.Context, node string, req *scanrpc.SCAN_StopTaskRequest, broker *mq.Broker) (*scanrpc.SCAN_StopTaskResponse, error) {
		t, err := manager.GetTaskById(req.TaskId)
		if err != nil {
			return nil, err
		}
		t.Cancel()
		return &scanrpc.SCAN_StopTaskResponse{}, nil
	}
	scanHelper.DoSCAN_InvokeScript = s.rpc_invokeScript
	scanHelper.DoSCAN_StartScript = s.rpc_startScript
	scanHelper.DoSCAN_QueryYakScript = s.rpcQueryYakScript

	s.node.GetRPCServer().RegisterServices(scanrpc.MethodList, scanHelper.Do)
}

func (s *ScanNode) _scriptEngineHook(engine *antlr4yak.Engine) error {
	return nil
}

func (s *ScanNode) feedbackHttpFlow(isHttps bool, u string, req *http.Request, rsp *http.Response, _ string) {
	result, err := spec.NewHTTPFlowScanResult(isHttps, req, rsp)
	if err != nil {
		return
	}
	s.feedback(result)
}

func (s *ScanNode) feedbackVuln(v *Vuln) {
	result, err := NewVulnResult(v)
	if err != nil {
		return
	}
	s.feedback(result)
}

func NewVulnResult(v *Vuln) (*spec.ScanResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &spec.ScanResult{
		Type:    spec.ScanResult_Vuln,
		Content: raw,
	}, nil
}

func (s *ScanNode) feedback(result *spec.ScanResult) {
	msg := s.node.NewBaseMessage(spec.MessageType_Scanner)
	raw, err := json.Marshal(result)
	if err != nil {
		log.Errorf("marshal scan result failed: %s", err)
		return
	}
	msg.Content = raw
	atomic.AddUint64(&s.feedbackCount, 1)
	atomic.AddUint64(&s.feedbackBytes, uint64(len(raw)))
	s.recordTaskStat(result.TaskId, len(raw))
	if result.Type == spec.ScanResult_Vuln {
		atomic.AddUint64(&s.feedbackVulnCount, 1)
		atomic.AddUint64(&s.feedbackVulnBytes, uint64(len(raw)))
		s.recordTaskVuln(result.TaskId, len(raw))
	}

	log.Debugf("scanner feedback data: %v", msg.Type)
	s.node.Notify(
		spec.BackendKey_Scanner,
		msg,
	)
}
