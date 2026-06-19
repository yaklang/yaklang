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

// NewCustomDNSLog 创建一个 DNSLog 客户端，用于申请 DNSLog 域名并查询 DNS 回连记录
// 在 yak 中通过 dnslog.NewCustomDNSLog 调用，可通过选项设置平台、是否本地、脚本等
// 参数:
//   - opts: 可选配置项，如 dnslog.mode、dnslog.local、dnslog.script、dnslog.random
//
// 返回值:
//   - DNSLog 客户端对象，可调用 GetSubDomainAndToken/CheckDNSLogByToken 等方法
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 DNSLog 平台/反连服务
// client = dnslog.NewCustomDNSLog(dnslog.random())
// domain, token = client.GetSubDomainAndToken()~
// println("dnslog domain:", domain)
// // 触发对 domain 的 DNS 请求后查询回连事件
// events = client.CheckDNSLogByToken()~
// ```
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

// QueryCustomScript 是为自定义 DNSLog 脚本预留的占位函数，当前不执行任何操作
// 在 yak 中通过 dnslog.QueryCustomScript 调用
//
// Example:
// ```
// // 该示例为示意性用法：占位接口，无实际副作用
// dnslog.QueryCustomScript()
// ```
func queryCustomScript() {
	defer func() {}()
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

// random 设置 DNSLog 客户端随机选择可用平台(mode 设为通配)
// 在 yak 中通过 dnslog.random 调用
// 返回值:
//   - 一个 dnslog.NewCustomDNSLog 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：随机选择 DNSLog 平台
// client = dnslog.NewCustomDNSLog(dnslog.random())
// ```
func randomDNSLogPlatforms() _dnslogConfigOpt {
	mode := "*"
	return func(config *_dnslogConfig) {
		config.mode = mode
	}
}

// mode 指定 DNSLog 使用的平台名称
// 在 yak 中通过 dnslog.mode 调用
// 参数:
//   - mode: DNSLog 平台标识字符串
//
// 返回值:
//   - 一个 dnslog.NewCustomDNSLog 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定 DNSLog 平台
// client = dnslog.NewCustomDNSLog(dnslog.mode("dnslog.cn"))
// ```
func setMode(mode string) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.mode = mode
	}
}

// local 设置是否使用本地反连服务来申请与查询 DNSLog
// 在 yak 中通过 dnslog.local 调用
// 参数:
//   - isLocal: 是否使用本地模式
//
// 返回值:
//   - 一个 dnslog.NewCustomDNSLog 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用本地反连服务
// client = dnslog.NewCustomDNSLog(dnslog.local(true))
// ```
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

// script 指定用于申请与查询 DNSLog 的自定义 yak 脚本名称，并自动切换为本地模式
// 在 yak 中通过 dnslog.script 调用
// 参数:
//   - name: 自定义 DNSLog 脚本的名称
//
// 返回值:
//   - 一个 dnslog.NewCustomDNSLog 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用自定义脚本驱动 DNSLog
// client = dnslog.NewCustomDNSLog(dnslog.script("my-dnslog-script"))
// ```
func setScript(name string) _dnslogConfigOpt {
	return func(config *_dnslogConfig) {
		config.scriptName = name
		config.isLocal = true
	}
}

var WithDNSLog_SetScript = setScript
