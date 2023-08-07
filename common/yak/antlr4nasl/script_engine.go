package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
	"sync"
	"time"
)

type ScriptEngine struct {
	proxies                        []string
	Kbs                            *NaslKBs
	naslLibsPath, dependenciesPath string
	scriptFilter                   func(script *NaslScriptInfo) bool
	scripts                        map[string]*NaslScriptInfo
	excludeScripts                 map[string]struct{} // 基于OID排除一些脚本
	goroutineNum                   int
	debug                          bool
	engineHooks                    []func(engine *Engine)
	loadedScripts                  map[string]struct{}
	loadedScriptsLock              *sync.Mutex
	scriptExecMutexs               map[string]*sync.Mutex
	scriptExecMutexsLock           *sync.Mutex
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		scripts:           make(map[string]*NaslScriptInfo),
		excludeScripts:    make(map[string]struct{}),
		goroutineNum:      10,
		Kbs:               NewNaslKBs(),
		loadedScripts:     make(map[string]struct{}),
		loadedScriptsLock: &sync.Mutex{},
		scriptFilter: func(script *NaslScriptInfo) bool {
			return true
		},
		scriptExecMutexsLock: &sync.Mutex{},
		scriptExecMutexs:     make(map[string]*sync.Mutex),
	}
}
func (engine *ScriptEngine) GetScriptMuxByName(name string) *sync.Mutex {
	engine.scriptExecMutexsLock.Lock()
	defer engine.scriptExecMutexsLock.Unlock()
	if v, ok := engine.scriptExecMutexs[name]; ok {
		return v
	}
	engine.scriptExecMutexs[name] = &sync.Mutex{}
	return engine.scriptExecMutexs[name]
}
func (engine *ScriptEngine) SetScriptFilter(filter func(script *NaslScriptInfo) bool) {
	engine.scriptFilter = filter
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
	if engine.scriptFilter(script) {
		engine.scripts[script.OriginFileName] = script
	}
}
func (engine *ScriptEngine) LoadScriptsFromDb(plugins ...string) {
	for _, plugin := range plugins {
		script, err := NewNaslScriptObjectFromDb(plugin)
		if err != nil {
			log.Error(err)
			return
		}
		engine.LoadScript(script)
	}
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

func (e *ScriptEngine) LoadFamilys(familys ...string) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	for _, family := range familys {
		var scripts []*yakit.NaslScript
		if db := db.Where("family = ?", family).Find(&scripts); db.Error != nil {
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
func (e *ScriptEngine) LoadWithConditions(conditions map[string]any) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	var scripts []*yakit.NaslScript
	if db := db.Where(conditions).Find(&scripts); db.Error != nil {
		log.Errorf("load scripts with conditions error: %v", db.Error)
	}
	for _, script := range scripts {
		if _, ok := e.excludeScripts[script.OID]; ok {
			continue
		}
		e.LoadScript(NewNaslScriptObjectFromNaslScript(script))
	}
}
func (e *ScriptEngine) ScanTarget(target string) error {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return err
	}
	return e.Scan(host, strconv.Itoa(port))
}
func (e *ScriptEngine) GetRootScripts() map[string]*NaslScriptInfo {
	//忽略了循环依赖
	rootScripts := make(map[string]*NaslScriptInfo)
	tmp := map[string]struct{}{}
	for _, info := range e.scripts {
		for _, dependency := range info.Dependencies {
			tmp[dependency] = struct{}{}
		}
	}
	for _, info := range e.scripts {
		if _, ok := tmp[info.OriginFileName]; !ok {
			rootScripts[info.OriginFileName] = info
		}
	}
	return rootScripts
}
func (e *ScriptEngine) Scan(host string, ports string) error {
	rootScripts := e.GetRootScripts()
	if len(rootScripts) == 0 {
		return utils.Errorf("no scripts to scan")
	}
	var allErrors multiError
	res := pingutil.PingAuto(host, "80,443,22", 3*time.Second, e.proxies...)
	if res.Ok {
		e.Kbs.SetKB("Host/dead", 0)
		e.Kbs.SetKB("Host/State", "up")
	} else {
		//ping检测不存活 或排除打印机设备时会标注为dead
		e.Kbs.SetKB("Host/dead", 1)
		e.Kbs.SetKB("Host/State", "down")
	}
	log.Infof("start syn scan host: %s, ports: %s", host, ports)
	servicesInfo, err := ServiceScan(host, ports, e.proxies...)
	if err != nil {
		return err
	}
	e.Kbs.SetKB("Host/scanned", 1)
	openPorts := []int{}
	portInfos := []*fp.MatchResult{}
	for _, result := range servicesInfo {
		if result.State == fp.OPEN {
			fingerprint := result.Fingerprint
			openPorts = append(openPorts, result.Port)
			portInfos = append(portInfos, result)
			e.Kbs.SetKB(fmt.Sprintf("Ports/tcp/%d", result.Port), 1)
			if fingerprint.ServiceName != "" {
				var serverName string
				if fingerprint.ServiceName == "http" {
					serverName = "www"
				} else {
					serverName = fingerprint.ServiceName
				}
				e.Kbs.SetKB(fmt.Sprintf("Services/%s", serverName), fingerprint.Port)
				e.Kbs.SetKB(fmt.Sprintf("Known/%s/%d", fingerprint.Proto, fingerprint.Port), fingerprint.ServiceName)
			}
			if fingerprint.Version != "" {
				e.Kbs.SetKB(fmt.Sprintf("Version/%s/%d", fingerprint.Proto, fingerprint.Port), fingerprint.Version)
			}
			for _, cpe := range fingerprint.CPEs {
				e.Kbs.SetKB(fmt.Sprintf("APP/%s/%d", fingerprint.Proto, fingerprint.Port), cpe)
			}
		}
	}
	// 缺少os finger_print、tcp_seq_index、ipidseq、Traceroute
	e.Kbs.SetKB("Host/port_infos", portInfos)
	swg := utils.NewSizedWaitGroup(e.goroutineNum)
	errorsMux := sync.Mutex{}
	for _, script := range rootScripts {
		if _, ok := e.excludeScripts[script.OID]; ok {
			continue
		}
		swg.Add()
		go func(script *NaslScriptInfo) {
			defer swg.Done()
			engine := New()
			engine.host = host
			engine.SetProxies(e.proxies...)
			engine.SetIncludePath(e.naslLibsPath)
			engine.SetDependenciesPath(e.dependenciesPath)
			engine.SetKBs(e.Kbs)
			engine.InitBuildInLib()
			engine.Debug(e.debug)
			engine.SetAutoLoadDependencies(true)
			engine.scriptExecMutexsLock = e.scriptExecMutexsLock
			engine.scriptExecMutexs = e.scriptExecMutexs
			//engine.RegisterBuildInMethodHook("log_message", func(origin NaslBuildInMethod, engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//	irisk := params.getParamByName("data").Value
			//	if v, ok := irisk.(*yakit.Risk); ok {
			//		_ = v
			//	}
			//	return origin(engine, params)
			//})
			engine.loadedScripts = e.loadedScripts
			engine.loadedScriptsLock = e.loadedScriptsLock
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
