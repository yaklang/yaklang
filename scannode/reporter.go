package scannode

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"net"
	"strings"
)

type ScannerAgentReporter struct {
	TaskId    string
	SubTaskId string
	RuntimeId string

	agent *ScanNode
}

func NewScannerAgentReporter(taskId string, subTaskId string, runtimeId string, agent *ScanNode) *ScannerAgentReporter {
	return &ScannerAgentReporter{
		TaskId:    taskId,
		SubTaskId: subTaskId,
		RuntimeId: runtimeId,
		agent:     agent,
	}
}

func (r *ScannerAgentReporter) Report(record *schema.Report) error {
	if r.agent != nil {
		raw, err := json.Marshal(record)
		if err != nil {
			return utils.Errorf("marshal report failed: %s", err)
		}
		r.agent.feedback(&spec.ScanResult{
			Type:      spec.ScanResult_Report,
			Content:   raw,
			TaskId:    r.TaskId,
			RuntimeId: r.RuntimeId,
			SubTaskId: r.SubTaskId,
		})
	}
	return nil
}

func (r *ScannerAgentReporter) ReportWeakPassword(result interface{}) error {
	var s = r.agent
	switch ret := result.(type) {
	case *bruteutils.BruteItemResult:
		host, port, err := utils.ParseStringToHostPort(ret.Target)
		if err != nil {
			return err
		}

		if !utils.IsIPv4(host) {
			return r.ReportRisk(
				fmt.Sprintf(
					"%v WeakPassword for %v: %v/%v",
					strings.ToUpper(ret.Type),
					ret.Target, ret.Username, ret.Password,
				),
				ret.Target, utils.Jsonify(ret),
			)
		}

		var targetType = VulnTargetType_Service
		if utils.IsHttpOrHttpsUrl(ret.Target) {
			targetType = VulnTargetType_Url
		}

		vul := &Vuln{
			IPAddr:       host,
			Host:         host,
			Port:         port,
			IsPrivateNet: utils.IsPrivateIP(net.ParseIP(host)),
			Target:       ret.Target,
			TargetRaw: postgres.Jsonb{RawMessage: utils.Jsonify(map[string]interface{}{
				"target": ret.Target, "host": host, "port": port,
			})},
			TargetType: targetType,
			Plugin: strings.Join([]string{
				"weakpassword", strings.ToLower(ret.Type),
			}, "/"),
			Detail: postgres.Jsonb{RawMessage: utils.Jsonify(ret)},
		}
		res, err := NewVulnResult(vul)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %v", spew.Sdump(ret))
	}
}

func (r *ScannerAgentReporter) ReportRisk(
	title string, target string, details interface{},
	tags ...string,
) error {
	var s = r.agent
	//details, err := json.Marshal(details)
	//if err != nil {
	//	return err
	//}

	vul := &Vuln{
		Target: target,
		TargetRaw: postgres.Jsonb{RawMessage: utils.Jsonify(map[string]interface{}{
			"target": target, "title": title,
		})},
		TargetType: VulnTargetType_Risk,
		Plugin:     strings.Join(append([]string{"risk"}, tags...), "/"),
	}
	if v, ok := details.(map[string]interface{}); ok {
		if ip := net.ParseIP(utils.FixForParseIP(utils.MapGetString(v, "IP"))); ip != nil {
			vul.IPAddr = ip.String()
			if i, err := utils.IPv4ToUint32(ip.To4()); err == nil {
				vul.IPv4Int = uint32(i)
			}
			vul.IsPrivateNet = utils.IsPrivateIP(ip)
		}
		vul.Host = utils.MapGetString(v, "Host")
		vul.Port = int(utils.MapGetFloat64(v, "Port"))
		vul.Hash = utils.MapGetString(v, "Hash")
		vul.FromThreatAnalysisRuntimeId = utils.MapGetString(v, "RuntimeId")
		vul.FromThreatAnalysisTaskId = r.TaskId
		Details := utils.MapGetString(v, "Details")
		var lib map[string]interface{}
		_ = json.Unmarshal([]byte(Details), &lib)
		vul.Detail = postgres.Jsonb{RawMessage: utils.Jsonify(lib)}
		vul.Payload = utils.MapGetString(v, "Payload")
		vul.RiskTypeVerbose = utils.MapGetString(v, "RiskTypeVerbose")
		vul.RiskType = utils.MapGetString(v, "RiskType")
		vul.Severity = utils.MapGetString(v, "Severity")
		vul.FromYakScript = utils.MapGetString(v, "FromYakScript")
		vul.TitleVerbose = utils.MapGetString(v, "TitleVerbose")
		vul.Title = utils.MapGetString(v, "Title")
		vul.ReverseToken = utils.MapGetString(v, "ReverseToken")
		vul.Url = utils.MapGetString(v, "Url")
		vul.Description = utils.MapGetString(v, "Description")
		vul.Solution = utils.MapGetString(v, "Solution")
		vul.Request = utils.MapGetString(v, "Request")
		vul.Response = utils.MapGetString(v, "Response")
		vul.Parameter = utils.MapGetString(v, "Parameter")
		vul.IsPotential = utils.MapGetBool(v, "IsPotential")
		vul.CVE = utils.MapGetString(v, "CVE")
		vul.CveAccessVector = utils.MapGetString(v, "CveAccessVector")
		vul.CveAccessComplexity = utils.MapGetString(v, "CveAccessComplexity")
	}
	res, err := NewVulnResult(vul)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	s.feedback(res)
	return nil
}

func (r *ScannerAgentReporter) ReportVul(i interface{}) error {
	var s = r.agent
	switch ret := i.(type) {
	case *tools.PocVul:
		var targetType = VulnTargetType_Service
		if strings.HasPrefix(strings.TrimSpace(ret.Target), "http://") ||
			strings.HasPrefix(strings.TrimSpace(ret.Target), "https://") {
			targetType = VulnTargetType_Url
		}

		raw, _ := json.Marshal(ret)
		targetRaw, _ := json.Marshal(map[string]interface{}{
			"ip":     ret.IP,
			"port":   ret.Port,
			"target": ret.Target,
		})

		res, err := NewVulnResult(&Vuln{
			IPAddr:       ret.IP,
			Host:         ret.IP,
			Port:         ret.Port,
			IsPrivateNet: utils.IsPrivateIP(net.ParseIP(ret.IP)),
			Target:       ret.Target,
			TargetRaw:    postgres.Jsonb{RawMessage: targetRaw},
			TargetType:   targetType,
			Plugin:       fmt.Sprintf("palm-poc-invoker/%v/%v", ret.Source, ret.PocName),
			Detail:       postgres.Jsonb{RawMessage: raw},
		})
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	case *Vuln:
		res, err := NewVulnResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %s", spew.Sdump(i))
	}
}

func (r *ScannerAgentReporter) ReportFingerprint(i interface{}) error {
	var s = r.agent
	switch ret := i.(type) {
	case *fp.MatchResult:
		res, err := spec.NewScanFingerprintResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %v", spew.Sdump(i))
	}
}

func (r *ScannerAgentReporter) ReportProcess(process float64) error {
	res, err := spec.NewScanProcessResult(process)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	r.agent.feedback(res)
	return nil
}
func (r *ScannerAgentReporter) ReportTCPOpenPort(host interface{}, port interface{}) error {
	var s = r.agent

	hostRaw, portInt, err := utils.ParseStringToHostPort(utils.HostPort(fmt.Sprint(host), port))
	if err != nil {
		return err
	}

	ip := net.ParseIP(utils.FixForParseIP(hostRaw))
	if ip == nil {
		return utils.Errorf("IP parse failed: %s", spew.Sdump(host))
	}
	res, err := spec.NewScanTCPOpenPortResult(ip, portInt, spec.PortStateType_Open)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	s.feedback(res)
	return nil
}
