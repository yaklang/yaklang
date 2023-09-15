package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/tidwall/gjson"
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
	reqParams := &ypb.ExecRequest{
		Script: string(simpleDetect),
	}
	reqRecord := req.LastRecord
	reqPortScan := req.PortScanRequest
	reqBrute := req.StartBruteParams
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

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "target-file",
		Value: tmpTargetFile.Name(),
	})
	// 解析用户名
	userListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(reqBrute.Usernames, "\n"), "\n", reqBrute.UsernameFile,
	)

	if err != nil {
		return err
	}
	defer os.RemoveAll(userListFile)

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "user-list-file",
		Value: userListFile,
	})

	// 解析密码
	passListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(reqBrute.Passwords, "\n"), "\n", reqBrute.PasswordFile,
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(passListFile)
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "pass-list-file",
		Value: passListFile,
	})

	if reqBrute.GetConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "brute-concurrent",
			Value: fmt.Sprint(reqBrute.GetConcurrent()),
		})
	}

	if reqBrute.GetTargetTaskConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "task-concurrent",
			Value: fmt.Sprint(reqBrute.GetTargetTaskConcurrent()),
		})
	}

	if reqBrute.GetDelayMin() > 0 && reqBrute.GetDelayMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "delay-min",
			Value: fmt.Sprint(reqBrute.GetDelayMin()),
		})
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "delay-max",
			Value: fmt.Sprint(reqBrute.GetDelayMax()),
		})
	}
	// ok to stop
	if reqBrute.GetOkToStop() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "ok-to-stop",
			Value: "",
		})
	}
	// 是否使用默认字典？
	if reqBrute.GetReplaceDefaultUsernameDict() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "replace-default-username-dict",
		})
	}
	if reqBrute.GetReplaceDefaultPasswordDict() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "replace-default-password-dict",
		})
	}

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "task-name",
		Value: reqPortScan.TaskName,
	})

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "host-count",
		Value: fmt.Sprintf("%v", hostCount),
	})

	if reqRecord.Percent > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "record-ptr",
			Value: strconv.FormatInt(reqRecord.GetLastRecordPtr(), 10),
		})

		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "progress-data",
			Value: fmt.Sprintf("%.3f", reqRecord.GetPercent()),
		})

		runtimeId := gjson.Get(reqRecord.ExtraInfo, `Params.#(Key="runtime_id").Value`).String()
		var targets []string
		for ah := range yakit.YieldAliveHostRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId) {
			targets = append(targets, ah.IP)
		}
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "record-file",
			Value: filepath.Join(consts.GetDefaultYakitBaseTempDir(), runtimeId),
		})
	}

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "ports",
		Value: reqPortScan.Ports,
	})
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "mode",
		Value: reqPortScan.GetMode(),
	})

	if reqPortScan.GetExcludeHosts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-hosts",
			Value: reqPortScan.GetExcludeHosts(),
		})
	}

	if reqPortScan.GetExcludePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-ports",
			Value: reqPortScan.GetExcludePorts(),
		})
	}

	if reqPortScan.GetSaveToDB() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-to-db",
		})
	}

	if reqPortScan.GetSaveClosedPorts() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-closed-ports",
		})
	}

	// 主动发包
	if reqPortScan.GetActive() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "active",
		})
	}

	// 设置指纹扫描的并发
	if reqPortScan.GetConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(reqPortScan.GetConcurrent()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(50),
		})
	}

	// 设置 SYN 扫描的并发
	if reqPortScan.GetSynConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: fmt.Sprint(reqPortScan.GetSynConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: "1000"})
	}

	if len(reqPortScan.GetProto()) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: strings.Join(reqPortScan.GetProto(), ","),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: "tcp",
		})
	}

	if len(utils.StringArrayFilterEmpty(reqPortScan.GetProxy())) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proxy",
			Value: strings.Join(reqPortScan.GetProxy(), ","),
		})
	}

	// 爆破设置
	if reqPortScan.GetEnableBrute() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "enable-brute",
		})
	}

	// 爬虫设置
	if reqPortScan.GetEnableBasicCrawler() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "enable-basic-crawler",
		})
	}
	if reqPortScan.GetBasicCrawlerRequestMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: fmt.Sprint(reqPortScan.GetBasicCrawlerRequestMax()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: "5",
		})
	}

	if reqPortScan.GetProbeTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: fmt.Sprint(reqPortScan.GetProbeTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: "5.0"})
	}

	if reqPortScan.GetProbeMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-max", Value: "3"})
	}

	switch reqPortScan.GetFingerprintMode() {
	case "service":
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "service",
		})
	case "web":
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "web",
		})
	default:
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "all",
		})
	}

	// handle plugin names
	var callback func()
	reqParams.Params, callback, err = appendPluginNamesEx("script-name-file", "\n", reqParams.Params, reqPortScan.GetScriptNames()...)
	if callback != nil {
		defer callback()
	}
	if err != nil {
		return utils.Errorf("load plugin names failed: %s", err)
	}

	if reqPortScan.GetSkippedHostAliveScan() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "skipped-host-alive-scan"})
	}

	if reqPortScan.GetHostAliveConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(reqPortScan.GetHostAliveConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(20)})
	}

	if reqPortScan.GetHostAliveTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(reqPortScan.GetHostAliveTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(5.0)})
	}

	if reqPortScan.GetHostAlivePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: fmt.Sprint(reqPortScan.GetHostAlivePorts())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: "22,80,443"})
	}

	return s.Exec(reqParams, stream)
}

func (s *Server) SaveCancelSimpleDetect(ctx context.Context, req *ypb.RecordPortScanRequest) (*ypb.Empty, error) {
	// 用于管理进度保存相关内容
	manager := NewProgressManager(s.GetProjectDatabase())
	uid := uuid.NewV4().String()
	manager.AddSimpleDetectTaskToPool(uid, req)
	return nil, nil
}

func (s *Server) RecoverSimpleDetectUnfinishedTask(req *ypb.RecoverExecBatchYakScriptUnfinishedTaskRequest, stream ypb.Yak_RecoverSimpleDetectUnfinishedTaskServer) error {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqTask, err := manager.GetSimpleProgressByUid(req.GetUid(), true, false)
	if err != nil {
		return utils.Errorf("recover request by uid[%s] failed: %s", req.GetUid(), err)
	}

	return s.SimpleDetect(reqTask, stream)
}
