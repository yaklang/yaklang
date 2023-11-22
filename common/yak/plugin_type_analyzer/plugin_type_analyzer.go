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

type (
	CheckRuler        func(*ssaapi.Program)
	TypeInfoCollector func(*ssaapi.Program) *YaklangInfo
	SSAOptCollector   func() []ssaapi.Option
)

type PluginTypeAnalyzer struct {
	SSAOptCollectors   map[string]SSAOptCollector
	TypeInfoCollectors map[string][]TypeInfoCollector
	CheckRulers        map[string][]CheckRuler
}

var pluginTypeAnalyzer = &PluginTypeAnalyzer{
	SSAOptCollectors:   make(map[string]SSAOptCollector),
	TypeInfoCollectors: make(map[string][]TypeInfoCollector),
	CheckRulers:        make(map[string][]CheckRuler),
}

func RegisterSSAOptCollector(pluginTyp string, f SSAOptCollector) {
	pluginTypeAnalyzer.SSAOptCollectors[pluginTyp] = f
}

func RegisterTypeInfoCollector(pluginTyp string, f TypeInfoCollector) {
	if _, ok := pluginTypeAnalyzer.TypeInfoCollectors[pluginTyp]; !ok {
		pluginTypeAnalyzer.TypeInfoCollectors[pluginTyp] = make([]TypeInfoCollector, 0)
	}
	pluginTypeAnalyzer.TypeInfoCollectors[pluginTyp] = append(pluginTypeAnalyzer.TypeInfoCollectors[pluginTyp], f)
}

func RegisterCheckRuler(pluginTyp string, f CheckRuler) {
	if _, ok := pluginTypeAnalyzer.CheckRulers[pluginTyp]; !ok {
		pluginTypeAnalyzer.CheckRulers[pluginTyp] = make([]CheckRuler, 0)
	}
	pluginTypeAnalyzer.CheckRulers[pluginTyp] = append(pluginTypeAnalyzer.CheckRulers[pluginTyp], f)
}

func getPluginSSAOpt(pluginType string) []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	if funcs, ok := pluginTypeAnalyzer.SSAOptCollectors[pluginType]; ok {
		ret = append(ret, funcs()...)
	}
	return ret
}

func GetPluginSSAOpt(pluginType string) []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	ret = append(ret, getPluginSSAOpt("yak")...)
	if pluginType != "yak" {
		ret = append(ret, getPluginSSAOpt(pluginType)...)
	}
	return ret
}

func getPluginInfo(pluginType string, prog *ssaapi.Program) []*YaklangInfo {
	ret := make([]*YaklangInfo, 0)
	if funcs, ok := pluginTypeAnalyzer.TypeInfoCollectors[pluginType]; ok {
		for _, f := range funcs {
			func() {
				defer recover()
				ret = append(ret, f(prog))
			}()
		}
	}
	return ret
}

func GetPluginInfo(pluginType string, prog *ssaapi.Program) []*YaklangInfo {
	ret := make([]*YaklangInfo, 0)
	ret = append(ret, getPluginInfo("yak", prog)...)
	if pluginType != "yak" {
		ret = append(ret, getPluginInfo(pluginType, prog)...)
	}
	return ret
}

func checkPluginType(pluginType string, prog *ssaapi.Program) {
	if funcs, ok := pluginTypeAnalyzer.CheckRulers[pluginType]; ok {
		for _, f := range funcs {
			func() {
				defer recover()
				f(prog)
			}()
		}
	}
}

func CheckPluginType(pluginType string, prog *ssaapi.Program) {
	checkPluginType("yak", prog)
	if pluginType != "yak" {
		checkPluginType(pluginType, prog)
	}
}
