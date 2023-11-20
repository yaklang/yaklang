package plugin_type_analyzer

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type YaklangInfo struct {
	Name string
	KV   []*YaklangInfoKV
}

func NewYakLangInfo(name string) *YaklangInfo {
	return &YaklangInfo{
		Name: name,
		KV:   make([]*YaklangInfoKV, 0),
	}
}

func (y *YaklangInfo) AddKV(value *YaklangInfoKV) {
	y.KV = append(y.KV, value)
}

type YaklangInfoKV struct {
	Key    string
	Value  any
	Extern []*YaklangInfoKV
}

func NewYaklangInfoKV(key string, value any) *YaklangInfoKV {
	return &YaklangInfoKV{
		Key:    key,
		Value:  value,
		Extern: make([]*YaklangInfoKV, 0),
	}
}

func (y *YaklangInfoKV) AddExtern(key string, value any) {
	y.Extern = append(y.Extern, NewYaklangInfoKV(key, value))
}

func (y *YaklangInfoKV) AddExternInfo(info *YaklangInfoKV) {
	y.Extern = append(y.Extern, info)
}

func (y *YaklangInfoKV) String() string {
	ret := fmt.Sprintf("%s: %v", y.Key, y.Value)
	for _, extern := range y.Extern {
		for _, str := range strings.Split(extern.String(), "\n") {
			if str != "" {
				ret += "\n\t" + str
			}
		}
	}
	return ret + "\n"
}

type PluginTypeAnalyzer interface {
	GetTypeSSAOpt() []ssaapi.Option
	GetTypeInfo(*ssaapi.Program) []*YaklangInfo
	CheckRule(*ssaapi.Program)
}

var pluginTypeAnalyzers = defaultAnalyzers()

func defaultAnalyzers() map[string]PluginTypeAnalyzer {
	return map[string]PluginTypeAnalyzer{
		"yak": &yakAnalyzer{},
	}
}

func GetPluginSSAOpt(pluginTyp string) []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	// all special plugin is yak.
	ret = append(ret, pluginTypeAnalyzers["yak"].GetTypeSSAOpt()...)
	if pluginTyp != "yak" {
		// if special plugin
		if rule, ok := pluginTypeAnalyzers[pluginTyp]; ok {
			ret = append(ret, rule.GetTypeSSAOpt()...)
		}
	}
	return ret
}

func GetPluginInfo(pluginType string, prog *ssaapi.Program) []*YaklangInfo {
	ret := make([]*YaklangInfo, 0)
	ret = append(ret, pluginTypeAnalyzers["yak"].GetTypeInfo(prog)...)
	if pluginType != "yak" {
		if rule, ok := pluginTypeAnalyzers[pluginType]; ok {
			ret = append(ret, rule.GetTypeInfo(prog)...)
		}
	}
	return ret
}

func CheckPluginType(pluginTyp string, prog *ssaapi.Program) {
	// all special plugin is yak.
	pluginTypeAnalyzers["yak"].CheckRule(prog)
	if pluginTyp != "yak" {
		// if special plugin
		if rule, ok := pluginTypeAnalyzers[pluginTyp]; ok {
			rule.CheckRule(prog)
		}
	}
}
