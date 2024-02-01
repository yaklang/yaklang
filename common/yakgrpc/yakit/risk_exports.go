package yakit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/samber/lo"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

type (
	RiskParamsOpt func(r *Risk)
	riskType      struct {
		Types   []string
		Verbose string
	}
)

var (
	riskTypeVerboses = []*riskType{
		{Types: []string{"sqli", "sqlinj", "sql-inj", "sqlinjection", "sql-injection"}, Verbose: "SQL注入"},
		{Types: []string{"xss"}, Verbose: "XSS"},
		{Types: []string{"rce", "rce-command"}, Verbose: "命令执行/注入"},
		{Types: []string{"rce-code"}, Verbose: "代码执行/注入"},
		{Types: []string{"lfi", "file-read", "file-download"}, Verbose: "文件包含/读取/下载"},
		{Types: []string{"rfi"}, Verbose: "远程文件包含"},
		{Types: []string{"file-write", "file-upload"}, Verbose: "文件写入/上传"},
		{Types: []string{"xxe"}, Verbose: "XML外部实体攻击"},
		{Types: []string{"unserialize", "deserialization"}, Verbose: "反序列化"},
		{Types: []string{"unath", "unauth-access"}, Verbose: "未授权访问"},
		{Types: []string{"path-traversal"}, Verbose: "路径遍历"},
		{Types: []string{"info-exposure", "information-exposure"}, Verbose: "敏感信息泄漏"},
		{Types: []string{"auth-bypass", "authentication-bypass"}, Verbose: "身份验证绕过"},
		{Types: []string{"privilege-escalation"}, Verbose: "垂直/水平权限提升"},
		{Types: []string{"logic"}, Verbose: "逻辑漏洞"},
		{Types: []string{"insecure-default"}, Verbose: "默认配置漏洞"},
		{Types: []string{"weak-password", "weak-credential"}, Verbose: "弱口令"},
		{Types: []string{"compliance-test"}, Verbose: "合规检测"},
		{Types: []string{"ssti"}, Verbose: "SSTI"},
		{Types: []string{"ssrf"}, Verbose: "SSRF"},
		{Types: []string{"csrf"}, Verbose: "CSRF"},
		{Types: []string{"random-port-trigger[tcp]"}, Verbose: "反连[TCP]-随机端口"},
		{Types: []string{"random-port-trigger[udp]"}, Verbose: "反连[UDP]-随机端口"},
		{Types: []string{"reverse", "reverse-"}, Verbose: "反连[unknown]"},
		{Types: []string{"reverse-tcp"}, Verbose: "反连[TCP]"},
		{Types: []string{"reverse-tls"}, Verbose: "反连[TLS]"},
		{Types: []string{"reverse-rmi"}, Verbose: "反连[RMI]"},
		{Types: []string{"reverse-rmi-handshake"}, Verbose: "反连[RMI握手]"},
		{Types: []string{"reverse-http"}, Verbose: "反连[HTTP]"},
		{Types: []string{"reverse-https"}, Verbose: "反连[HTTPS]"},
		{Types: []string{"reverse-dns"}, Verbose: "反连[DNS]"},
		{Types: []string{"reverse-ldap"}, Verbose: "反连[LDAP]"},
	}
	RiskTypes = make([]string, 0)
)

func init() {
	for _, t := range riskTypeVerboses {
		RiskTypes = append(RiskTypes, t.Types...)
	}
}

func RiskTypeToVerbose(i string) string {
	i = strings.ToLower(i)
	for _, t := range riskTypeVerboses {
		if lo.Contains(t.Types, i) {
			return t.Verbose
		}
	}
	return "其他"
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
		r.RiskTypeVerbose = RiskTypeToVerbose(i)
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

const MaxSize = 2 << 20 // 2MB

func limitSize(s string, maxSize int) string {
	if len(s) <= maxSize {
		return s
	}

	i := 0
	for size := 0; size < maxSize-3; {
		r, runeSize := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError {
			break
		}
		if size+runeSize > maxSize {
			break
		}
		i += runeSize
		size += runeSize
	}

	temp := make([]byte, i)
	copy(temp, s[:i])

	return string(temp) + "..."
}

func WithRiskParam_Request(i interface{}) RiskParamsOpt {
	data := utils.InterfaceToString(i)
	data = limitSize(data, MaxSize)
	return func(r *Risk) {
		r.QuotedRequest = utils.InterfaceToQuotedString(data)
	}
}

func WithRiskParam_Response(i interface{}) RiskParamsOpt {
	data := utils.InterfaceToString(i)
	data = limitSize(data, MaxSize)
	return func(r *Risk) {
		r.QuotedResponse = utils.InterfaceToQuotedString(data)
	}
}

func WithRiskParam_Details(i interface{}) RiskParamsOpt {
	return func(r *Risk) {
		if i == nil {
			return
		}

		details := utils.InterfaceToGeneralMap(i)
		if details != nil {
			// 遍历 details map 并检查每个值的大小
			for key, value := range details {
				valueStr := utils.InterfaceToString(value)
				if len(valueStr) > MaxSize {
					// 如果值的大小超过2MB，裁剪它
					details[key] = limitSize(valueStr, MaxSize)
				}
			}
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

		raw, err := json.Marshal(details)
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

func WithRiskParam_Ignore(i bool) RiskParamsOpt {
	return func(r *Risk) {
		r.Ignore = true
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
			r.IP = netx.LookupFirst(r.Host, netx.WithTimeout(3*time.Second))
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

var (
	beforeRiskSave      []func(*Risk)
	beforeRiskSaveMutex = new(sync.Mutex)
)

func RegisterBeforeRiskSave(f func(*Risk)) {
	beforeRiskSaveMutex.Lock()
	defer beforeRiskSaveMutex.Unlock()
	beforeRiskSave = append(beforeRiskSave, func(risk *Risk) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("risk save callback error: %v", err)
			}
		}()
		f(risk)
	})
}

func _saveRisk(r *Risk) error {
	if r.Ignore {
		log.Infof("ignore risk: %v", r.Title)
		return nil
	}

	beforeRiskSaveMutex.Lock()
	defer beforeRiskSaveMutex.Unlock()
	for _, m := range beforeRiskSave {
		m(r)
	}

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
	if r.Description == "" && r.Solution == "" {
		r.Description, r.Solution = SolutionAndDescriptionByCWE(r.FromYakScript, r.RiskTypeVerbose, r.TitleVerbose)
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
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return false
	}

	retryCount := 0
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
	counter := 0
	for {
		counter++
		domain, token, _, err := cybertunnel.RequireDNSLogDomainByRemote(consts.GetDefaultPublicReverseServer(), "")
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
	counter := 0
	for {
		counter++
		domain, token, _, err := cybertunnel.RequireDNSLogDomainByRemote(consts.GetDefaultPublicReverseServer(), "")
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
	counter := 0
	for {
		counter++
		if counter > 3 {
			return nil, utils.Errorf("cannot found result for dnslog[%v]", token)
		}
		events, err := cybertunnel.QueryExistedDNSLogEventsEx(consts.GetDefaultPublicReverseServer(), token, "", f)
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

	maybeScanner := ""
	maybeScannerVerbose := ""
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

func SolutionAndDescriptionByCWE(FromYakScript, RiskTypeVerbose, TitleVerbose string) (description, solution string) {
	riskTypeList := map[string]int{
		"SQL":    89,
		"XSS":    79,
		"命令执行":   77,
		"命令注入":   77,
		"代码执行":   94,
		"代码注入":   94,
		"CSRF":   352,
		"文件包含":   41,
		"文件读取":   41,
		"文件下载":   41,
		"文件写入":   434,
		"文件上传":   434,
		"XXE":    91,
		"XML":    91,
		"反序列化":   502,
		"未授权访问":  552,
		"路径遍历":   22,
		"敏感信息泄漏": 200,
		"身份验证错误": 305,
		"权限提升":   271,
		"业务逻辑漏洞": 840,
		"默认配置漏洞": 1188,
		"弱口令":    1391,
		"SSRF":   918,
	}
	for k, v := range riskTypeList {
		if strings.Contains(FromYakScript, k) || strings.Contains(RiskTypeVerbose, k) || strings.Contains(TitleVerbose, k) {
			cweDb := consts.GetGormCVEDatabase()
			if cweDb != nil {
				cweData, _ := cveresources.GetCWEById(cweDb, v)
				if cweData != nil {
					description = cweData.DescriptionZh
					solution = cweData.ExtendedDescriptionZh
					return description, solution
				}
			}
		}
	}
	return "", ""
}
