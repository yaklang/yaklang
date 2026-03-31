package scannode

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

type ScannerAgentReporter struct {
	TaskId    string
	SubTaskId string
	RuntimeId string

	executionRef *jobExecutionRef
	agent        *ScanNode
}

// convertRawToString 将原始数据转换为字符串，处理 JSON 反序列化后的各种数据格式
func convertRawToString(raw interface{}) string {
	if raw == nil {
		return ""
	}
	switch data := raw.(type) {
	case []byte:
		return string(data)
	case []interface{}:
		// JSON 反序列化后的数组，每个元素是 float64
		bytes := make([]byte, len(data))
		for i, v := range data {
			if f, ok := v.(float64); ok {
				bytes[i] = byte(f)
			}
		}
		return string(bytes)
	case string:
		return data
	default:
		return utils.InterfaceToString(raw)
	}
}

func NewScannerAgentReporter(
	taskId string,
	subTaskId string,
	runtimeId string,
	executionRef *jobExecutionRef,
	agent *ScanNode,
) *ScannerAgentReporter {
	return &ScannerAgentReporter{
		TaskId:       taskId,
		SubTaskId:    subTaskId,
		RuntimeId:    runtimeId,
		executionRef: executionRef,
		agent:        agent,
	}
}

func (r *ScannerAgentReporter) Report(record *schema.Report) error {
	if r.agent == nil {
		return nil
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return utils.Errorf("marshal report failed: %s", err)
	}
	return r.publishJobReport(legionReportKindScan, raw)
}

func (r *ScannerAgentReporter) ReportWeakPassword(result interface{}) error {
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
		return r.publishJobRisk(
			legionRiskKindWeakPassword,
			weakPasswordTitle(ret),
			ret.Target,
			"",
			weakPasswordDedupeKey(ret),
			res.Content,
		)
	default:
		return utils.Errorf("unsupported: %v", spew.Sdump(ret))
	}
}

func (r *ScannerAgentReporter) ReportRisk(
	title string, target string, details interface{},
	tags ...string,
) error {
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
		vul.Request = convertRawToString(utils.MapGetRaw(v, "Request"))
		vul.Response = convertRawToString(utils.MapGetRaw(v, "Response"))
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
	return r.publishJobRisk(
		riskKindFromVuln(vul),
		riskTitle(title, vul),
		target,
		normalizeSeverity(vul.Severity),
		riskDedupeKey(vul, target, title),
		res.Content,
	)
}

func (r *ScannerAgentReporter) ReportVul(i interface{}) error {
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
		vulnTitle := firstNonEmpty(ret.TitleName, ret.PocName, ret.CVE)
		return r.publishJobRisk(
			legionRiskKindVulnerability,
			riskTitle(vulnTitle, nil),
			ret.Target,
			normalizeSeverity(ret.Severity),
			pocVulnDedupeKey(ret),
			res.Content,
		)
	case *Vuln:
		res, err := NewVulnResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		return r.publishJobRisk(
			riskKindFromVuln(ret),
			riskTitle(ret.Title, ret),
			ret.Target,
			normalizeSeverity(ret.Severity),
			riskDedupeKey(ret, ret.Target, ret.Title),
			res.Content,
		)
	default:
		return utils.Errorf("unsupported: %s", spew.Sdump(i))
	}
}

func (r *ScannerAgentReporter) ReportFingerprint(i interface{}) error {
	switch ret := i.(type) {
	case *fp.MatchResult:
		res, err := spec.NewScanFingerprintResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		target := utils.HostPort(ret.Target, ret.Port)
		return r.publishJobAsset(
			legionAssetKindServiceFingerprint,
			fingerprintTitle(ret),
			target,
			fingerprintIdentityKey(ret),
			res.Content,
		)
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
	return r.publishJobProgress(process)
}

func (r *ScannerAgentReporter) ReportTCPOpenPort(host interface{}, port interface{}) error {
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
	target := utils.HostPort(hostRaw, portInt)
	return r.publishJobAsset(
		legionAssetKindTCPOpenPort,
		target,
		target,
		tcpOpenPortIdentityKey(hostRaw, portInt),
		res.Content,
	)
}

func (r *ScannerAgentReporter) ReportStatusCard(_ []byte) error {
	return nil
}

var nonIdentifierChars = regexp.MustCompile(`[^a-z0-9]+`)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium", "warning":
		return "medium"
	case "low":
		return "low"
	default:
		return ""
	}
}

func normalizeIdentifierToken(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return ""
	}
	normalized := nonIdentifierChars.ReplaceAllString(trimmed, "_")
	return strings.Trim(normalized, "_")
}

func weakPasswordTitle(result *bruteutils.BruteItemResult) string {
	if result == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s weak password for %s",
		strings.ToUpper(result.Type),
		result.Target,
	)
}

func weakPasswordDedupeKey(result *bruteutils.BruteItemResult) string {
	if result == nil {
		return ""
	}
	return utils.CalcSha256(
		strings.Join(
			[]string{"weak_password", result.Target, result.Type, result.Username},
			"|",
		),
	)
}

func riskKindFromVuln(vuln *Vuln) string {
	if vuln == nil {
		return legionRiskKindSecurityRisk
	}
	if strings.Contains(strings.ToLower(vuln.Plugin), "weakpassword") {
		return legionRiskKindWeakPassword
	}
	if token := normalizeIdentifierToken(vuln.RiskType); token != "" {
		return token
	}
	if vuln.CVE != "" || strings.Contains(strings.ToLower(vuln.Plugin), "poc") {
		return legionRiskKindVulnerability
	}
	return legionRiskKindSecurityRisk
}

func riskTitle(title string, vuln *Vuln) string {
	if normalized := firstNonEmpty(
		title,
		func() string {
			if vuln == nil {
				return ""
			}
			return vuln.TitleVerbose
		}(),
		func() string {
			if vuln == nil {
				return ""
			}
			return vuln.Title
		}(),
	); normalized != "" {
		return normalized
	}
	if vuln == nil {
		return ""
	}
	return firstNonEmpty(vuln.Plugin, vuln.Target)
}

func riskDedupeKey(vuln *Vuln, target string, title string) string {
	if vuln != nil && strings.TrimSpace(vuln.Hash) != "" {
		return strings.TrimSpace(vuln.Hash)
	}
	return utils.CalcSha256(
		strings.Join(
			[]string{
				riskKindFromVuln(vuln),
				strings.TrimSpace(target),
				strings.TrimSpace(title),
				func() string {
					if vuln == nil {
						return ""
					}
					return strings.TrimSpace(vuln.Plugin)
				}(),
			},
			"|",
		),
	)
}

func pocVulnDedupeKey(vuln *tools.PocVul) string {
	if vuln == nil {
		return ""
	}
	if token := strings.TrimSpace(vuln.UUID); token != "" {
		return token
	}
	return utils.CalcSha256(
		strings.Join(
			[]string{
				legionRiskKindVulnerability,
				strings.TrimSpace(vuln.Target),
				firstNonEmpty(vuln.TitleName, vuln.PocName, vuln.CVE),
				strings.TrimSpace(vuln.Source),
			},
			"|",
		),
	)
}

func fingerprintTitle(result *fp.MatchResult) string {
	if result == nil {
		return ""
	}
	return firstNonEmpty(
		result.GetServiceName(),
		result.GetHtmlTitle(),
		utils.HostPort(result.Target, result.Port),
	)
}

func fingerprintIdentityKey(result *fp.MatchResult) string {
	if result == nil {
		return ""
	}
	if identifier := strings.TrimSpace(result.Identifier()); identifier != "" {
		return identifier
	}
	return utils.CalcSha256(utils.HostPort(result.Target, result.Port))
}

func tcpOpenPortIdentityKey(host string, port int) string {
	return fmt.Sprintf("tcp://%s:%d", host, port)
}
