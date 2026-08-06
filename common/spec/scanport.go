package spec

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type ScanPortTask struct {
	// 扫描目标
	Hosts string `json:"host"`
	Ports string `json:"port"`
}

type ScanResultType string

const (
	// 只有端口开放信息
	ScanResult_PortState ScanResultType = "port_state"

	// Fp.MatcherResult 包含指纹信息
	ScanResult_Fingerprint ScanResultType = "fingerprint"

	// *yakit.Report 整体报告
	ScanResult_Report ScanResultType = "report"

	// HttpFlow 的资产信息
	ScanResult_HTTPFlow ScanResultType = "http-flow"

	// 漏洞信息，弱密码啥的也应该包含在这个里面
	ScanResult_Vuln ScanResultType = "vuln"

	// 发现域名资产啥的
	ScanResult_Domain ScanResultType = "domain"

	//扫描任务进度
	ScanResult_Process ScanResultType = "process"

	ScanResult_StatusCard ScanResultType = "status"

	// SSA 对象已上传，等待服务端异步导入
	ScanResult_SSAArtifactReady ScanResultType = "ssa-artifact-ready"

	// SSA artifact upload failed; server should mark the scan as failed with the
	// provided error_code and error_message.
	ScanResult_SSAArtifactUploadFailed ScanResultType = "ssa-artifact-upload-failed"
)

type ScanResult struct {
	Type    ScanResultType  `json:"type"`
	Content json.RawMessage `json:"content"`

	// 如果这三个字段有的话，说明是分布式任务，需要额外处理一下这个内容
	TaskId    string `json:"task_id"`
	RuntimeId string `json:"runtime_id"`
	SubTaskId string `json:"sub_task_id"`
}

type PortStateType string

const (
	PortStateType_Unknown PortStateType = "unknown"
	PortStateType_Open    PortStateType = "open"
	PortStateType_Closed  PortStateType = "closed"
)

type PortFingerprint struct {
	Host        string               `json:"host"`
	Port        int                  `json:"port"`
	Proto       fp.TransportProto    `json:"proto"`
	State       PortStateType        `json:"state"`
	Reason      string               `json:"reason"`
	Product     string               `json:"product"`
	Version     string               `json:"version"`
	Hostname    string               `json:"hostname"`
	DeviceType  string               `json:"device_type"`
	Domains     []string             `json:"domains"`
	CPEs        []string             `json:"cpes"`
	Banner      string               `json:"banner"`
	ServiceName string               `json:"service_name"`
	Title       string               `json:"title"`
	HTTP        *PortFingerprintHTTP `json:"http,omitempty"`
	TLS         *PortFingerprintTLS  `json:"tls,omitempty"`
}

type PortFingerprintHTTP struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	StatusCode  int    `json:"status_code"`
	IsHTTPS     bool   `json:"is_https"`
	Server      string `json:"server"`
	ContentType string `json:"content_type"`
}

type PortFingerprintTLS struct {
	Enabled           bool     `json:"enabled"`
	Checked           bool     `json:"checked"`
	SNI               string   `json:"sni"`
	ServerName        string   `json:"server_name"`
	Protocol          string   `json:"protocol"`
	Version           string   `json:"version"`
	CipherSuite       string   `json:"cipher_suite"`
	Issuer            string   `json:"issuer"`
	Subject           string   `json:"subject"`
	NotBefore         string   `json:"not_before"`
	NotAfter          string   `json:"not_after"`
	DNSNames          []string `json:"dns_names"`
	FingerprintSHA256 string   `json:"fingerprint_sha256"`
}

func NewHTTPFlowScanResult(isHttps bool, req *http.Request, rsp *http.Response) (*ScanResult, error) {
	return nil, utils.Error("not implemented")
}

func NewScanFingerprintResult(m *fp.MatchResult) (*ScanResult, error) {
	if m.Fingerprint == nil {
		return nil, errors.Errorf("fetch fingerprint to %v failed: %v", m.Target, m.Reason)
	}

	var state PortStateType
	switch m.State {
	case fp.OPEN:
		state = PortStateType_Open
	case fp.CLOSED:
		state = PortStateType_Closed
	case fp.UNKNOWN:
		state = PortStateType_Unknown
	default:
		state = PortStateType_Unknown
	}

	httpSummary := newPortFingerprintHTTP(m)
	tlsSummary := newPortFingerprintTLS(m, httpSummary)

	f := &PortFingerprint{
		Host:        m.Target,
		Port:        m.Port,
		Proto:       m.GetProto(),
		State:       state,
		Reason:      m.Reason,
		Product:     fingerprintProduct(m.Fingerprint),
		Version:     fingerprintVersion(m.Fingerprint),
		Hostname:    m.Fingerprint.Hostname,
		DeviceType:  m.Fingerprint.DeviceType,
		Domains:     fingerprintDomains(m, httpSummary),
		CPEs:        m.GetCPEs(),
		Banner:      m.GetBanner(),
		ServiceName: m.GetServiceName(),
		Title:       m.GetHtmlTitle(),
		HTTP:        httpSummary,
		TLS:         tlsSummary,
	}

	raw, err := json.Marshal(f)
	if err != nil {
		return nil, errors.Errorf("marshal port fingerprint failed: %v", err)
	}

	return &ScanResult{
		Type:    ScanResult_Fingerprint,
		Content: raw,
	}, nil
}

func fingerprintProduct(info *fp.FingerprintInfo) string {
	if info == nil {
		return ""
	}
	if info.ProductVerbose != "" {
		return info.ProductVerbose
	}
	for _, raw := range info.CPEs {
		cpe, err := webfingerprint.ParseToCPE(raw)
		if err != nil {
			continue
		}
		if cpe.Product != "" && cpe.Product != "*" {
			return cpe.Product
		}
	}
	return ""
}

func fingerprintVersion(info *fp.FingerprintInfo) string {
	if info == nil {
		return ""
	}
	if info.Version != "" {
		return info.Version
	}
	for _, raw := range info.CPEs {
		cpe, err := webfingerprint.ParseToCPE(raw)
		if err != nil {
			continue
		}
		if cpe.Version != "" && cpe.Version != "*" {
			return cpe.Version
		}
	}
	return ""
}

func newPortFingerprintHTTP(m *fp.MatchResult) *PortFingerprintHTTP {
	if m == nil || m.Fingerprint == nil || len(m.Fingerprint.HttpFlows) == 0 {
		return nil
	}

	flow := m.Fingerprint.HttpFlows[0]
	if flow == nil {
		return nil
	}

	statusCode := flow.StatusCode
	if statusCode == 0 {
		statusCode = lowhttp.GetStatusCodeFromResponse(flow.ResponseHeader)
	}

	title := utils.ExtractTitleFromHTMLTitle(string(flow.ResponseBody), "")
	if title == "" {
		title = m.GetHtmlTitle()
	}

	return &PortFingerprintHTTP{
		URL:         fingerprintHTTPURL(m, flow),
		Title:       title,
		StatusCode:  statusCode,
		IsHTTPS:     flow.IsHTTPS,
		Server:      lowhttp.GetHTTPPacketHeader(flow.ResponseHeader, "Server"),
		ContentType: lowhttp.GetHTTPPacketHeader(flow.ResponseHeader, "Content-Type"),
	}
}

func fingerprintHTTPURL(m *fp.MatchResult, flow *fp.HTTPFlow) string {
	if m == nil || m.Fingerprint == nil {
		return ""
	}

	if len(m.Fingerprint.CPEFromUrls) > 0 {
		urls := make([]string, 0, len(m.Fingerprint.CPEFromUrls))
		for rawURL := range m.Fingerprint.CPEFromUrls {
			if strings.TrimSpace(rawURL) != "" {
				urls = append(urls, rawURL)
			}
		}
		sort.Strings(urls)
		if len(urls) > 0 {
			return urls[0]
		}
	}

	if flow == nil {
		return ""
	}

	host := lowhttp.GetHTTPPacketHeader(flow.RequestHeader, "Host")
	if host == "" {
		host = utils.HostPort(m.Target, m.Port)
	}

	path := "/"
	if requestLine := firstHTTPPacketLine(flow.RequestHeader); requestLine != "" {
		parts := strings.Fields(requestLine)
		if len(parts) >= 2 {
			if u, err := url.Parse(parts[1]); err == nil && u.IsAbs() {
				return u.String()
			}
			if strings.HasPrefix(parts[1], "/") {
				path = parts[1]
			}
		}
	}

	scheme := "http"
	if flow.IsHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func firstHTTPPacketLine(raw []byte) string {
	line := string(raw)
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

func newPortFingerprintTLS(m *fp.MatchResult, httpSummary *PortFingerprintHTTP) *PortFingerprintTLS {
	if m == nil || m.Fingerprint == nil {
		return nil
	}

	info := m.Fingerprint
	tlsSummary := &PortFingerprintTLS{
		Enabled: tlsServiceEnabled(info, httpSummary),
		Checked: info.CheckedTLS || len(info.TLSInspectResults) > 0,
	}

	if len(info.TLSInspectResults) > 0 {
		applyTLSInspectResult(tlsSummary, info.TLSInspectResults[0])
	}

	if !tlsSummary.Enabled && !tlsSummary.Checked {
		return nil
	}
	return tlsSummary
}

func tlsServiceEnabled(info *fp.FingerprintInfo, httpSummary *PortFingerprintHTTP) bool {
	if info == nil {
		return false
	}
	if len(info.TLSInspectResults) > 0 {
		return true
	}
	if httpSummary != nil && httpSummary.IsHTTPS {
		return true
	}
	return strings.Contains(strings.ToLower(info.ServiceName), "https")
}

func applyTLSInspectResult(summary *PortFingerprintTLS, result *netx.TLSInspectResult) {
	if summary == nil || result == nil {
		return
	}

	summary.SNI = result.ServerName
	summary.ServerName = result.ServerName
	summary.Protocol = result.Protocol
	if result.Version != 0 {
		summary.Version = tls.VersionName(result.Version)
	}
	if result.CipherSuite != 0 {
		summary.CipherSuite = tls.CipherSuiteName(result.CipherSuite)
	}

	if len(result.Raw) == 0 {
		return
	}
	cert, err := x509.ParseCertificate(result.Raw)
	if err != nil {
		return
	}
	summary.Issuer = cert.Issuer.String()
	summary.Subject = cert.Subject.String()
	summary.NotBefore = cert.NotBefore.Format(time.RFC3339Nano)
	summary.NotAfter = cert.NotAfter.Format(time.RFC3339Nano)
	summary.DNSNames = append([]string(nil), cert.DNSNames...)
	fingerprint := sha256.Sum256(cert.Raw)
	summary.FingerprintSHA256 = hex.EncodeToString(fingerprint[:])
}

func fingerprintDomains(m *fp.MatchResult, httpSummary *PortFingerprintHTTP) []string {
	if m == nil || m.Fingerprint == nil {
		return []string{}
	}

	var domains []string
	seen := map[string]struct{}{}
	addFingerprintDomain(&domains, seen, m.Target)
	addFingerprintDomain(&domains, seen, m.Fingerprint.Hostname)
	if httpSummary != nil {
		addFingerprintDomain(&domains, seen, httpSummary.URL)
	}
	for _, result := range m.Fingerprint.TLSInspectResults {
		if result == nil {
			continue
		}
		addFingerprintDomain(&domains, seen, result.ServerName)
		if len(result.Raw) > 0 {
			if cert, err := x509.ParseCertificate(result.Raw); err == nil {
				for _, dnsName := range cert.DNSNames {
					addFingerprintDomain(&domains, seen, dnsName)
				}
			}
		}
		for _, domain := range result.RelativeDomains {
			addFingerprintDomain(&domains, seen, domain)
		}
	}
	return domains
}

func addFingerprintDomain(domains *[]string, seen map[string]struct{}, raw string) {
	domain := normalizeFingerprintDomain(raw)
	if domain == "" {
		return
	}
	if net.ParseIP(utils.FixForParseIP(domain)) != nil {
		return
	}
	if _, ok := seen[domain]; ok {
		return
	}
	seen[domain] = struct{}{}
	*domains = append(*domains, domain)
}

func normalizeFingerprintDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		return strings.TrimSuffix(strings.Trim(parsed.Hostname(), "[]"), ".")
	}
	if host, _, err := net.SplitHostPort(raw); err == nil && host != "" {
		return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	}
	return strings.TrimSuffix(strings.Trim(raw, "[]"), ".")
}

type PortState struct {
	Host  string            `json:"host"`
	Port  int               `json:"port"`
	Proto fp.TransportProto `json:"proto"`
	State PortStateType     `json:"state"`
}

func NewScanTCPOpenPortResult(ip net.IP, port int, state PortStateType) (*ScanResult, error) {
	raw, err := json.Marshal(&PortState{
		Host:  ip.String(),
		Port:  port,
		Proto: fp.TCP,
		State: state,
	})
	if err != nil {
		return nil, errors.Errorf("marshal port state failed: %v", err)
	}

	return &ScanResult{
		Type:    ScanResult_PortState,
		Content: raw,
	}, nil
}
func NewScanProcessResult(process float64) (*ScanResult, error) {
	raw, err := json.Marshal(map[string]any{
		"process": process,
	})
	if err != nil {
		return nil, err
	}
	return &ScanResult{
		Type:    ScanResult_Process,
		Content: raw,
	}, nil
}

func (p *PortState) String() string {
	var prefix string
	switch p.State {
	case PortStateType_Closed:
		prefix = "CLOSED"
	case PortStateType_Open:
		prefix = "  OPEN"
	case PortStateType_Unknown:
		prefix = "UNKNOW"
	default:
		prefix = "UNKNOW"
	}
	return fmt.Sprintf("%v: [%v] %15s:%v", p.Proto, prefix, p.Host, p.Port)
}
