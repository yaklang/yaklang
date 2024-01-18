package plugin_type

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

type PluginType string

const (
	// plugin type : "yak" "mitm" "port-scan" "codec"
	PluginTypeYak      PluginType = "yak"
	PluginTypeMitm     PluginType = "mitm"
	PluginTypePortScan PluginType = "port-scan"
	PluginTypeCodec    PluginType = "codec"
)

func ToPluginType(plugin string) PluginType {
	switch plugin {
	case "yak":
		return PluginTypeYak
	case "mitm":
		return PluginTypeMitm
	case "port-scan":
		return PluginTypePortScan
	case "codec":
		return PluginTypeCodec
	default:
		log.Errorf("unknown plugin type: %s", plugin)
		return PluginTypeYak
	}
}

type (
	CheckRuler      func(*ssaapi.Program) *result.StaticAnalyzeResults
	SSAOptCollector func() []ssaapi.Option
)

type PluginTypeAnalyzer struct {
	SSAOptCollectors map[PluginType]SSAOptCollector
	CheckRulers      map[PluginType][]CheckRuler
}

var pluginTypeAnalyzer = &PluginTypeAnalyzer{
	SSAOptCollectors: make(map[PluginType]SSAOptCollector),
	CheckRulers:      make(map[PluginType][]CheckRuler),
}

func RegisterSSAOptCollector(pluginTyp PluginType, f SSAOptCollector) {
	pluginTypeAnalyzer.SSAOptCollectors[pluginTyp] = f
}

func RegisterCheckRuler(pluginTyp PluginType, f CheckRuler) {
	if _, ok := pluginTypeAnalyzer.CheckRulers[pluginTyp]; !ok {
		pluginTypeAnalyzer.CheckRulers[pluginTyp] = make([]CheckRuler, 0)
	}
	pluginTypeAnalyzer.CheckRulers[pluginTyp] = append(pluginTypeAnalyzer.CheckRulers[pluginTyp], f)
}

func GetPluginSSAOpt(pluginType PluginType) []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	if funcs, ok := pluginTypeAnalyzer.SSAOptCollectors[pluginType]; ok {
		ret = append(ret, funcs()...)
	}
	return ret
}

func CheckPluginType(pluginType PluginType, prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	if funcs, ok := pluginTypeAnalyzer.CheckRulers[pluginType]; ok {
		for _, f := range funcs {
			func() {
				defer func() {
					err := recover()
					if err != nil {
						log.Errorf("CheckPluginType %s panic: %v", pluginType, err)
					}
				}()
				ret.Merge(f(prog))
			}()
		}
	}
	return ret
}
