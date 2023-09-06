package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/network"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

//go:embed grpc_scanPort_script.yak
var scanPortScript []byte

func (s *Server) PortScan(req *ypb.PortScanRequest, stream ypb.Yak_PortScanServer) error {

	reqParams := &ypb.ExecRequest{
		Script: string(scanPortScript),
	}

	// 把文件写到本地。
	tmpTargetFile, err := ioutil.TempFile("", "yakit-portscan-*.txt")
	if err != nil {
		return utils.Errorf("create temp target file failed: %s", err)
	}
	raw, _ := ioutil.ReadFile(req.GetTargetsFile())
	targetsLineFromFile := utils.PrettifyListFromStringSplited(string(raw), "\n")
	targetsLine := utils.PrettifyListFromStringSplited(req.GetTargets(), "\n")
	targets := append(targetsLine, targetsLineFromFile...)

	// validation
	for _, target := range targets {
		if !utils.IsValidDomain(target) && !utils.IsValidCIDR(target) && !utils.IsIPv4(target) && !utils.IsIPv6(target) {
			return utils.Errorf("invalid target: %s\ninput must be ip, domain or cidr.", strconv.Quote(target))
		}
	}

	var allTargets = strings.Join(targets, ",")
	if req.GetEnableCClassScan() {
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
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "ports",
		Value: utils.ConcatPorts(utils.ParseStringToPorts(req.Ports)),
	})
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "mode",
		Value: req.GetMode(),
	})

	if req.GetExcludeHosts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-hosts",
			Value: req.GetExcludeHosts(),
		})
	}

	if req.GetExcludePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-ports",
			Value: req.GetExcludePorts(),
		})
	}

	if req.GetSaveToDB() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-to-db",
		})
	}

	if req.GetSaveClosedPorts() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-closed-ports",
		})
	}

	// 主动发包
	if req.GetActive() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "active",
		})
	}

	// 设置指纹扫描的并发
	if req.GetConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(req.GetConcurrent()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(50),
		})
	}

	// 设置 SYN 扫描的并发
	if req.GetSynConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: fmt.Sprint(req.GetSynConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: "1000"})
	}

	if len(req.GetProto()) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: strings.Join(req.GetProto(), ","),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: "tcp",
		})
	}

	if len(utils.StringArrayFilterEmpty(req.GetProxy())) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proxy",
			Value: strings.Join(req.GetProxy(), ","),
		})
	}

	// 爬虫设置
	if req.GetEnableBasicCrawler() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "enable-basic-crawler",
		})
	}
	if req.GetBasicCrawlerRequestMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: fmt.Sprint(req.GetBasicCrawlerRequestMax()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: "5",
		})
	}

	if req.GetProbeTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: fmt.Sprint(req.GetProbeTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: "5.0"})
	}

	if req.GetProbeMax() > 0 {
		probeMax := strconv.Itoa(int(req.GetProbeMax()))
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-max", Value: probeMax})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-max", Value: "3"})
	}

	switch req.GetFingerprintMode() {
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
	reqParams.Params, callback, err = appendPluginNamesEx("script-name-file", "\n", reqParams.Params, req.GetScriptNames()...)
	if callback != nil {
		defer callback()
	}
	if err != nil {
		return utils.Errorf("load plugin names failed: %s", err)
	}

	if req.GetSkippedHostAliveScan() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "skipped-host-alive-scan"})
	}

	if req.GetHostAliveConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(req.GetHostAliveConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(20)})
	}

	if req.GetHostAliveTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(req.GetHostAliveTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(5.0)})
	}

	if req.GetHostAlivePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: fmt.Sprint(req.GetHostAlivePorts())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: "22,80,443"})
	}

	return s.Exec(reqParams, stream)
}

func (s *Server) ViewPortScanCode(ctx context.Context, req *ypb.Empty) (*ypb.SimpleScript, error) {
	return &ypb.SimpleScript{Content: string(scanPortScript)}, nil
}
