package script_core

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

//func BuildInMethodCheck(engine *ScriptEngine) {
//	includeLibCodes := []string{}
//	missMethod := map[string]struct{}{}
//	naslLibPath := engine.naslLibsPath
//	files, err := utils.GetAllFiles(naslLibPath)
//	if err != nil {
//		panic(err)
//	}
//	for _, file := range files {
//		fileName := filepath.Base(file)
//		if !strings.HasSuffix(fileName, ".inc") {
//			continue
//		}
//		includeLibCodes = append(includeLibCodes, fmt.Sprintf(`include("%s");`, fileName))
//	}
//	err = engine.SafeEval(strings.Join(includeLibCodes, "\n"))
//	if err != nil {
//		panic(err)
//	}
//	for script, _ := range engine.scripts {
//		engine.compiler.RegisterVisitHook("a", func(c *visitors.Compiler, ctx antlr.ParserRuleContext) {
//			if v, ok := ctx.(*nasl.IdentifierExpressionContext); ok {
//				id := v.GetText()
//				hasMethod := false
//				if _, ok := NaslLib[id]; ok {
//					hasMethod = true
//				}
//				if _, ok := lib.NaslBuildInNativeMethod[id]; ok {
//					hasMethod = true
//				}
//				if _, ok := engine.GetVirtualMachine().GetVar(id); ok {
//					hasMethod = true
//				}
//				if !hasMethod {
//					missMethod[id] = struct{}{}
//				}
//			}
//		})
//		err := engine.Compile(script)
//		if err != nil {
//			panic(err)
//		}
//		spew.Dump(missMethod)
//	}
//}

//	func TestScriptLib(t *testing.T) {
//		engine := NewNaslExecutor()
//		engine.Debug()                                       // 开启调试模式，脚本退出时会打印调试信息
//		engine.Init()                                        // 导入内置原生库
//		InitPluginGroup(engine)                              // 初始化插件组
//		PatchEngine(engine)                                  // 一些库缺少函数
//		engine.SetIncludePath("/Users/z3/nasl/nasl-plugins") // 设置nasl依赖库位置
//		engine.LoadGroup(PluginGroupApache)
//
//		//获取所有脚本的依赖
//		libs := map[string]struct{}{}
//		engine.compiler.RegisterVisitHook("includeHook", func(c *visitors.Compiler, ctx antlr.ParserRuleContext) {
//			if v, ok := ctx.(*nasl.CallExpressionContext); ok {
//				if v.SingleExpression().GetText() != "include" {
//					return
//				}
//				if v.ArgumentList() == nil {
//					return
//				}
//				argumentsCtx, ok := v.ArgumentList().(*nasl.ArgumentListContext)
//				if !ok {
//					return
//				}
//				arguments := argumentsCtx.AllArgument()
//				if arguments == nil || len(arguments) == 0 {
//					return
//				}
//				libs[strings.Trim(arguments[0].GetText(), `"`)] = struct{}{}
//			}
//		})
//		for path, _ := range engine.scripts {
//			code, err := os.ReadFile(path)
//			if err != nil {
//				panic(err)
//			}
//			err = engine.Compile(string(code))
//			if err != nil {
//				panic(err)
//			}
//		}
//		engine.compiler.UnregisterVisitHook("includeHook")
//		//for lib, _ := range libs {
//		//	fmt.Println(lib)
//		//}
//		//检测依赖库用到的内置函数是否存在
//		missMethod := map[string]struct{}{}
//		userDefinedMethod := map[string]struct{}{}
//		engine.compiler.RegisterVisitHook("buildInMethodCheck", func(c *visitors.Compiler, ctx antlr.ParserRuleContext) {
//
//			if v, ok := ctx.(*nasl.FunctionDeclarationStatementContext); ok {
//				id := v.Identifier()
//				if v1, ok := id.(*nasl.IdentifierContext); ok {
//					userDefinedMethod[v1.GetText()] = struct{}{}
//				}
//			}
//			if v, ok := ctx.(*nasl.CallExpressionContext); ok {
//				id := v.SingleExpression().GetText()
//				hasMethod := false
//				if _, ok := NaslLib[id]; ok {
//					hasMethod = true
//				}
//				if _, ok := lib.NaslBuildInNativeMethod[id]; ok {
//					hasMethod = true
//				}
//				if _, ok := engine.GetVirtualMachine().GetVar(id); ok {
//					hasMethod = true
//				}
//				if !hasMethod {
//					missMethod[id] = struct{}{}
//				}
//			}
//		})
//		libsPath := []string{}
//		for lib, _ := range libs {
//			libsPath = append(libsPath, path.Join(engine.naslLibsPath, lib))
//		}
//		for _, path := range libsPath {
//			code, err := os.ReadFile(path)
//			if err != nil {
//				panic(err)
//			}
//			err = engine.Compile(string(code))
//			if err != nil {
//				panic(err)
//			}
//		}
//		engine.compiler.UnregisterVisitHook("buildInMethodCheck")
//		for s, _ := range missMethod {
//			if _, ok := userDefinedMethod[s]; !ok {
//				fmt.Println(s)
//			}
//		}
//		//BuildInMethodCheck(engine) // 检测当前已经加载的脚本内置函数是否存在
//	}
func TestPocScanner(t *testing.T) {
	consts.GetGormProjectDatabase()
	engine := NewScriptEngine()
	//engine.vm.GetConfig().SetStopRecover(true)
	engine.Debug() // 开启调试模式，脚本退出时会打印调试信息
	engine.LoadFamilys("Product detection")
	//engine.LoadScriptFromDb("gb_cisco_asa_detect.nasl")
	//engine.LoadScriptFromDb("gb_apache_hadoop_detect.nasl")
	engine.SetGoroutineNum(10)
	// 需要把ACT_SCAN的脚本都patch一遍
	engine.AddScriptPatch("ping_host.nasl", func(code string) string {
		codeBytes, err := os.ReadFile("/Users/z3/Downloads/ping_host_patch.nasl")
		if err != nil {
			return code
		}
		return string(codeBytes)
	})
	engine.AddScriptPatch("http_keepalive.inc", func(code string) string {
		codeLines := strings.Split(code, "\n")
		if len(codeLines) > 341 {
			codeLines[341] = "if( \" HTTP/1.1\" >< data && ! egrep( pattern:\"User-Agent:.+\", string:data, icase:TRUE ) ) {"
			code = strings.Join(codeLines, "\n")
		}
		return code
	})
	engine.AddScriptPatch("gb_altn_mdaemon_http_detect.nasl", func(code string) string {
		codeLines := strings.Split(code, "\n")
		if len(codeLines) > 55 {
			codeLines[55] = "if ((res =~ \"MDaemon[- ]Webmail\" || res =~ \"Server\\s*:\\s*WDaemon\") && \"WorldClient.dll\" >< res) {"
			code = strings.Join(codeLines, "\n")
		}
		return code
	})
	engine.AddScriptPatch("gb_apache_tomcat_open_redirect_vuln_lin.nasl", func(code string) string {
		codeBytes, err := os.ReadFile("/Users/z3/nasl/nasl-plugins/2018/apache/gb_apache_tomcat_open_redirect_vuln_lin.nasl")
		if err != nil {
			return code
		}
		return string(codeBytes)
	})
	engine.AddEngineHooks(func(engine *executor.Executor) {
		inFun := false
		engine.AddBreakPoint(func(v *yakvm.VirtualMachine) bool {
			defer func() {
				if err := recover(); err != nil {
					fmt.Println(err)
				}
			}()
			fm := v.CurrentFM()
			if fm == nil {
				return false
			}
			if fm.GetVerbose() == "function: recv_mysql_server_handshake" {
				inFun = true
			}
			if inFun {
				if fm.CurrentCode().StartLineNumber == 96 {
					v, ok := fm.CurrentScope().GetValueByName("buf")
					if ok {
						println(v.Value)
					}
				}
				inFun = false
			}
			return false
		})
		//	engine.RegisterBuildInMethodHook("build_detection_report", func(origin NaslBuildInMethod, engine *executor.Executor, params *executor.NaslBuildInMethodParam) (interface{}, error) {
		//		scriptObj := engine.Ctx.ScriptObj
		//		app := params.GetParamByName("app", "").String()
		//		version := params.GetParamByName("version", "").String()
		//		install := params.GetParamByName("install", "").String()
		//		cpe := params.GetParamByName("cpe", "").String()
		//		concluded := params.GetParamByName("concluded", "").String()
		//		if strings.TrimSpace(concluded) == "" || concluded == "Concluded from:" || concluded == "unknown" {
		//			return origin(engine.Ctx, params)
		//		}
		//		riskType := ""
		//		if v, ok := utils2.ActToChinese[scriptObj.Category]; ok {
		//			riskType = v
		//		} else {
		//			riskType = scriptObj.Category
		//		}
		//		source := "[NaslScript] " + scriptObj.ScriptName
		//		concludedUrl := params.GetParamByName("concludedUrl", "").String()
		//		solution := utils.MapGetString(scriptObj.Tags, "solution")
		//		summary := utils.MapGetString(scriptObj.Tags, "summary")
		//		cve := strings.Join(scriptObj.CVE, ", ")
		//		//xrefStr := ""
		//		//for k, v := range engine.scriptObj.Xrefs {
		//		//	xrefStr += fmt.Sprintf("\n Reference: %s(%s)", v, k)
		//		//}
		//		title := fmt.Sprintf("检测目标存在 [%s] 应用，版本号为 [%s]", app, version)
		//		if cve != "" {
		//			title += fmt.Sprintf(", CVE: %s", summary)
		//		}
		//		yakit.NewRisk(concludedUrl,
		//			yakit.WithRiskParam_Title(title),
		//			yakit.WithRiskParam_RiskType(riskType),
		//			yakit.WithRiskParam_Severity("low"),
		//			yakit.WithRiskParam_YakitPluginName(source),
		//			yakit.WithRiskParam_Description(summary),
		//			yakit.WithRiskParam_Solution(solution),
		//			yakit.WithRiskParam_Details(map[string]interface{}{
		//				"app":       app,
		//				"version":   version,
		//				"install":   install,
		//				"cpe":       cpe,
		//				"concluded": concluded,
		//				"source":    source,
		//				"cve":       cve,
		//			}),
		//		)
		//		return origin(engine.Ctx, params)
		//	})
	})
	//start := time.Now()
	//_, err := engine.ScanTarget("https://uat.sdeweb.hkcsl.com")
	//if err != nil {
	//	log.Error(err)
	//}
	//log.Info("scan time: ", time.Since(start))
	//data := engine.GetKBData()
	//data["Host/port_infos"] = nil
	//spew.Dump(data)
}
func TestLoadSetting(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	l, err := net.Listen("tcp", spew.Sprintf(":%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()
	engine := NewScriptEngine()
	engine.Debug()
	engine.SetCache(false)
	PatchEngine(engine)
	//engine.LoadCategory("ACT_SETTINGS")
	//engine.LoadScript("snmp_default_communities.nasl")
	engine.LoadScript("ids_evasion.nasl")
	//engine.LoadScript("compliance_tests.nasl")
	engine.ShowScriptTree()
	engine.SetPreferenceByScriptName("ids_evasion.nasl", "TCP evasion technique", "split")
	resultCh := engine.Scan("127.0.0.1", strconv.Itoa(port))
	for result := range resultCh {
		spew.Dump(result.Kbs.GetKB("NIDS/TCP/split"))
	}
}
