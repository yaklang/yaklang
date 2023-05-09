package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
	"strconv"
	"sync"
)

type ScriptEngine struct {
	Kbs                *NaslKBs
	naslLibsPath       string
	scripts            map[string]struct{}
	excludeScripts     map[string]struct{}
	scriptGroupDefines map[ScriptGroup][]string
	goroutineNum       int
	debug              bool
	engineHooks        []func(engine *Engine)
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		scripts:            make(map[string]struct{}),
		excludeScripts:     make(map[string]struct{}),
		scriptGroupDefines: make(map[ScriptGroup][]string),
		goroutineNum:       10,
		Kbs:                NewNaslKBs(),
	}
}
func (engine *ScriptEngine) GetAllScriptGroups() map[ScriptGroup][]string {
	return engine.scriptGroupDefines
}
func (engine *ScriptEngine) GetKBData() map[string]interface{} {
	return engine.Kbs.GetData()
}
func (engine *ScriptEngine) SetNaslLibsPath(path string) {
	engine.naslLibsPath = path
}
func (engine *ScriptEngine) SetGoroutineNum(num int) {
	engine.goroutineNum = num
}
func (engine *ScriptEngine) AddEngineHooks(hooks func(engine *Engine)) {
	engine.engineHooks = append(engine.engineHooks, hooks)
}
func (engine *ScriptEngine) Debug(debug ...bool) {
	if len(debug) > 0 {
		engine.debug = debug[0]
	} else {
		engine.debug = true
	}
}
func (engine *ScriptEngine) AddExcludeScripts(paths ...string) {
	for _, p := range paths {
		engine.excludeScripts[p] = struct{}{}
	}
}
func (engine *ScriptEngine) LoadScript(path string) {
	if utils.IsDir(path) {
		raw, err := utils.ReadFilesRecursively(path)
		if err == nil {
			for _, r := range raw {
				engine.LoadScript(r.Path)
			}
		}
	} else if utils.IsFile(path) {
		engine.scripts[path] = struct{}{}
	}
}
func (engine *ScriptEngine) RunWithDescription() {

}
func (engine *ScriptEngine) AddScriptIntoGroup(group ScriptGroup, paths ...string) {
	engine.scriptGroupDefines[group] = append(engine.scriptGroupDefines[group], paths...)
}
func (e *ScriptEngine) LoadGroups(groups ...ScriptGroup) {
	for _, group := range groups {
		if v, ok := e.scriptGroupDefines[group]; ok {
			for _, p := range v {
				e.LoadScript(p)
			}
		}
	}
}
func (e *ScriptEngine) ScanTarget(target string) error {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return err
	}
	return e.Scan(host, strconv.Itoa(port))
}
func (e *ScriptEngine) Scan(host string, ports string) error {
	var allErrors multiError
	log.Infof("start syn scan host: %s, ports: %s", host, ports)
	servicesInfo, err := ServiceScan(host, ports)
	if err != nil {
		return err
	}
	openPorts := []int{}
	for _, result := range servicesInfo {
		if result.State == fp.OPEN {
			openPorts = append(openPorts, result.Port)
			e.Kbs.AddKB("Host/scanned", result.Port)
			var ServiceName string
			switch result.Fingerprint.ServiceName {
			case "http", "https":
				ServiceName = "www"
			}
			if ServiceName != "" {
				e.Kbs.SetKB(fmt.Sprintf("Services/%s", ServiceName), result.Port)
			}
		}
	}
	swg := utils.NewSizedWaitGroup(e.goroutineNum)
	errorsMux := sync.Mutex{}
	for script, _ := range e.scripts {
		scriptName := filepath.Base(script)
		if _, ok := e.excludeScripts[scriptName]; ok {
			continue
		}
		swg.Add()
		go func(script string) {
			defer swg.Done()
			engine := New()
			engine.host = host
			engine.SetIncludePath(e.naslLibsPath)
			engine.SetKBs(e.Kbs)
			engine.InitBuildInLib()
			for _, hook := range e.engineHooks {
				hook(engine)
			}
			err := engine.RunFile(script)
			if err != nil {
				log.Errorf("run script %s met error: %s", script, err)
				errorsMux.Lock()
				allErrors = append(allErrors, err)
				errorsMux.Unlock()
			}
		}(script)
	}
	swg.Wait()
	if len(allErrors) != 0 {
		return allErrors
	}
	return nil
}
func (e *ScriptEngine) SetIncludePath(p string) {
	e.naslLibsPath = p
}
