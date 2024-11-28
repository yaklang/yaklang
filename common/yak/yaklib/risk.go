package yaklib

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bot"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// NewRisk 创建一条漏洞记录结构体并保存到数据库中，第一个参数是目标URL，后面可以传入零个或多个选项参数，用于指定 risk 的结构
// Example:
// ```
// risk.NewRisk("http://example.com", risk.title("SQL注入漏洞"), risk.type("sqli"), risk.severity("high"), risk.description(""), risk.solution(""))
// ```
func YakitNewRiskBuilder(client *YakitClient) func(target string, opts ...yakit.RiskParamsOpt) {
	return func(target string, opts ...yakit.RiskParamsOpt) {
		risk, _ := yakit.NewRisk(target, opts...)
		if risk != nil {
			if botClient == nil {
				log.Info("start to create bot client")
				client := bot.FromEnv()
				if client != nil && len(client.Configs()) > 0 {
					botClient = client
				}
			}
			if botClient != nil {
				title := risk.TitleVerbose
				if title == "" {
					title = risk.Title
				}
				log.Infof("use bot notify risk: %s", risk.Title)
				botClient.SendMarkdown(fmt.Sprintf(`# Yakit 发现 Risks

风险标题：%v

风险目标：%v

`, title, risk.IP))
			}
		}
		client.Output(risk)
	}
}

// YieldRiskByTarget 根据目标(ip或ip:port)获取风险记录，返回风险记录的管道
// Example:
// ```
// for risk := range YieldRiskByTarget("example.com") {
// println(risk)
// }
// ```
func YieldRiskByTarget(target string) chan *schema.Risk {
	return yakit.YieldRisksByTarget(consts.GetGormProjectDatabase(), context.Background(), target)
}

// YieldRiskByRuntimeId 根据 RuntimeID 获取风险记录，返回风险记录的管道
// Example:
// ```
// for risk := range YieldRiskByRuntimeId("161c5372-3e75-46f6-a6bf-1a3182da625e") {
// println(risk)
// }
// ```
func YieldRiskByRuntimeId(runtimeId string) chan *schema.Risk {
	return yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)
}

// YieldRiskByCreateAt 根据创建时间戳获取风险记录，返回风险记录的管道
// Example:
// ```
// ts = time.Parse("2006-01-02 15:04:05", "2020-01-01 00:00:00")~.Unix()
// for risk := range YieldRiskByCreateAt(ts) {
// println(risk)
// }
// ```
func YieldRiskByCreateAt(timestamp int64) chan *schema.Risk {
	return yakit.YieldRisksByCreateAt(consts.GetGormProjectDatabase(), context.Background(), timestamp)
}

// YieldRiskByScriptName 根据插件名戳获取风险记录，返回风险记录的管道
// Example:
// ```
// for risk := range YieldRiskByScriptName("基础 XSS 检测") {
// println(risk)
// }
// ```
func YieldRiskByScriptName(scriptName string) chan *schema.Risk {
	return yakit.YieldRisksByScriptName(consts.GetGormProjectDatabase(), context.Background(), scriptName)
}

// DeleteRiskByTarget 根据目标(ip或ip:port)删除风险记录
// Example:
// ```
// DeleteRiskByTarget("example.com")
// ```
func DeleteRiskByTarget(addr string) error {
	return yakit.DeleteRiskByTarget(consts.GetGormProjectDatabase(), addr)
}

// DeleteRiskByID 根据风险记录ID删除风险记录
func DeleteRiskByID(id int64) error {
	return yakit.DeleteRiskByID(consts.GetGormProjectDatabase(), id)
}

// NewPublicReverseRMIUrl 返回一个公网 Bridge 的反向 RMI URL
// Example:
// ```
// url := NewPublicReverseRMIUrl()
// ```
func NewPublicReverseRMIUrl() string {
	return yakit.NewPublicReverseProtoUrl("rmi")()
}

// NewPublicReverseHTTPSUrl 返回一个公网 Bridge 的反向 HTTPS URL
// Example:
// ```
// url := NewPublicReverseHTTPSUrl()
// ```
func NewPublicReverseHTTPSUrl() string {
	return yakit.NewPublicReverseProtoUrl("https")()
}

// NewPublicReverseHTTPUrl 返回一个公网 Bridge 的反向 HTTP URL
// Example:
// ```
// url := NewPublicReverseHTTPUrl()
// ```
func NewPublicReverseHTTPUrl() string {
	return yakit.NewPublicReverseProtoUrl("http")()
}

// NewLocalReverseRMIUrl 返回一个本地 Bridge 的反向 RMI URL
// Example:
// ```
// url := NewLocalReverseRMIUrl()
// ```
func NewLocalReverseRMIUrl() string {
	return yakit.NewLocalReverseProtoUrl("rmi")()
}

// NewLocalReverseHTTPSUrl 返回一个本地 Bridge 的反向 HTTPS URL
// Example:
// ```
// url := NewLocalReverseHTTPSUrl()
// ```
func NewLocalReverseHTTPSUrl() string {
	return yakit.NewLocalReverseProtoUrl("https")()
}

// NewLocalReverseHTTPUrl 返回一个本地 Bridge 的反向 HTTP URL
// Example:
// ```
// url := NewLocalReverseHTTPUrl()
// ```
func NewLocalReverseHTTPUrl() string {
	return yakit.NewLocalReverseProtoUrl("http")()
}

var (
	botClient   *bot.Client
	RiskExports = map[string]interface{}{
		"CreateRisk":                yakit.CreateRisk,
		"Save":                      yakit.SaveRisk,
		"NewRisk":                   YakitNewRiskBuilder(GetYakitClientInstance()),
		"RegisterBeforeRiskSave":    yakit.RegisterBeforeRiskSave,
		"YieldRiskByTarget":         YieldRiskByTarget,
		"YieldRiskByRuntimeId":      YieldRiskByRuntimeId,
		"YieldRiskByCreateAt":       YieldRiskByCreateAt,
		"YieldRiskByScriptName":     YieldRiskByScriptName,
		"DeleteRiskByTarget":        DeleteRiskByTarget,
		"DeleteRiskByID":            DeleteRiskByID,
		"NewUnverifiedRisk":         yakit.NewUnverifiedRisk,
		"NewPublicReverseRMIUrl":    NewPublicReverseRMIUrl,
		"NewPublicReverseHTTPSUrl":  NewPublicReverseHTTPSUrl,
		"NewPublicReverseHTTPUrl":   NewPublicReverseHTTPUrl,
		"NewLocalReverseRMIUrl":     NewLocalReverseRMIUrl,
		"NewLocalReverseHTTPSUrl":   NewLocalReverseHTTPSUrl,
		"NewLocalReverseHTTPUrl":    NewLocalReverseHTTPUrl,
		"HaveReverseRisk":           yakit.HaveReverseRisk,
		"NewRandomPortTrigger":      yakit.NewRandomPortTrigger,
		"NewDNSLogDomain":           yakit.NewDNSLogDomain,
		"NewHTTPLog":                yakit.NewHTTPLog,
		"CheckDNSLogByToken":        yakit.YakitNewCheckDNSLogByToken(yakit.YakitPluginInfo{}),
		"CheckHTTPLogByToken":       yakit.YakitNewCheckHTTPLogByToken(yakit.YakitPluginInfo{}),
		"CheckRandomTriggerByToken": yakit.YakitNewCheckRandomTriggerByToken(yakit.YakitPluginInfo{}),
		"CheckICMPTriggerByLength":  yakit.YakitNewCheckICMPTriggerByLength(yakit.YakitPluginInfo{}),
		"CheckServerReachable":      yakit.CheckServerReachable,
		"ExtractTokenFromUrl":       yakit.ExtractTokenFromUrl,
		"payload":                   yakit.WithRiskParam_Payload,
		"title":                     yakit.WithRiskParam_Title,
		"type":                      yakit.WithRiskParam_RiskType,
		"titleVerbose":              yakit.WithRiskParam_TitleVerbose,
		"description":               yakit.WithRiskParam_Description,
		"solution":                  yakit.WithRiskParam_Solution,
		"typeVerbose":               yakit.WithRiskParam_RiskVerbose,
		"parameter":                 yakit.WithRiskParam_Parameter,
		"token":                     yakit.WithRiskParam_Token,
		"details":                   yakit.WithRiskParam_Details,
		"request":                   yakit.WithRiskParam_Request,
		"response":                  yakit.WithRiskParam_Response,
		"runtimeId":                 yakit.WithRiskParam_RuntimeId,
		"potential":                 yakit.WithRiskParam_Potential,
		"cve":                       yakit.WithRiskParam_CVE,
		"severity":                  yakit.WithRiskParam_Severity,
		"level":                     yakit.WithRiskParam_Severity,
		"fromYakScript":             yakit.WithRiskParam_FromScript,
		"ignore":                    yakit.WithRiskParam_Ignore,

		// RandomPortTrigger

	}
)
