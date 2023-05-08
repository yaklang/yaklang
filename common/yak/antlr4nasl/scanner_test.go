package antlr4nasl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
	"strings"
	"testing"
)

func PatchEngine(engine *Engine) {
	engine.AddNaslLibPatch("http_func", func(s string) string {
		s += `

function http_get_port( default_list, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts ) {
 local_var final_port_list;

  final_port_list = http_get_ports(default_list:default_list,host:host,ignore_broken:ignore_broken,ignore_unscanned:ignore_unscanned,ignore_cgi_disabled:ignore_cgi_disabled,dont_use_vhosts:dont_use_vhosts);
  foreach port( final_port_list ) {
	return port;
  }
  return -1;
}
`
		return s
	})
	engine.AddNaslLibPatch("smtp_func", func(s string) string {
		s += `
function smtp_get_port( default_list, ignore_broken, ignore_unscanned ) {

  local_var final_port_list;

  final_port_list = smtp_get_ports(default_list:default_list,ignore_broken:ignore_broken,ignore_unscanned:ignore_unscanned);
	foreach port( final_port_list ) {
		return port;
	}
	return -1;
}
`
		return s
	})
}
func InitPluginGroup(engine *ScriptEngine) {
	apachePath := `/Users/z3/nasl/nasl-plugins/2013/apache
/Users/z3/nasl/nasl-plugins/2014/apache
/Users/z3/nasl/nasl-plugins/2022/apache
/Users/z3/nasl/nasl-plugins/2023/apache
/Users/z3/nasl/nasl-plugins/2012/apache
/Users/z3/nasl/nasl-plugins/2009/apache
/Users/z3/nasl/nasl-plugins/2017/apache
/Users/z3/nasl/nasl-plugins/2010/apache
/Users/z3/nasl/nasl-plugins/2019/apache
/Users/z3/nasl/nasl-plugins/2021/apache
/Users/z3/nasl/nasl-plugins/2020/apache
/Users/z3/nasl/nasl-plugins/2018/apache
/Users/z3/nasl/nasl-plugins/2011/apache
/Users/z3/nasl/nasl-plugins/2016/apache`
	oraclePath := `/Users/z3/nasl/nasl-plugins/2013/oracle
/Users/z3/nasl/nasl-plugins/2014/oracle
/Users/z3/nasl/nasl-plugins/2022/oracle
/Users/z3/nasl/nasl-plugins/2023/oracle
/Users/z3/nasl/nasl-plugins/2015/oracle
/Users/z3/nasl/nasl-plugins/2017/oracle
/Users/z3/nasl/nasl-plugins/2019/oracle
/Users/z3/nasl/nasl-plugins/2021/oracle
/Users/z3/nasl/nasl-plugins/2020/oracle
/Users/z3/nasl/nasl-plugins/2018/oracle
/Users/z3/nasl/nasl-plugins/2016/oracle`
	for _, path := range strings.Split(apachePath, "\n") {
		engine.AddPluginIntoGroup(PluginGroupApache, path)
	}
	for _, path := range strings.Split(oraclePath, "\n") {
		engine.AddPluginIntoGroup(PluginGroupOracle, path)
	}
}

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
//		engine := New()
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
	engine := NewScriptEngine()
	//engine.vm.GetConfig().SetStopRecover(true)
	engine.Debug()          // 开启调试模式，脚本退出时会打印调试信息
	InitPluginGroup(engine) // 初始化插件组
	engine.AddEngineHooks(func(engine *Engine) {
		PatchEngine(engine) // 一些库缺少函数
	})
	engine.SetIncludePath("/Users/z3/nasl/nasl-plugins") // 设置nasl依赖库位置
	//engine.LoadScript("/Users/z3/nasl/nasl-plugins/gb_apache_struts_detect.nasl")
	engine.LoadGroup(PluginGroupApache)
	engine.AddExcludeScript("gb_log4j_CVE-2021-44228_http_active.nasl") // 找不到http_cgi_dirs
	engine.LoadScript("/Users/z3/nasl/nasl-plugins/2022/apache/gb_log4j_CVE-2021-44228_pop3_active.nasl")
	//BuildInMethodCheck(engine) // 检测当前已经加载的脚本内置函数是否存在
	err := engine.Scan("34.241.215.249", "8080")
	var knownErrors multiError
	undefinedVars := []string{}
	if err != nil {
		if errors, ok := err.(multiError); ok {
			re, err := regexp.Compile("cannot found value by variable name:\\[(.*)\\]")
			if err != nil {
				panic(err)
			}
			re2, err := regexp.Compile("method `(.*)` is not implement")
			if err != nil {
				panic(err)
			}

			for _, err2 := range errors {
				res := re.FindStringSubmatch(err2.Error())
				if len(res) > 0 {
					undefinedVars = append(undefinedVars, res[1])
				} else {
					res = re2.FindStringSubmatch(err2.Error())
					if len(res) > 0 {
						undefinedVars = append(undefinedVars, res[1])
					} else {
						knownErrors = append(knownErrors, err2)
					}
				}
			}
		} else {
			knownErrors = append(knownErrors, err)
		}
	}
	if len(knownErrors) > 0 {
		log.Error(knownErrors)
	}
	spew.Dump(undefinedVars)
}
