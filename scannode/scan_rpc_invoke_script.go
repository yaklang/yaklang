package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/scannode/scanrpc"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func (s *ScanNode) rpc_startScript(ctx context.Context, node string, req *scanrpc.SCAN_StartScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_StartScriptResponse, error) {
	if req.Content == "" {
		log.Error("empty content for rpc_startScript")
		return nil, utils.Error("empty content")
	}
	rsp, err := s.rpc_invokeScript(ctx, node, &scanrpc.SCAN_InvokeScriptRequest{
		TaskId:          uuid.New().String(),
		RuntimeId:       uuid.New().String(),
		SubTaskId:       uuid.New().String(),
		ScriptContent:   req.Content,
		ScriptJsonParam: "{}",
	}, broker)
	if err != nil {
		return nil, err
	}
	_ = rsp
	return &scanrpc.SCAN_StartScriptResponse{}, nil
}

func (s *ScanNode) rpc_invokeScript(ctx context.Context, node string, req *scanrpc.SCAN_InvokeScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_InvokeScriptResponse, error) {
	runtimeId := req.RuntimeId
	_ = runtimeId

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskId := fmt.Sprintf("script-task-%v", req.SubTaskId)
	s.manager.Add(taskId, &Task{
		TaskType: "script-task",
		TaskId:   taskId,
		Ctx:      ctx,
		Cancel:   cancel,
	})
	defer s.manager.Remove(taskId)

	scanNodePath, err := os.Executable()
	if err != nil {
		return nil, utils.Errorf("rpc call InvokeScript failed: fetch node path err: %s", err)
	}
	_ = scanNodePath
	reportor := NewScannerAgentReporter(req.TaskId, req.SubTaskId, req.RuntimeId, s)
	res := scanrpc.SCAN_InvokeScriptResponse{}
	yakitServer := yaklib.NewYakitServer(
		0,
		yaklib.SetYakitServer_ProgressHandler(func(id string, progress float64) {
			reportor.ReportProcess(progress)
			return
		}),
		yaklib.SetYakitServer_LogHandler(func(level string, info string) {
			log.Infof("LEVEL: %v INFO: %v", level, info)
			switch strings.ToLower(level) {
			case "fingerprint":
				var res fp.MatchResult
				err := json.Unmarshal([]byte(info), &res)
				if err != nil {
					log.Errorf("unmarshal fingerprint failed: %v", err)
					return
				}
				reportor.ReportFingerprint(&res)
			case "synscan-result":
				var res synscan.SynScanResult
				err := json.Unmarshal([]byte(info), &res)
				if err != nil {
					log.Errorf("unmarshal synscan-result failed: %v", err)
					return
				}
				reportor.ReportTCPOpenPort(res.Host, res.Port)
			case "json-risk":
				var rawData = make(map[string]interface{})
				err := json.Unmarshal([]byte(info), &rawData)
				if err != nil {
					log.Errorf("unmarshal risk failed: %s", err)
					return
				}
				var title = utils.MapGetFirstRaw(rawData, "TitleVerbose", "Title")
				if title == "" {
					title = "暂无标题"
				}
				var target = utils.MapGetFirstRaw(rawData, "Url", "url")
				var host = utils.MapGetString(rawData, "Host")
				var port = utils.MapGetString(rawData, "Port")
				if target == "" {
					target = utils.HostPort(host, port)
				}

				reportor.ReportRisk(fmt.Sprint(title), fmt.Sprint(target), rawData)
			case "report":
				reportId, _ := strconv.ParseInt(info, 10, 64)
				if reportId <= 0 {
					return
				}
				db := consts.GetGormProjectDatabase()
				if db == nil {
					return
				}
				reportIns, err := yakit.GetReportRecord(db, reportId)
				if err != nil {
					log.Errorf("query report failed: %s", err)
					return
				}
				reportOutput, err := reportIns.ToReport()
				if err != nil {
					log.Errorf("report marshal from database failed: %s", err)
					return
				}
				err = reportor.Report(reportOutput)
				if err != nil {
					log.Errorf("report to palm-server failed: %s", err)
				}
			case "json":
				var rawData = make(map[string]interface{})
				err := json.Unmarshal([]byte(info), &rawData)
				if err == nil {
					flag := utils.MapGetFirstRaw(rawData, "Flag", "flag")
					if flag == "ReturnData" {
						data := utils.MapGetFirstRaw(rawData, "Data", "data")
						if data != nil {
							res.Data = data
						}
					}
				}
			}
		}),
	)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	var params = []string{"--yakit-webhook", yakitServer.Addr()}
	_ = params

	if runtimeId != "" {
		params = append(params, "--runtime-id", runtimeId)
	}

	// 把用户传入的参数转换为命令行参数
	var paramsKeyValue = make(map[string]interface{})
	var paramsRaw interface{}
	_ = json.Unmarshal([]byte(req.ScriptJsonParam), &paramsRaw)
	if values := utils.InterfaceToGeneralMap(paramsRaw); len(values) > 0 {
		for k, v := range values {
			if k == "__DEFAULT__" {
				continue
			}
			paramsKeyValue[k] = v
		}
	}
	for k, v := range paramsKeyValue {
		k = strings.TrimLeft(k, "-")
		params = append(params, "--"+k)
		params = append(params, utils.InterfaceToString(v))
	}

	// 要执行的代码内容
	f, err := consts.TempFile("distributed-yakcode-*.yak")
	if err != nil {
		return nil, err
	}
	f.WriteString(req.ScriptContent)
	f.Close()
	defer func() {
		os.RemoveAll(f.Name())
	}()

	baseCmd := []string{"distyak", f.Name()}
	// 执行脚本
	log.Infof("yak %v %v", f.Name(), params)
	cmd := exec.CommandContext(ctx, scanNodePath,
		append(baseCmd, params...)...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")))
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeId))

	var remoteReverseIP string
	var remoteReversePort int
	var remoteAddr string
	var remoteSecret string
	if remoteReverseIP != "" && remoteReversePort > 0 {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("YAK_BRIDGE_REMOTE_REVERSE_ADDR=%v", utils.HostPort(remoteReverseIP, remoteReversePort)),
			fmt.Sprintf("YAK_BRIDGE_ADDR=%v", remoteAddr),
			fmt.Sprintf("YAK_BRIDGE_SECRET=%v", remoteSecret),
		)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		log.Errorf("exec yakScript %v failed: %s", f.Name(), err)
		return nil, utils.Errorf("exec yakScript %v failed: %s", f.Name(), err)
	}

	return &res, nil
}

func (s *ScanNode) rpcQueryYakScript(ctx context.Context, node string, req *ypb.QueryYakScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_QueryYakScriptResponse, error) {

	if req.GetNoResultReturn() {
		return &scanrpc.SCAN_QueryYakScriptResponse{
			Pagination: req.GetPagination(),
			Total:      0,
			Data:       nil,
			Groups:     nil,
		}, nil
	}
	p, data, err := yakit.QueryYakScript(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	rsp := &scanrpc.SCAN_QueryYakScriptResponse{
		Pagination: &ypb.Paging{
			Page:    int64(p.Page),
			Limit:   int64(p.Limit),
			OrderBy: req.Pagination.OrderBy,
			Order:   req.Pagination.Order,
		},
		Total: int64(p.TotalRecord),
	}
	for _, d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	var gs []string
	groups, err := yakit.QueryGroupCount(consts.GetGormProfileDatabase(), nil, 0)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.IsPocBuiltIn == true {
			continue
		}
		gs = append(gs, group.Value)
	}
	if len(gs) > 0 {
		rsp.Groups = gs
	}
	return rsp, nil
}
