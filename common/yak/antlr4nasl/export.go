package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bindata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	utils2 "github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
)

type NaslScriptConfig struct {
	plugin     []string
	family     []string
	proxies    []string
	riskHandle func(risk interface{})
}

func NewNaslScriptConfig() *NaslScriptConfig {
	return &NaslScriptConfig{}
}

type NaslScriptConfigOptFunc func(c *NaslScriptConfig)

var Exports = map[string]interface{}{
	"UpdateDatabase": func(p string) {
		saveScript := func(path string) {
			if !strings.HasSuffix(path, ".nasl") {
				log.Errorf("Error load script %s: not a nasl file", path)
				return
			}
			engine := New()
			engine.SetDescription(true)
			engine.InitBuildInLib()
			err := engine.SafeRunFile(path)
			if err != nil {
				log.Errorf("Error load script %s: %s", path, err.Error())
				return
			}
			scriptIns := engine.GetScriptObject()
			err = scriptIns.Save()
			if err != nil {
				log.Errorf("Error save script %s: %s", path, err.Error())
			}
		}
		if utils.IsDir(p) {
			swg := utils.NewSizedWaitGroup(20)
			raw, err := utils.ReadFilesRecursively(p)
			if err == nil {
				for _, r := range raw {
					if !strings.HasSuffix(r.Path, ".nasl") && !strings.HasSuffix(r.Path, ".inc") {
						continue
					}
					swg.Add()
					go func(path string) {
						defer swg.Done()
						saveScript(path)
					}(r.Path)
				}
			}
			swg.Wait()
		} else if utils.IsFile(p) {
			saveScript(p)
		}
	},
	"NewScriptGroup": func(name string, scriptNames ...string) error {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Errorf("cannot fetch database: %s", db.Error)
		}
		for _, scriptName := range scriptNames {
			scriptIns, err := yakit.QueryNaslScriptByName(db, scriptName)
			if err != nil {
				log.Errorf("cannot find script %s: %s", scriptName, err.Error())
				continue
			}
			if scriptIns == nil {
				return utils.Errorf("cannot find script %s", scriptName)
			}
			scriptIns.Group = name
			if db := db.Save(scriptIns); db.Error != nil {
				return db.Error
			}
		}
		return nil
	},
	"RemoveDatabase": func() error {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Errorf("cannot fetch database: %s", db.Error)
		}
		if db := db.Model(&yakit.NaslScript{}).Unscoped().Delete(&yakit.NaslScript{}); db.Error != nil {
			return db.Error
		}
		return nil
	},
	"QueryAllScript": func() []*NaslScriptInfo {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return nil
		}
		var scripts []*yakit.NaslScript
		if db := db.Find(&scripts); db.Error != nil {
			return nil
		}
		var ret []*NaslScriptInfo
		for _, s := range scripts {
			ret = append(ret, NewNaslScriptObjectFromNaslScript(s))
		}
		return ret
	},
	"ScanTarget": func(target string, opts ...NaslScriptConfigOptFunc) (map[string]interface{}, error) {
		config := NewNaslScriptConfig()
		for _, opt := range opts {
			opt(config)
		}
		engine := NewScriptEngine()
		engine.LoadScriptsFromDb(config.plugin...)
		engine.LoadFamilys(config.family...)

		engine.proxies = config.proxies
		riskHandle := config.riskHandle
		engine.AddEngineHooks(func(engine *Engine) {
			engine.RegisterBuildInMethodHook("build_detection_report", func(origin NaslBuildInMethod, engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
				scriptObj := engine.scriptObj
				app := params.getParamByName("app", "").String()
				version := params.getParamByName("version", "").String()
				install := params.getParamByName("install", "").String()
				cpe := params.getParamByName("cpe", "").String()
				concluded := params.getParamByName("concluded", "").String()
				if strings.TrimSpace(concluded) == "" || concluded == "Concluded from:" || concluded == "unknown" {
					return origin(engine, params)
				}
				riskType := ""
				if v, ok := utils2.ActToChinese[scriptObj.Category]; ok {
					riskType = v
				} else {
					riskType = scriptObj.Category
				}
				source := "[NaslScript] " + engine.scriptObj.ScriptName
				concludedUrl := params.getParamByName("concludedUrl", "").String()
				solution := utils.MapGetString(engine.scriptObj.Tags, "solution")
				summary := utils.MapGetString(engine.scriptObj.Tags, "summary")
				cve := strings.Join(scriptObj.CVE, ", ")
				//xrefStr := ""
				//for k, v := range engine.scriptObj.Xrefs {
				//	xrefStr += fmt.Sprintf("\n Reference: %s(%s)", v, k)
				//}
				title := fmt.Sprintf("检测目标存在 [%s] 应用，版本号为 [%s]", app, version)
				if cve != "" {
					title += fmt.Sprintf(", CVE: %s", summary)
				}
				risk, _ := yakit.NewRisk(concludedUrl,
					yakit.WithRiskParam_Title(title),
					yakit.WithRiskParam_RiskType(riskType),
					yakit.WithRiskParam_Severity("low"),
					yakit.WithRiskParam_YakitPluginName(source),
					yakit.WithRiskParam_Description(summary),
					yakit.WithRiskParam_Solution(solution),
					yakit.WithRiskParam_Details(map[string]interface{}{
						"app":       app,
						"version":   version,
						"install":   install,
						"cpe":       cpe,
						"concluded": concluded,
						"source":    source,
						"cve":       cve,
					}),
				)
				if riskHandle != nil {
					riskHandle(risk)
				}
				return origin(engine, params)
			})
			engine.SetAutoLoadDependencies(true)
			// 需要把ACT_SCAN的脚本都patch一遍
			engine.AddNaslLibPatch("ping_host.nasl", func(code string) string {
				codeBytes, err := bindata.Asset("data/nasl-patches/" + "ping_host_patch.nasl")
				if err != nil {
					log.Errorf("read ping_host_patch.nasl error: %v", err)
					return code
				}
				return string(codeBytes)
			})
			engine.AddNaslLibPatch("http_keepalive.inc", func(code string) string {
				codeLines := strings.Split(code, "\n")
				if len(codeLines) > 341 {
					codeLines[341] = "if( \" HTTP/1.1\" >< data && ! egrep( pattern:\"User-Agent:.+\", string:data, icase:TRUE ) ) {"
					code = strings.Join(codeLines, "\n")
				}
				return code
			})
			engine.AddNaslLibPatch("gb_altn_mdaemon_http_detect.nasl", func(code string) string {
				codeLines := strings.Split(code, "\n")
				if len(codeLines) > 55 {
					codeLines[55] = "if ((res =~ \"MDaemon[- ]Webmail\" || res =~ \"Server\\s*:\\s*WDaemon\") && \"WorldClient.dll\" >< res) {"
					code = strings.Join(codeLines, "\n")
				}
				return code
			})
		})

		err := engine.ScanTarget(target)
		//if err != nil {
		//	return nil, err
		//}
		return engine.GetKBData(), err
	},
	"plugin": func(plugin string) NaslScriptConfigOptFunc {
		return func(c *NaslScriptConfig) {
			c.plugin = append(c.plugin, plugin)
		}
	},
	"family": func(family string) NaslScriptConfigOptFunc {
		return func(c *NaslScriptConfig) {
			c.family = append(c.family, family)
		}
	},
	"riskHandle": func(f func(interface{})) NaslScriptConfigOptFunc {
		return func(c *NaslScriptConfig) {
			c.riskHandle = f
		}
	},
	"proxy": func(proxy ...string) NaslScriptConfigOptFunc {
		return func(c *NaslScriptConfig) {
			c.proxies = proxy
		}
	},
}
