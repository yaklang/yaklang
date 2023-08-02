package yakit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type RiskParamsOpt func(r *Risk)

func RiskTypeToVerbose(i string) string {
	switch strings.ToLower(i) {
	case "random-port-trigger[tcp]":
		return "反连[TCP]-随机端口"
	case "random-port-trigger[udp]":
		return "反连[UDP]-随机端口"
	case "reverse":
		fallthrough
	case "reverse-":
		return "反连[unknown]"
	case "reverse-tcp":
		return "反连[TCP]"
	case "reverse-tls":
		return "反连[TLS]"
	case "reverse-rmi":
		return "反连[RMI]"
	case "reverse-rmi-handshake":
		return "反连[RMI握手]"
	case "reverse-http":
		return "反连[HTTP]"
	case "reverse-https":
		return "反连[HTTPS]"
	case "reverse-dns":
		return "反连[DNS]"
	case "reverse-ldap":
		return "反连[LDAP]"
	case "xss":
		return "XSS"
	case "sqli", "sqlinj", "sql-inj", "sqlinjection", "sql-injection":
		return "SQL注入"
	case "ssti":
		return "SSTI"
	case "ssrf":
		return "SSRF"
	case "rce":
		return "远程代码执行"
	case "lfi":
		return "本地文件包含(LFI)"
	case "rfi":
		return "远程文件包含(RFI)"
	}
	return strings.ToUpper(i)
}

func WithRiskParam_Payload(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.Payload = strconv.Quote(i)
	}
}

func WithRiskParam_Title(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.Title = i
	}
}

func WithRiskParam_TitleVerbose(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.TitleVerbose = i
	}
}

func WithRiskParam_Description(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.Description = i
	}
}

func WithRiskParam_YakitPluginName(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.FromYakScript = i
	}
}

func WithRiskParam_Solution(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.Solution = i
	}
}

func WithRiskParam_RiskType(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.RiskType = i
		r.RiskTypeVerbose = RiskTypeToVerbose(i)
	}
}

func WithRiskParam_RiskVerbose(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.RiskTypeVerbose = i
	}
}

func WithRiskParam_Parameter(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.Parameter = i
	}
}

func WithRiskParam_Token(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.ReverseToken = i
	}
}

func WithRiskParam_Request(i interface{}) RiskParamsOpt {
	return func(r *Risk) {
		r.QuotedRequest = utils.InterfaceToQuotedString(i)
	}
}

func WithRiskParam_Response(i interface{}) RiskParamsOpt {
	return func(r *Risk) {
		r.QuotedResponse = utils.InterfaceToQuotedString(i)
	}
}

func WithRiskParam_Details(i interface{}) RiskParamsOpt {
	return func(r *Risk) {
		if i == nil {
			return
		}

		details := utils.InterfaceToGeneralMap(i)
		if details != nil {
			requestRaw := utils.MapGetFirstRaw(
				details,
				"request", "req", "request_raw", "request_bytes", "request_str",
				"requestRaw", "requestBytes", "requestStr", "Request",
				"RequestRaw", "RequestBytes", "RequestStr", "REQUEST",
				"packet", "packetBytes", "http_request", "http", "http_packet",
				"httprequest", "httpreq", "httprequest", "HTTP", "HTTP_REQUEST", "HTTPREQUEST",
			)
			requestBytes := utils.InterfaceToBytes(requestRaw)
			var requestStr string
			if bytes.HasPrefix(requestBytes, []byte(`"`)) && bytes.HasSuffix(requestBytes, []byte(`"`)) {
				requestStr, _ = strconv.Unquote(string(requestBytes))
			} else {
				requestStr = string(requestBytes)
			}
			if requestStr != "" {
				r.QuotedRequest = strconv.Quote(requestStr)
			}

			responseRaw := utils.MapGetFirstRaw(
				details,
				"response", "rsp", "resp", "response_raw", "response_bytes", "response_str",
				"responseRaw", "responseBytes", "responseStr", "Response",
				"ResponseRaw", "ResponseBytes", "ResponseStr", "RESPONSE",
				"httprsp", "httpresponse", "http_response", "response_packet", "http_rsp",
				"response", "response_bytes", "HTTP_RESPONSE", "HTTPRESPONSE", "HTTP_RSP",
			)
			responseBytes := utils.InterfaceToBytes(responseRaw)
			var responseStr string
			if bytes.HasPrefix(responseBytes, []byte(`"`)) && bytes.HasSuffix(responseBytes, []byte(`"`)) {
				responseStr, _ = strconv.Unquote(string(responseBytes))
			} else {
				responseStr = string(responseBytes)
			}
			if responseStr != "" {
				r.QuotedResponse = strconv.Quote(responseStr)
			}

			payloadStr := utils.InterfaceToString(utils.MapGetFirstRaw(details, "payload", "payloads", "payloadStr", "payloadRaw", "Payload", "Payloads", "cmd", "command"))
			if payloadStr != "" {
				if strings.HasPrefix(payloadStr, `"`) && strings.HasSuffix(payloadStr, `"`) {
					raw, _ := strconv.Unquote(payloadStr)
					if raw != "" {
						payloadStr = raw
					}
				}
				r.Payload = payloadStr
			}
		}

		raw, err := json.Marshal(i)
		if err != nil {
			log.Error(err)
			return
		}
		r.Details = strconv.Quote(string(raw))
	}
}

func WithRiskParam_RuntimeId(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.RuntimeId = i
	}
}

func WithRiskParam_Potential(i bool) RiskParamsOpt {
	return func(r *Risk) {
		r.IsPotential = i
	}
}

func WithRiskParam_CVE(s string) RiskParamsOpt {
	return func(r *Risk) {
		r.CVE = s
	}
}

func WithRiskParam_Severity(i string) RiskParamsOpt {
	return func(r *Risk) {
		switch strings.TrimSpace(strings.ToLower(i)) {
		case "high":
			r.Severity = "high"
		case "critical", "panic", "fatal":
			r.Severity = "critical"
		case "warning", "warn", "middle", "medium":
			r.Severity = "warning"
		case "info", "debug", "trace", "fingerprint", "note", "fp":
			r.Severity = "info"
		case "low", "default":
			r.Severity = "low"
		default:
			r.Severity = "low"
		}
	}
}

func WithRiskParam_FromScript(i string) RiskParamsOpt {
	return func(r *Risk) {
		r.FromYakScript = i
	}
}

func CreateRisk(u string, opts ...RiskParamsOpt) *Risk {
	return _createRisk(u, opts...)
}

func _createRisk(u string, opts ...RiskParamsOpt) *Risk {
	r := &Risk{
		Hash: uuid.NewV4().String(),
	}

	if utils.IsIPv4(u) {
		r.IP = u
		r.IPInteger, _ = utils.IPv4ToUint64(u)
	} else {
		if strings.Contains(u, "://") {
			r.Url = u
		}
	}

	host, port, _ := utils.ParseStringToHostPort(u)
	if host != "" {
		r.Host = host
	}
	if port > 0 {
		r.Port = port
	}

	if r.IP == "" && r.Host != "" {
		if utils.IsIPv4(r.Host) {
			r.IP = r.Host
			r.IPInteger, _ = utils.IPv4ToUint64(r.Host)
		} else {
			r.IP = yakdns.LookupFirst(r.Host, yakdns.WithTimeout(3*time.Second))
		}
	}

	for _, opt := range opts {
		opt(r)
	}

	if r.Title == "" {
		r.Title = fmt.Sprintf("no title risk for target: %v", u)
	}

	if r.RuntimeId == "" {
		r.RuntimeId = os.Getenv(consts.YAK_RUNTIME_ID)
	}

	if r.RiskType == "" {
		r.RiskType = "info"
		r.RiskTypeVerbose = "信息[默认]"
	}

	return r
}

func NewRisk(u string, opts ...RiskParamsOpt) (*Risk, error) {
	r := _createRisk(u, opts...)
	return r, _saveRisk(r)
}

func SaveRisk(r *Risk) error {
	return _saveRisk(r)
}

func NewUnverifiedRisk(u string, token string, opts ...RiskParamsOpt) (*Risk, error) {
	r := _createRisk(u, opts...)
	r.WaitingVerified = true
	r.ReverseToken = token
	return r, _saveRisk(r)
}

func _saveRisk(r *Risk) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return utils.Errorf("no database connection")
	}

	cveDb := consts.GetGormCVEDatabase()
	if cveDb != nil {
		cveData, _ := cveresources.GetCVE(cveDb.Model(&cveresources.CVE{}), r.CVE)
		if cveData != nil {
			r.CveAccessVector = cveData.AccessVector
			r.CveAccessComplexity = cveData.AccessComplexity
		}
	}
	count := 0
	for {
		count++
		err := CreateOrUpdateRisk(db, r.Hash, r)
		if err != nil {
			if count < 20 {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Errorf("save risk failed: %s", err)
			return utils.Errorf("save risk record failed: %s", err)
		}
		return nil
	}

}

func NewPublicReverseProtoUrl(proto string) func(opts ...RiskParamsOpt) string {
	return func(opts ...RiskParamsOpt) string {
		addr := os.Getenv(consts.YAK_BRIDGE_REMOTE_REVERSE_ADDR)
		if addr == "" {
			return ""
		}

		token := utils.RandStringBytes(8)
		u := fmt.Sprintf("%v://%v/%v", proto, addr, token)
		_, err := NewUnverifiedRisk(u, token, opts...)
		if err != nil {
			log.Error(err)
		}
		return u
	}
}

func NewLocalReverseProtoUrl(proto string) func(opts ...RiskParamsOpt) string {
	return func(opts ...RiskParamsOpt) string {
		addr := os.Getenv(consts.YAK_BRIDGE_LOCAL_REVERSE_ADDR)
		if addr == "" {
			return ""
		}

		token := utils.RandStringBytes(8)
		u := fmt.Sprintf("%v://%v/%v", proto, addr, token)
		_, err := NewUnverifiedRisk(u, token, opts...)
		if err != nil {
			log.Error(err)
		}
		return u
	}
}

func HaveReverseRisk(token string) bool {
	if token == "" {
		return false
	}
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return false
	}

	var retryCount = 0
	for {
		retryCount++
		var count int
		if db := db.Model(&Risk{}).Where(
			"reverse_token LIKE ?", "%"+token+"%",
		).Where("waiting_verified = ?", false).Count(&count); db.Error != nil {
		}
		if count > 0 {
			return true
		}
		if retryCount > 5 {
			return false
		}
		time.Sleep(1 * time.Second)
	}
}

func ExtractTokenFromUrl(tokenUrl string) string {
	u, err := url.Parse(tokenUrl)
	if err != nil {
		return ""
	}

	token := strings.TrimLeft(u.EscapedPath(), "/")
	token = strings.TrimRight(token, "&/?")
	return token
}

func _fetBridgeAddrAndSecret() (string, string, error) {
	addr := os.Getenv(consts.YAK_BRIDGE_ADDR)
	secret := os.Getenv(consts.YAK_BRIDGE_SECRET)

	if addr == "" || secret == "" {
		return "", "", utils.Errorf("no yak bridge addr")
	}
	return addr, secret, nil
}

func NewDNSLogDomainWithContext(ctx context.Context) (domain string, token string, _ error) {
	var counter = 0
	for {
		counter++
		domain, token, err := cybertunnel.RequireDNSLogDomain(consts.GetDefaultPublicReverseServer())
		if err != nil {
			select {
			case <-ctx.Done():
				return "", "", err
			default:
			}
			if counter > 10 {
				return "", "", err
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return domain, token, nil
	}
}
func NewDNSLogDomain() (domain string, token string, _ error) {
	var counter = 0
	for {
		counter++
		domain, token, err := cybertunnel.RequireDNSLogDomain(consts.GetDefaultPublicReverseServer())
		if err != nil {
			if counter > 10 {
				return "", "", err
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return domain, token, nil
	}
}

func CheckDNSLogByToken(token string, timeout ...float64) ([]*tpb.DNSLogEvent, error) {
	var f float64
	if len(timeout) > 0 {
		f = timeout[0]
	}
	if f <= 0 {
		f = 5.0
	}
	var counter = 0
	for {
		counter++
		if counter > 3 {
			return nil, utils.Errorf("cannot found result for dnslog[%v]", token)
		}
		events, err := cybertunnel.QueryExistedDNSLogEventsEx(consts.GetDefaultPublicReverseServer(), token, f)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		for _, e := range events {
			NewRisk(
				"dnslog://"+e.RemoteAddr,
				WithRiskParam_Title(fmt.Sprintf(`DNSLOG[%v] - %v from: %v`, e.Type, e.Domain, e.RemoteAddr)),
				WithRiskParam_TitleVerbose(fmt.Sprintf(`DNSLOG 触发 - %v 源：%v`, e.Domain, e.RemoteAddr)),
				WithRiskParam_Details(e.Raw),
				WithRiskParam_RiskType(fmt.Sprintf("dns[%v]", e.Type)),
				WithRiskParam_RiskType(fmt.Sprint("dnslog")),
				WithRiskParam_Payload(e.Domain), WithRiskParam_Token(e.Token),
			)
		}
		if len(events) > 0 {
			return events, nil
		}
		time.Sleep(1 * time.Second)
	}
}

func NewRandomPortTrigger(opt ...RiskParamsOpt) (token string, addr string, _ error) {
	token = utils.RandStringBytes(8)
	addr, secret, err := _fetBridgeAddrAndSecret()
	if err != nil {
		return "", "", err
	}
	rsp, err := cybertunnel.RequirePortByToken(token, addr, secret, utils.TimeoutContextSeconds(5))
	if err != nil {
		return "", "", err
	}

	if rsp.GetPort() <= 0 {
		return "", "", utils.Errorf("error fetch available random port from bridge")
	}
	checkAddr := utils.HostPort(rsp.GetExternalIP(), rsp.GetPort())
	_, err = NewUnverifiedRisk(checkAddr, token, opt...)
	if err != nil {
		log.Errorf("create unverified risk failed: %s", err)
	}
	return token, checkAddr, nil
}

func CheckICMPTriggerByLength(i int) (*tpb.ICMPTriggerNotification, error) {
	addr, secret, err := _fetBridgeAddrAndSecret()
	if err != nil {
		return nil, err
	}

	event, err := cybertunnel.QueryICMPLengthTriggerNotifications(
		i, addr, secret, nil)
	if err != nil {
		return nil, err
	}

	NewRisk(
		event.CurrentRemoteAddr,
		WithRiskParam_RiskType("icmp-length-trigger[icmp]"),
		WithRiskParam_Title(
			fmt.Sprintf("ICMP Specific Length Trigger[bridge:%v] Detected %v", addr, event.CurrentRemoteAddr),
		),
		WithRiskParam_Title(
			fmt.Sprintf("检测到特定长度 ICMP 反连[bridge:%v] 来源：%v", addr, event.CurrentRemoteAddr),
		),
		WithRiskParam_Details(event),
		WithRiskParam_Severity("info"),
	)
	return event, nil
}

func CheckRandomTriggerByToken(t string) (*tpb.RandomPortTriggerEvent, error) {
	addr, secret, err := _fetBridgeAddrAndSecret()
	if err != nil {
		return nil, err
	}

	event, err := cybertunnel.QueryExistedRandomPortTriggerEvents(t, addr, secret, utils.TimeoutContextSeconds(5))
	if err != nil {
		return nil, err
	}

	if event.Timestamp-event.TriggerTimestamp >= 60 {
		return nil, utils.Errorf("no result in 60s!")
	}

	var maybeScanner = ""
	var maybeScannerVerbose = ""
	if event.CurrentRemoteCachedConnectionCount > 50 {
		maybeScanner = fmt.Sprintf(", Maybe Scanner [%v's connection count: %v in one minute]", event.RemoteIP, event.CurrentRemoteCachedConnectionCount)
		maybeScanner = fmt.Sprintf(" (疑似扫描器 [该 IP[%v] 一分钟内缓存 %v 个连接])", event.RemoteIP, event.CurrentRemoteCachedConnectionCount)
	}

	NewRisk(event.RemoteAddr,
		WithRiskParam_RiskType("random-port-trigger[tcp]"),
		WithRiskParam_Title(
			fmt.Sprintf("Random Port Trigger[bridge:%v] Detected %v%v", event.LocalPort, event.RemoteAddr, maybeScanner),
		),
		WithRiskParam_Title(
			fmt.Sprintf("检测到随机端口反连[bridge:%v] 来源：%v%v", event.LocalPort, event.RemoteAddr, maybeScannerVerbose),
		),
		WithRiskParam_Token(t),
		WithRiskParam_Details(event),
		WithRiskParam_Severity("info"),
	)
	return event, nil
}

//
//var (
//	RiskExports = map[string]interface{}{
//		"CreateRisk":                CreateRisk,
//		"Save":                      _saveRisk,
//		"NewRisk":                   NewRisk,
//		"NewUnverifiedRisk":         NewUnverifiedRisk,
//		"NewPublicReverseRMIUrl":    NewPublicReverseProtoUrl("rmi"),
//		"NewPublicReverseHTTPSUrl":  NewPublicReverseProtoUrl("https"),
//		"NewPublicReverseHTTPUrl":   NewPublicReverseProtoUrl("http"),
//		"NewLocalReverseRMIUrl":     NewLocalReverseProtoUrl("rmi"),
//		"NewLocalReverseHTTPSUrl":   NewLocalReverseProtoUrl("https"),
//		"NewLocalReverseHTTPUrl":    NewLocalReverseProtoUrl("http"),
//		"HaveReverseRisk":           HaveReverseRisk,
//		"NewRandomPortTrigger":      NewRandomPortTrigger,
//		"NewDNSLogDomain":           NewDNSLogDomain,
//		"CheckDNSLogByToken":        CheckDNSLogByToken,
//		"CheckRandomTriggerByToken": CheckRandomTriggerByToken,
//		"CheckICMPTriggerByLength":  CheckICMPTriggerByLength,
//		"ExtractTokenFromUrl":       ExtractTokenFromUrl,
//		"payload":                   WithRiskParam_Payload,
//		"title":                     WithRiskParam_Title,
//		"type":                      WithRiskParam_RiskType,
//		"titleVerbose":              WithRiskParam_TitleVerbose,
//		"typeVerbose":               WithRiskParam_RiskVerbose,
//		"parameter":                 WithRiskParam_Parameter,
//		"token":                     WithRiskParam_Token,
//		"details":                   WithRiskParam_Details,
//		"severity":                  WithRiskParam_Severity,
//		"level":                     WithRiskParam_Severity,
//		"fromYakScript":             WithRiskParam_FromScript,
//
//		// RandomPortTrigger
//
//	}
//)
