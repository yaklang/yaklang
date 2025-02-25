package yaklib

import (
	"context"
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type CustomDNSLog struct {
	mode       string
	scriptName string
	domain     string
	token      string
	isLocal    bool
	timeout    float64
}

type IEngine interface {
	ExecuteEx(code string, params map[string]interface{}) (*antlr4yak.Engine, error)
}

var EngineInterface IEngine

func SetEngineInterface(engine IEngine) {
	EngineInterface = engine
}

func NewCustomDNSLog(opts ..._dnslogConfigOpt) *CustomDNSLog {
	config := &_dnslogConfig{
		mode:    "",
		isLocal: false,
	}
	for _, r := range opts {
		r(config)
	}
	return &CustomDNSLog{
		mode:       config.mode,
		scriptName: config.scriptName,
		isLocal:    config.isLocal,
		timeout:    config.timeout,
	}
}

func (c *CustomDNSLog) GetSubDomainAndToken() (string, string, error) {
	// 就不给那么多次了吧
	const maxAttempts = 5
	var (
		domain, token, mode string
		err                 error
	)

	getDomainAndToken := func() error {
		if c.isLocal {
			if c.scriptName != "" {
				domain, token, mode, err = customGet(c.scriptName)
			} else {
				domain, token, mode, err = cybertunnel.RequireDNSLogDomainByLocal(c.mode)
			}
		} else {
			domain, token, mode, err = cybertunnel.RequireDNSLogDomainByRemote(consts.GetDefaultPublicReverseServer(), c.mode)
		}
		return err
	}

	for i := 0; i < maxAttempts; i++ {
		if err = getDomainAndToken(); err == nil {
			c.mode = mode
			c.token = token
			c.domain = domain
			return domain, token, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return "", "", err
}

func (c *CustomDNSLog) CheckDNSLogByToken() ([]*tpb.DNSLogEvent, error) {
	if c.token == "" {
		return nil, fmt.Errorf("token is empty")
	}
	const maxAttempts = 3
	var f float64
	if c.timeout != 0 {
		f = c.timeout
	}
	if f <= 0 {
		f = 5.0
	}

	var (
		events []*tpb.DNSLogEvent
		err    error
	)

	getEvents := func() error {
		if c.isLocal {
			if c.scriptName != "" {
				events, err = customCheck(c.scriptName, c.token, c.mode, f)
			} else {
				events, err = cybertunnel.QueryExistedDNSLogEventsByLocalEx(c.token, c.mode, f)
			}
		} else {
			events, err = cybertunnel.QueryExistedDNSLogEventsEx(consts.GetDefaultPublicReverseServer(), c.token, c.mode, f)
		}
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("no events found")
		}
		return nil
	}

	for i := 0; i < maxAttempts; i++ {
		if err = getEvents(); err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("cannot found result for dnslog[%v]: %w", c.token, err)
	}

	for _, e := range events {
		yakit.NewRisk(
			"dnslog://"+e.RemoteAddr,
			yakit.WithRiskParam_Title(fmt.Sprintf(`DNSLOG[%v] - %v from: %v`, e.Type, e.Domain, e.RemoteAddr)),
			yakit.WithRiskParam_TitleVerbose(fmt.Sprintf(`DNSLOG 触发 - %v 源：%v`, e.Domain, e.RemoteAddr)),
			yakit.WithRiskParam_Details(e.Raw),
			yakit.WithRiskParam_Description("DNSLOG是一种回显机制，常用于在某些漏洞无法回显但可以发起DNS请求的情况下，利用此方式外带数据，以解决某些漏洞由于无回显而难以利用或检测的问题。 主要利用场景有SQL盲注、无回显的命令执行、无回显的SSRF、JAVA反序列化等。"),
			yakit.WithRiskParam_RiskType(fmt.Sprintf("dns[%v]", e.Type)),
			yakit.WithRiskParam_Payload(e.Domain), yakit.WithRiskParam_Token(e.Token),
		)
	}
	return events, nil
}

func customGet(name string) (string, string, string, error) {
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
	if err != nil {
		return "", "", "", err
	}

	engine, err := EngineInterface.ExecuteEx(script.Content, map[string]interface{}{
		"YAK_FILENAME": name,
	})
	if err != nil {
		return "", "", "", utils.Errorf("execute file %s code failed: %s", name, err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "requireDomain", []interface{}{})
	if err != nil {
		return "", "", "", utils.Errorf("import %v' s handle failed: %s", name, err)
	}
	var domain, token string
	domain = utils.InterfaceToStringSlice(result)[0]
	token = utils.InterfaceToStringSlice(result)[1]
	mode := "custom"
	return domain, token, mode, nil
}

func customCheck(name, token, mode string, timeout ...float64) ([]*tpb.DNSLogEvent, error) {
	var events []*tpb.DNSLogEvent
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
	if err != nil {
		return nil, err
	}

	engine, err := EngineInterface.ExecuteEx(script.Content, map[string]interface{}{
		"YAK_FILENAME": name,
	})
	if err != nil {
		return nil, utils.Errorf("execute file %s code failed: %s", name, err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "getResults", []interface{}{token})
	if err != nil {
		return nil, utils.Errorf("import %v' s handle failed: %s", name, err)
	}
	for _, v := range utils.InterfaceToSliceInterface(result) {
		event := utils.InterfaceToMapInterface(v)
		raw := []byte(spew.Sdump(event))
		e := &tpb.DNSLogEvent{
			Type:       utils.MapGetString(event, "Type"),
			Token:      utils.MapGetString(event, "Token"),
			Domain:     utils.MapGetString(event, "Domain"),
			RemoteAddr: utils.MapGetString(event, "RemoteAddr"),
			RemoteIP:   utils.MapGetString(event, "RemoteIP"),
			Raw:        raw,
			Timestamp:  utils.MapGetInt64(event, "Timestamp"),
		}
		events = append(events, e)
	}
	return events, nil
}

func queryCustomScript() {
}

var DNSLogExports = map[string]interface{}{
	"NewCustomDNSLog":   NewCustomDNSLog,
	"QueryCustomScript": queryCustomScript,
	"LookupFirst":       netx.LookupFirst,
	"random":            randomDNSLogPlatforms,
	"mode":              setMode,
	"local":             setLocal,
	"script":            setScript,
}

type _dnslogConfig struct {
	isLocal    bool
	mode       string
	scriptName string
	timeout    float64
}

type _dnslogConfigOpt func(config *_dnslogConfig)

func randomDNSLogPlatforms() _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.mode = "*"
	}
}

func setMode(mode string) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.mode = mode
	}
}

func setLocal(isLocal bool) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.isLocal = isLocal
	}
}

func setQueryTimeout(t float64) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.timeout = t
	}
}

func setScript(name string) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.scriptName = name
		config.isLocal = true
	}
}

var WithDNSLog_SetScript = setScript
