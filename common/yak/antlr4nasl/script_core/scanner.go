package script_core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
	utils2 "github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func NaslScan(hosts, ports string, opts ...NaslScriptConfigOptFunc) chan *NaslKBs {
	config := NewNaslScriptConfig()
	for _, opt := range opts {
		opt(config)
	}
	engine := NewScriptEngineWithConfig(config)
	engine.Debug(true)
	log.Infof("Loaded script total: %v", len(engine.scripts))
	engine.proxies = config.proxies
	riskHandle := config.riskHandle
	PatchEngine(engine)

	engine.AddMethodHook("build_detection_report", func(origin NaslBuildInMethod, ctx *ExecContext, params *executor.NaslBuildInMethodParam) (any, error) {
		scriptObj := ctx.ScriptObj
		app := params.GetParamByName("app", "").String()
		version := params.GetParamByName("version", "").String()
		install := params.GetParamByName("install", "").String()
		cpe := params.GetParamByName("cpe", "").String()
		concluded := params.GetParamByName("concluded", "__empty__").String()
		if strings.TrimSpace(concluded) == "" || concluded == "Concluded from:" || concluded == "unknown" {
			return origin(ctx, params)
		}
		riskType := ""
		if v, ok := utils2.ActToChinese[scriptObj.Category]; ok {
			riskType = v
		} else {
			riskType = scriptObj.Category
		}
		source := "[NaslScript] " + scriptObj.ScriptName
		concludedUrl := params.GetParamByName("concludedUrl", "").String()
		solution := utils.MapGetString(scriptObj.Tags, "solution")
		summary := utils.MapGetString(scriptObj.Tags, "summary")
		cve := strings.Join(scriptObj.CVE, ", ")
		//xrefStr := ""
		//for k, v := range engine.scriptObj.Xrefs {
		//	xrefStr += fmt.Sprintf("\n Reference: %s(%s)", v, k)
		//}
		title := fmt.Sprintf("检测目标存在 [%s] 应用，版本号为 [%s]", app, version)
		if cve != "" {
			title += fmt.Sprintf(", CVE: %s", summary)
		}
		risk, _ := yakit.NewRisk(ctx.Host,
			yakit.WithRiskParam_Title(title),
			yakit.WithRiskParam_RiskType(riskType),
			yakit.WithRiskParam_Severity("low"),
			yakit.WithRiskParam_FromScript(source),
			yakit.WithRiskParam_Description(summary),
			yakit.WithRiskParam_Solution(solution),
			yakit.WithRiskParam_Details(map[string]any{
				"app":          app,
				"version":      version,
				"install":      install,
				"cpe":          cpe,
				"concluded":    concluded,
				"source":       source,
				"cve":          cve,
				"concludedUrl": concludedUrl,
			}),
		)
		if riskHandle != nil {
			riskHandle(risk)
		}
		return origin(ctx, params)
	})
	// 需要把ACT_SCAN的脚本都patch一遍
	//engine.AddNaslLibPatch("gb_altn_mdaemon_http_detect.nasl", func(code string) string {
	//	codeLines := strings.Split(code, "\n")
	//	if len(codeLines) > 55 {
	//		codeLines[55] = "if ((res =~ \"MDaemon[- ]Webmail\" || res =~ \"Server\\s*:\\s*WDaemon\") && \"WorldClient.dll\" >< res) {"
	//		code = strings.Join(codeLines, "\n")
	//	}
	//	return code
	//})
	resultCh := engine.Scan(hosts, ports)
	res := make(chan *NaslKBs)
	go func() {
		defer close(res)
		for r := range resultCh {
			res <- r.Kbs
		}
	}()
	return res
}

// 临时的，用于测试
func ServiceScan(ctx *ExecContext) {
	hosts := ctx.Host
	ports := ctx.Ports
	result := []*fp.MatchResult{}
	os.Setenv("YAKMODE", "vm")
	yakEngine := yaklang.New()

	yakEngine.SetVars(map[string]any{
		"addRes": func(res *fp.MatchResult) {
			result = append(result, res)
		},
		"hosts": hosts,
		"ports": ports,
	})
	err := yakEngine.SafeEval(context.Background(), `

getPingScan = func() {
	return ping.Scan(hosts,ping.timeout(5), ping.concurrent(10)) 
}

res, err := servicescan.ScanFromPing(
	getPingScan(), 
	ports)
die(err)

for result = range res {
	if result.IsOpen(){
		addRes(result)	
	}
}

`)
	if err != nil {
		log.Errorf("scan error: %v", err)
	}
	ctx.Kbs.SetKB("Host/scanned", 1)
	openPorts := []int{}
	portInfos := []*fp.MatchResult{}
	for _, result := range result {
		if result.State == fp.OPEN {
			fingerprint := result.Fingerprint
			openPorts = append(openPorts, result.Port)
			portInfos = append(portInfos, result)
			ctx.Kbs.SetKB(fmt.Sprintf("Ports/%s/%d", result.GetProto(), result.Port), 1)
			if fingerprint.ServiceName != "" {
				var serverName string
				if fingerprint.ServiceName == "http" {
					serverName = "www"
				} else {
					serverName = fingerprint.ServiceName
				}
				ctx.Kbs.SetKB(fmt.Sprintf("Services/%s", serverName), fingerprint.Port)
				ctx.Kbs.SetKB(fmt.Sprintf("Known/%s/%d", fingerprint.Proto, fingerprint.Port), fingerprint.ServiceName)
			}
			if fingerprint.Version != "" {
				ctx.Kbs.SetKB(fmt.Sprintf("Version/%s/%d", fingerprint.Proto, fingerprint.Port), fingerprint.Version)
			}
			for _, cpe := range fingerprint.CPEs {
				ctx.Kbs.SetKB(fmt.Sprintf("APP/%s/%d", fingerprint.Proto, fingerprint.Port), cpe)
			}
		}
	}
	// 缺少os finger_print、tcp_seq_index、ipidseq、Traceroute
	ctx.Kbs.SetKB("Host/port_infos", portInfos)
}
