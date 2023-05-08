package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
	"sync"
)

type ScriptEngine struct {
	naslLibsPath       string
	scripts            map[string]struct{}
	excludeScripts     map[string]struct{}
	pluginGroupDefines map[PluginGroup][]string
	goroutineNum       int
	debug              bool
	engineHooks        []func(engine *Engine)
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		scripts:            make(map[string]struct{}),
		excludeScripts:     make(map[string]struct{}),
		pluginGroupDefines: make(map[PluginGroup][]string),
		goroutineNum:       10,
	}
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
func (engine *ScriptEngine) AddExcludeScript(path string) {
	engine.excludeScripts[path] = struct{}{}
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
func (engine *ScriptEngine) AddPluginIntoGroup(group PluginGroup, paths ...string) {
	engine.pluginGroupDefines[group] = append(engine.pluginGroupDefines[group], paths...)
}
func (e *ScriptEngine) LoadGroup(group PluginGroup) {
	if v, ok := e.pluginGroupDefines[group]; ok {
		for _, p := range v {
			e.LoadScript(p)
		}
	}
}
func (e *ScriptEngine) Scan(target string, ports string) error {
	var allErrors multiError
	log.Infof("start syn scan target: %s, ports: %s", target, ports)
	servicesInfo, err := ServiceScan(target, ports)
	if err != nil {
		return err
	}
	openPorts := []int{}
	kbs := NewNaslKBs()
	for _, result := range servicesInfo {
		if result.State == fp.OPEN {
			openPorts = append(openPorts, result.Port)
			kbs.AddKB("Host/scanned", result.Port)
			var ServiceName string
			switch result.Fingerprint.ServiceName {
			case "http", "https":
				ServiceName = "www"
			}
			if ServiceName != "" {
				kbs.SetKB(fmt.Sprintf("Services/%s", ServiceName), result.Port)
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
			engine.host = target
			engine.SetIncludePath(e.naslLibsPath)
			engine.SetKBs(kbs)
			engine.Init()
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
