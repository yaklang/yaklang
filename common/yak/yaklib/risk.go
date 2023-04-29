package yaklib

import (
	"context"
	"fmt"
	"yaklang/common/consts"
	"yaklang/common/log"
	"yaklang/common/utils/bot"
	"yaklang/common/yakgrpc/yakit"
	"sync"
)

var riskCounter int
var _riskCounterLock = new(sync.Mutex)

func addCounter() int {
	_riskCounterLock.Lock()
	defer _riskCounterLock.Unlock()
	riskCounter++
	return riskCounter
}

func yakitNewRisk(target string, opts ...yakit.RiskParamsOpt) {
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
		yakitStatusCard("漏洞/风险", fmt.Sprint(addCounter()))
		yakitOutputHelper(risk)
	}
}

var botClient *bot.Client
var (
	RiskExports = map[string]interface{}{
		"CreateRisk": yakit.CreateRisk,
		"Save":       yakit.SaveRisk,
		"NewRisk":    yakitNewRisk,
		"YieldRiskByTarget": func(target string) chan *yakit.Risk {
			return yakit.YieldRisksByTarget(consts.GetGormProjectDatabase(), context.Background(), target)
		},
		"YieldRiskByRuntimeId": func(runtimeId string) chan *yakit.Risk {
			return yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)
		},
		"YieldRiskByCreateAt": func(timestamp int64) chan *yakit.Risk {
			return yakit.YieldRisksByCreateAt(consts.GetGormProjectDatabase(), context.Background(), timestamp)
		},
		"NewUnverifiedRisk":         yakit.NewUnverifiedRisk,
		"NewPublicReverseRMIUrl":    yakit.NewPublicReverseProtoUrl("rmi"),
		"NewPublicReverseHTTPSUrl":  yakit.NewPublicReverseProtoUrl("https"),
		"NewPublicReverseHTTPUrl":   yakit.NewPublicReverseProtoUrl("http"),
		"NewLocalReverseRMIUrl":     yakit.NewLocalReverseProtoUrl("rmi"),
		"NewLocalReverseHTTPSUrl":   yakit.NewLocalReverseProtoUrl("https"),
		"NewLocalReverseHTTPUrl":    yakit.NewLocalReverseProtoUrl("http"),
		"HaveReverseRisk":           yakit.HaveReverseRisk,
		"NewRandomPortTrigger":      yakit.NewRandomPortTrigger,
		"NewDNSLogDomain":           yakit.NewDNSLogDomain,
		"CheckDNSLogByToken":        yakit.CheckDNSLogByToken,
		"CheckRandomTriggerByToken": yakit.CheckRandomTriggerByToken,
		"CheckICMPTriggerByLength":  yakit.CheckICMPTriggerByLength,
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

		// RandomPortTrigger

	}
)
