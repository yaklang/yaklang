package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/network"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_scanPort_script.yak
var scanPortScript []byte

func (s *Server) PortScan(req *ypb.PortScanRequest, stream ypb.Yak_PortScanServer) error {

	//reqParams := &ypb.ExecRequest{
	//	Script: string(scanPortScript),
	//}

	reqParams := &ypb.DebugPluginRequest{
		Code:             string(scanPortScript),
		PluginType:       "yak",
		LinkPluginConfig: &ypb.HybridScanPluginConfig{},
	}
	if req.GetLinkPluginConfig() != nil {
		reqParams.LinkPluginConfig = req.LinkPluginConfig
	}

	// 把文件写到本地。
	tmpTargetFile, err := ioutil.TempFile("", "yakit-portscan-*.txt")
	if err != nil {
		return utils.Errorf("create temp target file failed: %s", err)
	}
	raw, _ := ioutil.ReadFile(req.GetTargetsFile())
	targetsLineFromFile := utils.PrettifyListFromStringSplitEx(string(raw), "\n", ",")
	targetsLine := utils.PrettifyListFromStringSplitEx(req.GetTargets(), "\n", ",")
	targets := append(targetsLine, targetsLineFromFile...)

	// validation
	for _, target := range targets {
		if !utils.IsValidDomain(target) && !utils.IsValidCIDR(target) && !utils.IsIPv4(target) && !utils.IsIPv6(target) {
			host, port, err := utils.ParseStringToHostPort(target)
			if port <= 0 || err != nil {
				return utils.Errorf("invalid target: %s\ninput must be ip, domain or cidr (url/host:port).", strconv.Quote(target))
			}
			_ = host
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

	if len(req.ScriptNames) > 0 {
		reqParams.LinkPluginConfig.PluginNames = append(reqParams.LinkPluginConfig.PluginNames, req.ScriptNames...)
	}

	if len(req.GetUserFingerprintFiles()) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "fingerprint-files",
			Value: strings.Join(req.GetUserFingerprintFiles(), ","),
		})
	}

	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "target-file",
		Value: tmpTargetFile.Name(),
	})
	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "ports",
		Value: utils.ConcatPorts(utils.ParseStringToPorts(req.Ports)),
	})
	reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
		Key:   "mode",
		Value: req.GetMode(),
	})

	if req.GetExcludeHosts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "exclude-hosts",
			Value: req.GetExcludeHosts(),
		})
	}

	if req.GetExcludePorts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "exclude-ports",
			Value: req.GetExcludePorts(),
		})
	}

	if req.GetSaveToDB() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "save-to-db",
		})
	}

	if req.GetSaveClosedPorts() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "save-closed-ports",
		})
	}

	// 主动发包
	if req.GetActive() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "active",
		})
	}

	// 设置指纹扫描的并发
	if req.GetConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "concurrent",
			Value: fmt.Sprint(req.GetConcurrent()),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "concurrent",
			Value: fmt.Sprint(50),
		})
	}

	// 设置 SYN 扫描的网卡
	if req.GetSynScanNetInterface() != "" {
		reqParams.ExecParams = append(
			reqParams.ExecParams, &ypb.KVPair{Key: "syn-scan-net-interface", Value: req.GetSynScanNetInterface()},
		)
	}

	// 设置 SYN 扫描的并发
	if req.GetSynConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "syn-concurrent", Value: fmt.Sprint(req.GetSynConcurrent())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "syn-concurrent", Value: "1000"})
	}

	if len(req.GetProto()) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proto",
			Value: strings.Join(req.GetProto(), ","),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proto",
			Value: "tcp",
		})
	}

	if len(utils.StringArrayFilterEmpty(req.GetProxy())) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "proxy",
			Value: strings.Join(req.GetProxy(), ","),
		})
	}

	// 爬虫设置
	if req.GetEnableBasicCrawler() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key: "enable-basic-crawler",
		})
	}
	if req.GetBasicCrawlerRequestMax() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "basic-crawler-request-max",
			Value: fmt.Sprint(req.GetBasicCrawlerRequestMax()),
		})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "basic-crawler-request-max",
			Value: "5",
		})
	}

	if req.GetProbeTimeout() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-timeout", Value: fmt.Sprint(req.GetProbeTimeout())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-timeout", Value: "5.0"})
	}

	if req.GetProbeMax() > 0 {
		probeMax := strconv.Itoa(int(req.GetProbeMax()))
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-max", Value: probeMax})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "probe-max", Value: "3"})
	}

	switch req.GetFingerprintMode() {
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

	if req.GetSkippedHostAliveScan() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "skipped-host-alive-scan", Value: "true"})
	}

	if req.GetHostAliveConcurrent() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-concurrent", Value: fmt.Sprint(req.GetHostAliveConcurrent())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-concurrent", Value: fmt.Sprint(20)})
	}

	if req.GetHostAliveTimeout() > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-timeout", Value: fmt.Sprint(req.GetHostAliveTimeout())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-timeout", Value: fmt.Sprint(5.0)})
	}

	if req.GetHostAlivePorts() != "" {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-ports", Value: fmt.Sprint(req.GetHostAlivePorts())})
	} else {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "host-alive-ports", Value: "22,80,443"})
	}

	if req.GetBasicCrawlerEnableJSParser() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "basic-crawler-enable-jsparser", Value: ""})
	}

	if req.GetEnableFingerprintGroup() {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{Key: "use-fp-group"})
	}
	if len(utils.StringArrayFilterEmpty(req.GetFingerprintGroup())) > 0 {
		reqParams.ExecParams = append(reqParams.ExecParams, &ypb.KVPair{
			Key:   "fp-groups",
			Value: strings.Join(req.GetFingerprintGroup(), ","),
		})
	}
	return s.DebugPlugin(reqParams, stream)
}

func (s *Server) ViewPortScanCode(ctx context.Context, req *ypb.Empty) (*ypb.SimpleScript, error) {
	return &ypb.SimpleScript{Content: string(scanPortScript)}, nil
}
