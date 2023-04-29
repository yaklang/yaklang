package scannode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"sync"
	"time"
	"yaklang/common/crep"
	"yaklang/common/fp"
	"yaklang/common/log"
	"yaklang/common/mq"
	"yaklang/common/spec"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklang"
	"yaklang/scannode/scanrpc"
)

type TaskManager struct {
	tasks *sync.Map
}

func GetPalmHomeDir() string {
	return filepath.Join(utils.GetHomeDirDefault("/tmp/"), ".palm-desktop")
}

func (t *TaskManager) Add(taskId string, task *Task) {
	task.StartTimestamp = time.Now().Unix()
	ddl, ok := task.Ctx.Deadline()
	if ok {
		task.DeadlineTimestamp = ddl.Unix()
	}
	t.tasks.Store(taskId, task)
}

func (t *TaskManager) Remove(taskId string) {
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

func (t *TaskManager) GetTaskById(taskId string) (*Task, error) {
	ins, ok := t.tasks.Load(taskId)
	if ok {
		return ins.(*Task), nil
	}
	return nil, utils.Errorf("no existed task: %s", taskId)
}

type Task struct {
	TaskType          string
	TaskId            string
	Ctx               context.Context
	Cancel            context.CancelFunc
	StartTimestamp    int64
	DeadlineTimestamp int64
}

func (s *ScanNode) initScanRPC() {
	scanHelper := scanrpc.NewSCANServerHelper()
	s.helper = scanHelper

	manager := &TaskManager{tasks: new(sync.Map)}
	s.manager = manager

	scanHelper.DoSCAN_ScanFingerprint = func(ctx context.Context, node string, req *scanrpc.SCAN_ScanFingerprintRequest, broker *mq.Broker) (*scanrpc.SCAN_ScanFingerprintResponse, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		taskId := fmt.Sprintf("scan-fingerprint-[H:%v P:%v]-[%v]", req.Hosts, req.Ports, uuid.NewV4().String())
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
		taskId := fmt.Sprintf("proxy-collector-[Port:%v]-[%v]", req.Port, uuid.NewV4().String())
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
		tasks := manager.All()
		var ret []*scanrpc.Task
		for _, r := range tasks {
			ret = append(ret, &scanrpc.Task{
				TaskID:            r.TaskId,
				TaskType:          r.TaskType,
				StartTimestamp:    r.StartTimestamp,
				DeadlineTimestamp: r.DeadlineTimestamp,
			})
		}
		return &scanrpc.SCAN_GetRunningTasksResponse{Tasks: ret}, nil
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

	s.node.GetRPCServer().RegisterServices(scanrpc.MethodList, scanHelper.Do)
}

func (s *ScanNode) _scriptEngineHook(engine yaklang.YaklangEngine) error {
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

	log.Infof("scanner feedback data: %v", msg.Type)
	s.node.Notify(
		spec.BackendKey_Scanner,
		msg,
	)
}

func toHttpRequest(method, url string, body []byte, headers *http.Header) ([]byte, error) {
	r, err := http.NewRequest(
		method,
		url,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, err
	}

	headers.Set("Content-Length", fmt.Sprint(len(body)))
	for header, values := range *headers {
		for _, v := range values {
			r.Header.Set(header, v)
		}
	}
	return httputil.DumpRequest(r, true)
}
