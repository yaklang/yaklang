package script_core

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ScriptLoadMode 脚本加载方式
type ScriptLoadMode string

const (
	// LoadModeAuto 自动加载模式（默认），使用 tryLoadScript 的完整逻辑
	LoadModeAuto ScriptLoadMode = "auto"
	// LoadModeFileOnly 仅从文件加载
	LoadModeFileOnly ScriptLoadMode = "file_only"
	// LoadModeDBOnly 仅从数据库加载
	LoadModeDBOnly ScriptLoadMode = "db_only"
	// LoadModeFileFirst 优先文件，失败后数据库
	LoadModeFileFirst ScriptLoadMode = "file_first"
	// LoadModeDBFirst 优先数据库，失败后文件
	LoadModeDBFirst ScriptLoadMode = "db_first"
)

type NaslScriptConfig struct {
	plugins                 []string
	family                  string
	proxies                 []string
	riskHandle              func(risk any)
	conditions              map[string]any
	preference              map[string]any
	autoLoadDependencies    bool
	ignoreRequirementsError bool
	sourcePath              []string
	loadMode                ScriptLoadMode
}

func NewNaslScriptConfig() *NaslScriptConfig {
	return &NaslScriptConfig{
		ignoreRequirementsError: true,
		autoLoadDependencies:    true,
		preference:              make(map[string]any),
		conditions:              make(map[string]any),
		loadMode:                LoadModeAuto, // 默认使用自动模式
	}
}

type NaslScriptConfigOptFunc func(c *NaslScriptConfig)

func WithPreference(p interface{}) NaslScriptConfigOptFunc {
	preference := utils.InterfaceToMapInterface(p)
	return func(c *NaslScriptConfig) {
		c.preference = preference
	}
}
func WithIgnoreRequirementsError(b bool) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.ignoreRequirementsError = b
	}
}
func WithConditions(script ...any) NaslScriptConfigOptFunc {
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
	return func(c *NaslScriptConfig) {
		c.conditions = queryCondition
	}
}

func WithProxy(proxies ...string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.proxies = proxies
	}
}
func WithRiskHandle(f func(any)) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.riskHandle = f
	}
}
func WithFamily(family string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.family = family
	}
}

func WithPlugins(plugins ...string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.plugins = plugins
	}
}

func WithSourcePath(sourcePath ...string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.sourcePath = append(c.sourcePath, sourcePath...)
	}
}

func WithLoadMode(mode ScriptLoadMode) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.loadMode = mode
	}
}
