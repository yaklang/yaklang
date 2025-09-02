package antlr4nasl

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/script_core"
)

func QueryAllScripts(script ...any) []*script_core.NaslScriptInfo {
	queryCondition := map[string]any{}
	if len(script) > 0 {
		for k, v := range utils.InterfaceToMapInterface(script[0]) {
			if utils.StringArrayContains([]string{"origin_file_name", "cve", "script_name", "category", "family"}, k) {
				queryCondition[k] = v
			} else {
				log.Warnf("not allow query field %s", k)
			}
		}
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}

	var scripts []*schema.NaslScript
	if db := db.Where(queryCondition).Find(&scripts); db.Error != nil {
		log.Errorf("cannot query script: %s", db.Error.Error())
		return nil
	}
	var ret []*script_core.NaslScriptInfo
	for _, s := range scripts {
		ret = append(ret, script_core.NewNaslScriptObjectFromNaslScript(s))
	}
	return ret
}
func RemoveDatabase() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("cannot fetch database: %s", db.Error)
	}
	if db := db.Model(&schema.NaslScript{}).Unscoped().Delete(&schema.NaslScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}
func UpdateDatabase(p string) {
	saveScript := func(path string) {
		if !strings.HasSuffix(path, ".nasl") {
			log.Errorf("Error load script %s: not a nasl file", path)
			return
		}
		engine := script_core.NewScriptEngine()
		engine.AddScriptLoadedHook(func(scriptIns *script_core.NaslScriptInfo) {
			err := scriptIns.Save()
			if err != nil {
				log.Errorf("Error save script %s: %s", path, err.Error())
			}
		})
		err := engine.LoadScript(path)
		if err != nil {
			log.Errorf("Error load script %s: %s", path, err.Error())
			return
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
}
func ScanTarget(target string, opts ...script_core.NaslScriptConfigOptFunc) (chan *script_core.NaslKBs, error) {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return nil, err
	}
	return script_core.NaslScan(host, fmt.Sprint(port), opts...), nil
}

var Exports = map[string]any{
	"UpdateDatabase":  UpdateDatabase,
	"RemoveDatabase":  RemoveDatabase,
	"QueryAllScripts": QueryAllScripts,
	"ScanTarget":      ScanTarget,
	"Scan":            script_core.NaslScan,
	"plugin":          script_core.WithPlugins,
	"family":          script_core.WithFamily,
	"riskHandle":      script_core.WithRiskHandle,
	"proxy":           script_core.WithProxy,
	"conditions":      script_core.WithConditions,
	"sourcePaths":     script_core.WithSourcePath,
}
