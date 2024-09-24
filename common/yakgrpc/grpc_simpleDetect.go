package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/network"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed grpc_simple_detect_script.yak
var simpleDetect []byte

func (s *Server) SimpleDetect(req *ypb.RecordPortScanRequest, stream ypb.Yak_SimpleDetectServer) error {
	reqParams := &ypb.DebugPluginRequest{
		Code:       string(simpleDetect),
		PluginType: "yak",
		RuntimeId:  req.GetRuntimeId(),
	}

	reqRecord := req.LastRecord
	reqPortScan := req.PortScanRequest
	reqBrute := req.StartBruteParams

	if req.PortScanRequest.GetSkipCveBaseLine() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "skip-cve-baseline",
		})
	}

	// 把文件写到本地。
	tmpTargetFile, err := ioutil.TempFile("", "yakit-portscan-*.txt")
	if err != nil {
		return utils.Errorf("create temp target file failed: %s", err)
	}
	var targetsLineFromFile []string
	filePaths := utils.PrettifyListFromStringSplited(reqPortScan.GetTargetsFile(), ",")
	for _, filePath := range filePaths {
		raw, _ := ioutil.ReadFile(filePath)
		targetsLineFromFile = append(targetsLineFromFile, utils.PrettifyListFromStringSplited(string(raw), "\n")...)
	}

	targetsLine := utils.PrettifyListFromStringSplited(reqPortScan.GetTargets(), "\n")
	targets := append(targetsLine, targetsLineFromFile...)
	var allTargets string
	for _, target := range targets {
		hosts := utils.ParseStringToHosts(target)
		// 如果长度为 1 , 说明就是单个 IP
		if len(hosts) == 1 {
			allTargets += hosts[0] + "\n"
		} else {
			allTargets += strings.Join(hosts, "\n")
		}
	}
	allTargets = strings.Trim(allTargets, "\n")
	hostCount := len(strings.Split(allTargets, "\n"))
	if reqPortScan.GetEnableCClassScan() {
		allTargets = network.ParseStringToCClassHosts(allTargets)
	}
	_, _ = tmpTargetFile.WriteString(allTargets)
	if len(targets) <= 0 {
		return utils.Errorf("empty targets")
	}
	tmpTargetFile.Close()
	defer os.RemoveAll(tmpTargetFile.Name())

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "target-file",
		Value: tmpTargetFile.Name(),
	})
	// 解析用户名
	userListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(reqBrute.GetUsernames(), "\n"), "\n", reqBrute.GetUsernameFile(),
	)

	if err != nil {
		return err
	}
	defer os.RemoveAll(userListFile)

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "user-list-file",
		Value: userListFile,
	})

	// 解析密码
	passListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(reqBrute.GetPasswords(), "\n"), "\n", reqBrute.GetPasswordFile(),
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(passListFile)
	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "pass-list-file",
		Value: passListFile,
	})

	if reqBrute.GetConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "brute-concurrent",
			Value: fmt.Sprint(reqBrute.GetConcurrent()),
		})
	}

	if reqBrute.GetTargetTaskConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "task-concurrent",
			Value: fmt.Sprint(reqBrute.GetTargetTaskConcurrent()),
		})
	}

	if reqBrute.GetDelayMin() > 0 && reqBrute.GetDelayMax() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "delay-min",
			Value: fmt.Sprint(reqBrute.GetDelayMin()),
		})
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "delay-max",
			Value: fmt.Sprint(reqBrute.GetDelayMax()),
		})
	}
	// ok to stop
	if reqBrute.GetOkToStop() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "ok-to-stop",
			Value: "",
		})
	}
	// 是否使用默认字典？
	if reqBrute.GetReplaceDefaultUsernameDict() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "replace-default-username-dict",
		})
	}
	if reqBrute.GetReplaceDefaultPasswordDict() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "replace-default-password-dict",
		})
	}

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "task-name",
		Value: reqPortScan.GetTaskName(),
	})

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "host-count",
		Value: fmt.Sprintf("%v", hostCount),
	})

	if reqRecord.GetPercent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "record-ptr",
			Value: strconv.FormatInt(reqRecord.GetLastRecordPtr(), 10),
		})

		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "progress-data",
			Value: fmt.Sprintf("%.3f", reqRecord.GetPercent()),
		})

		runtimeId := req.GetRuntimeId()
		var targets []string
		for ah := range yakit.YieldAliveHostRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId) {
			targets = append(targets, ah.IP)
		}
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "record-file",
			Value: filepath.Join(consts.GetDefaultYakitBaseTempDir(), runtimeId),
		})
	}

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "ports",
		Value: reqPortScan.GetPorts(),
	})
	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "mode",
		Value: reqPortScan.GetMode(),
	})

	if reqPortScan.GetExcludeHosts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "exclude-hosts",
			Value: reqPortScan.GetExcludeHosts(),
		})
	}

	if reqPortScan.GetExcludePorts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "exclude-ports",
			Value: reqPortScan.GetExcludePorts(),
		})
	}

	if reqPortScan.GetSaveToDB() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "save-to-db",
		})
	}

	if reqPortScan.GetSaveClosedPorts() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "save-closed-ports",
		})
	}

	// 主动发包
	if reqPortScan.GetActive() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "active",
		})
	}

	// 设置指纹扫描的并发
	if reqPortScan.GetConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "concurrent",
			Value: fmt.Sprint(reqPortScan.GetConcurrent()),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "concurrent",
			Value: fmt.Sprint(50),
		})
	}

	// 设置 SYN 扫描的并发
	if reqPortScan.GetSynConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "syn-concurrent", Value: fmt.Sprint(reqPortScan.GetSynConcurrent())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "syn-concurrent", Value: "1000"})
	}

	if len(reqPortScan.GetUserFingerprintFiles()) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "user-fingerprint", Value: strings.Join(reqPortScan.GetUserFingerprintFiles(), "\n")})
	}

	if len(reqPortScan.GetProto()) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proto",
			Value: strings.Join(reqPortScan.GetProto(), ","),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proto",
			Value: "tcp",
		})
	}

	if len(utils.StringArrayFilterEmpty(reqPortScan.GetProxy())) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proxy",
			Value: strings.Join(reqPortScan.GetProxy(), ","),
		})
	}

	// 爆破设置
	if reqPortScan.GetEnableBrute() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "enable-brute",
		})
	}

	// 爬虫设置
	if reqPortScan.GetEnableBasicCrawler() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "enable-basic-crawler",
		})
	}
	if reqPortScan.GetBasicCrawlerRequestMax() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "basic-crawler-request-max",
			Value: fmt.Sprint(reqPortScan.GetBasicCrawlerRequestMax()),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "basic-crawler-request-max",
			Value: "5",
		})
	}

	if reqPortScan.GetProbeTimeout() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-timeout", Value: fmt.Sprint(reqPortScan.GetProbeTimeout())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-timeout", Value: "5.0"})
	}

	if reqPortScan.GetProbeMax() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-max", Value: "3"})
	}

	switch reqPortScan.GetFingerprintMode() {
	case "service":
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "fp-mode",
			Value: "service",
		})
	case "web":
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "fp-mode",
			Value: "web",
		})
	default:
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "fp-mode",
			Value: "all",
		})
	}

	LinkPluginList := s.PluginListGenerator(reqPortScan.GetLinkPluginConfig(), stream.Context())
	AllScriptNameList := lo.Uniq(append(reqPortScan.GetScriptNames(), LinkPluginList...))
	// handle plugin names
	var callback func()
	reqParams.ExecParams, callback, err = appendPluginNamesExKVPair("script-name-file", "\n", reqParams.ExecParams, AllScriptNameList...)
	if callback != nil {
		defer callback()
	}
	if err != nil {
		return utils.Errorf("load plugin names failed: %s", err)
	}

	if reqPortScan.GetSkippedHostAliveScan() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "skipped-host-alive-scan"})
	}

	if reqPortScan.GetHostAliveConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-concurrent", Value: fmt.Sprint(reqPortScan.GetHostAliveConcurrent())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-concurrent", Value: fmt.Sprint(20)})
	}

	if reqPortScan.GetHostAliveTimeout() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-timeout", Value: fmt.Sprint(reqPortScan.GetHostAliveTimeout())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-timeout", Value: fmt.Sprint(5.0)})
	}

	if reqPortScan.GetHostAlivePorts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-ports", Value: fmt.Sprint(reqPortScan.GetHostAlivePorts())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-ports", Value: "22,80,443"})
	}

	return s.DebugPlugin(reqParams, stream)
}

func (s *Server) RecoverSimpleDetectUnfinishedTask(req *ypb.RecoverExecBatchYakScriptUnfinishedTaskRequest, stream ypb.Yak_RecoverSimpleDetectUnfinishedTaskServer) error {
	reqTask, err := s.GetSimpleDetectRecordRequestById(context.Background(), &ypb.GetUnfinishedTaskDetailByIdRequest{RuntimeId: req.GetUid()})
	if err != nil {
		return utils.Errorf("recover request by uid[%s] failed: %s", req.GetUid(), err)
	}
	return s.SimpleDetect(reqTask, stream)
}

func (s *Server) SaveCancelSimpleDetect(ctx context.Context, req *ypb.RecordPortScanRequest) (*ypb.Empty, error) {
	// 用于管理进度保存相关内容
	runtimeId := req.GetRuntimeId()
	if runtimeId == "" {
		runtimeId = uuid.New().String()
	}
	AddSimpleDetectTask(runtimeId, req)
	return &ypb.Empty{}, nil
}

func (s *Server) QuerySimpleDetectUnfinishedTask(ctx context.Context, req *ypb.QueryUnfinishedTaskRequest) (*ypb.QueryUnfinishedTaskResponse, error) {
	filter := req.GetFilter()
	filter.ProgressSource = []string{KEY_SimpleDetectManager}
	p, progressList, err := yakit.QueryProgress(s.GetProjectDatabase(), req.GetPagination(), filter)
	if err != nil {
		return nil, err
	}

	var tasks []*ypb.UnfinishedTask
	for _, progress := range progressList {
		tasks = append(tasks, &ypb.UnfinishedTask{
			Percent:              progress.CurrentProgress,
			CreatedAt:            progress.CreatedAt.Unix(),
			RuntimeId:            progress.RuntimeId,
			YakScriptOnlineGroup: progress.YakScriptOnlineGroup,
			TaskName:             progress.TaskName,
			LastRecordPtr:        progress.LastRecordPtr,
			Target:               progress.Target,
		})
	}
	return &ypb.QueryUnfinishedTaskResponse{Tasks: tasks, Pagination: req.GetPagination(), Total: int64(p.TotalRecord)}, nil
}

func (s *Server) GetSimpleDetectRecordRequestById(ctx context.Context, req *ypb.GetUnfinishedTaskDetailByIdRequest) (*ypb.RecordPortScanRequest, error) {
	return GetSimpleDetectUnfinishedTaskByUid(s.GetProjectDatabase(), req.GetRuntimeId())
}

func (s *Server) RecoverSimpleDetectTask(req *ypb.RecoverUnfinishedTaskRequest, stream ypb.Yak_RecoverSimpleDetectTaskServer) error {
	reqTask, err := s.GetSimpleDetectRecordRequestById(context.Background(), &ypb.GetUnfinishedTaskDetailByIdRequest{RuntimeId: req.GetRuntimeId()})
	if err != nil {
		return utils.Errorf("recover request by uid[%s] failed: %s", req.GetRuntimeId(), err)
	}
	return s.SimpleDetect(reqTask, stream)
}

func (s *Server) DeleteSimpleDetectUnfinishedTask(ctx context.Context, req *ypb.DeleteUnfinishedTaskRequest) (*ypb.Empty, error) {
	filter := req.GetFilter()
	filter.ProgressSource = []string{KEY_SimpleDetectManager}
	_, err := yakit.DeleteProgress(s.GetProjectDatabase(), filter)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
