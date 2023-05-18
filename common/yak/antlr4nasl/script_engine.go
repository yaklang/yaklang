package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
	"sync"
)

type ScriptEngine struct {
	Kbs                            *NaslKBs
	naslLibsPath, dependenciesPath string
	scripts                        map[*NaslScriptInfo]struct{}
	excludeScripts                 map[string]struct{} // 基于OID排除一些脚本
	scriptGroupDefines             map[ScriptGroup][]string
	goroutineNum                   int
	debug                          bool
	engineHooks                    []func(engine *Engine)
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		scripts:            make(map[*NaslScriptInfo]struct{}),
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
func (engine *ScriptEngine) LoadScript(script *NaslScriptInfo) {
	engine.scripts[script] = struct{}{}
}
func (engine *ScriptEngine) LoadScriptFromFile(path string) {
	if utils.IsDir(path) {
		raw, err := utils.ReadFilesRecursively(path)
		if err == nil {
			for _, r := range raw {
				engine.LoadScriptFromFile(r.Path)
			}
		}
	} else if utils.IsFile(path) {
		script, err := NewNaslScriptObjectFromFile(path)
		if err != nil {
			log.Error(err)
			return
		}
		engine.LoadScript(script)
	}
}

func (engine *ScriptEngine) AddScriptIntoGroup(group ScriptGroup, paths ...string) {
	engine.scriptGroupDefines[group] = append(engine.scriptGroupDefines[group], paths...)
}
func (e *ScriptEngine) LoadGroups(groups ...ScriptGroup) {
	db := consts.GetGormProfileDatabase()
	for _, group := range groups {
		if v, ok := e.scriptGroupDefines[group]; ok {
			for _, p := range v {
				e.LoadScriptFromFile(p)
			}
		}
		if db == nil {
			continue
		}
		var scripts []*yakit.NaslScript
		if db := db.Find(&scripts).Where("group = ?", group); db.Error != nil {
			continue
		}
		for _, script := range scripts {
			if _, ok := e.excludeScripts[script.OID]; ok {
				continue
			}
			e.LoadScript(NewNaslScriptObjectFromNaslScript(script))
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
	e.Kbs.SetKB("Host/scanned", 1)
	openPorts := []int{}
	portInfos := []*fp.MatchResult{}
	for _, result := range servicesInfo {
		if result.State == fp.OPEN {
			openPorts = append(openPorts, result.Port)
			portInfos = append(portInfos, result)
			e.Kbs.SetKB(fmt.Sprintf("Ports/tcp/%d", result.Port), 1)
			//var ServiceName string
			//switch result.Fingerprint.ServiceName {
			//case "http", "https":
			//	ServiceName = "www"
			//}
			//if ServiceName != "" {
			//	e.Kbs.SetKB(fmt.Sprintf("Services/%s", ServiceName), result.Port)
			//}
		}
	}
	e.Kbs.SetKB("Host/port_infos", portInfos)
	swg := utils.NewSizedWaitGroup(e.goroutineNum)
	errorsMux := sync.Mutex{}
	for script, _ := range e.scripts {
		if _, ok := e.excludeScripts[script.OID]; ok {
			continue
		}
		swg.Add()
		go func(script *NaslScriptInfo) {
			defer swg.Done()
			engine := New()
			engine.host = host
			engine.SetIncludePath(e.naslLibsPath)
			engine.SetDependenciesPath(e.dependenciesPath)
			engine.SetKBs(e.Kbs)
			engine.InitBuildInLib()
			engine.Debug(e.debug)
			for _, hook := range e.engineHooks {
				hook(engine)
			}
			err := engine.RunScript(script)
			if err != nil {
				log.Errorf("run script %s met error: %s", script.OriginFileName, err)
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
func (e *ScriptEngine) SetDependencies(p string) {
	e.dependenciesPath = p
}
