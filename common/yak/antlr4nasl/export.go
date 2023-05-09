package antlr4nasl

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
)

type NaslScriptConfig struct {
	group []string
}

func NewNaslScriptConfig() *NaslScriptConfig {
	return &NaslScriptConfig{}
}

type NaslScriptConfigOptFunc func(c *NaslScriptConfig)

var Exports = map[string]interface{}{
	"UpdateDatabase": func(p string) {
		saveScript := func(path string) {
			engine := New()
			engine.SetDescription(true)
			engine.InitBuildInLib()
			err := engine.SafeRunFile(path)
			if err != nil {
				log.Errorf("Error load script %s: %s", path, err.Error())
				return
			}
			err = engine.GetScriptObject().Save()
			if err != nil {
				log.Errorf("Error save script %s: %s", path, err.Error())
			}
		}
		if utils.IsDir(p) {
			swg := utils.NewSizedWaitGroup(10)
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
				return err
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
	"KBToRisk": func() {

	},
	"ScanTarget": func(target string, opts ...NaslScriptConfigOptFunc) (map[string]interface{}, error) {
		config := NewNaslScriptConfig()
		for _, opt := range opts {
			opt(config)
		}
		engine := NewScriptEngine()
		InitPluginGroup(engine)
		for _, g := range config.group {
			engine.LoadGroups(ScriptGroup(g))
		}
		err := engine.ScanTarget(target)
		if err != nil {
			return nil, err
		}
		return engine.GetKBData(), nil
	},
	"group": func(groupName string) NaslScriptConfigOptFunc {
		return func(c *NaslScriptConfig) {
			c.group = append(c.group, groupName)
		}
	},
	"proxy": lowhttp.WithProxy,
}
