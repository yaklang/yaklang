package antlr4nasl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"path"
	"time"
)

type ScriptEngine struct {
	proxies                        []string
	Kbs                            *NaslKBs
	naslLibsPath, dependenciesPath string
	scriptFilter                   func(script *NaslScriptInfo) bool
	scripts                        map[string]*NaslScriptInfo // 将会被执行的脚本
	scriptCache                    map[string]*NaslScriptInfo // 用于缓存查询过的脚本
	dependencyScripts              map[string]struct{}        // 标记被依赖的脚本，减少查找根脚本的负担
	excludeScripts                 map[string]struct{}        // 排除一些脚本
	goroutineNum                   int
	debug                          bool
	engineHooks                    []func(engine *Engine)
	//loadedScriptsLock              *sync.Mutex
	//scriptExecMutexs               map[string]*sync.Mutex
	//scriptExecMutexsLock           *sync.Mutex
	config *NaslScriptConfig
}

func NewScriptEngineWithConfig(cfg *NaslScriptConfig) *ScriptEngine {
	engine := &ScriptEngine{
		scripts:        make(map[string]*NaslScriptInfo),
		scriptCache:    make(map[string]*NaslScriptInfo),
		excludeScripts: make(map[string]struct{}),
		goroutineNum:   10,
		Kbs:            NewNaslKBs(),
		//loadedScripts:  make(map[string]struct{}),
		//loadedScriptsLock: &sync.Mutex{},
		scriptFilter: func(script *NaslScriptInfo) bool {
			return true
		},
		//scriptExecMutexsLock: &sync.Mutex{},
		//scriptExecMutexs:     make(map[string]*sync.Mutex),
		config:            NewNaslScriptConfig(),
		dependencyScripts: make(map[string]struct{}),
	}
	engine.config = cfg
	engine.LoadScript(cfg.plugin)
	engine.LoadFamilys(cfg.family)
	if cfg.conditions != nil {
		engine.LoadWithConditions(cfg.conditions)
	}
	return engine
}
func NewScriptEngine() *ScriptEngine {
	return NewScriptEngineWithConfig(NewNaslScriptConfig())
}

//	func (engine *ScriptEngine) GetScriptMuxByName(name string) *sync.Mutex {
//		engine.scriptExecMutexsLock.Lock()
//		defer engine.scriptExecMutexsLock.Unlock()
//		if v, ok := engine.scriptExecMutexs[name]; ok {
//			return v
//		}
//		engine.scriptExecMutexs[name] = &sync.Mutex{}
//		return engine.scriptExecMutexs[name]
//	}
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
func (engine *ScriptEngine) AddExcludeScripts(names ...string) {
	for _, name := range names {
		engine.excludeScripts[name] = struct{}{}
	}
}
func (engine *ScriptEngine) tryLoadScript(script any, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}) ([]*NaslScriptInfo, error) {
	loadedScripts := utils.NewSet[*NaslScriptInfo]()
	var loadWithDepError error
	loadWithDep := func(script *NaslScriptInfo) error {
		if _, ok := loadedScript[script.OriginFileName]; ok {
			return nil
		}
		if engine.scriptFilter(script) {
			scriptInstances := utils.NewSet[*NaslScriptInfo]()
			if engine.config.autoLoadDependencies {
				dependencies := []string{}
				for _, dependency := range script.Dependencies {
					if dependency == "toolcheck.nasl" { // 不使用nasl内置的工具，所以跳过
						continue
					}
					//if dependency == "snmp_default_communities.nasl" { // 太慢了，先跳过
					//	continue
					//}
					if _, ok := engine.scripts[dependency]; ok {
						continue
					}
					dependencies = append(dependencies, path.Join(engine.dependenciesPath, dependency))
				}
				if len(dependencies) > 0 {
					scripts, err := engine.tryLoadScript(dependencies, cache, loadedScript)
					if err != nil {
						err = fmt.Errorf("load `%s` dependencies failed: %s", script.OriginFileName, err)
						loadWithDepError = err
						return err
					}
					scriptInstances.AddList(scripts)
					for _, info := range scripts {
						engine.dependencyScripts[info.OriginFileName] = struct{}{}
					}
				}
			}
			if script.Preferences != nil && engine.config.preference != nil {
				for k, v := range engine.config.preference {
					if _, ok := script.Preferences[k]; ok {
						val := map[string]interface{}{}
						val["name"] = k
						switch ret := v.(type) {
						case bool:
							if ret {
								val["value"] = "yes"
								val["type"] = "checkbox"
							} else {
								val["value"] = "no"
								val["type"] = "checkbox"
							}
						default:
							val["value"] = v
							val["type"] = "entry"
						}
						script.Preferences[k] = val
					}
				}
			}

			scriptInstances.Add(script)
			loadedScripts.AddList(scriptInstances.List())
			loadedScript[script.OriginFileName] = struct{}{}
			return nil
		}
		err := fmt.Errorf("script filtered")
		loadWithDepError = err
		return err
	}
	switch ret := script.(type) {
	case string:
		fileName := ret
		if path.IsAbs(fileName) { // 绝对路径则尝试从文件加载
			if utils.IsDir(fileName) {
				raw, err := utils.ReadFilesRecursively(fileName)
				if err != nil {
					return nil, fmt.Errorf("Load script from dir `%s` failed: %v", fileName, err)
				} else {
					return engine.tryLoadScript(raw, cache, loadedScript)
				}
			} else if utils.IsFile(fileName) {
				script, err := NewNaslScriptObjectFromFile(fileName)
				if err != nil {
					return nil, fmt.Errorf("Load script from file `%s` failed: %v", fileName, err)
				} else {
					cache[script.OriginFileName] = script
					loadWithDep(script)
				}
			} else {
				return nil, fmt.Errorf("Load script `%s` failed: file not exists", fileName)
			}
		} else { // 优先从本地文件加载
			loadOk := false
			if utils.IsFile(fileName) {
				script, err := NewNaslScriptObjectFromFile(fileName)
				if err != nil {
					return nil, fmt.Errorf("Load script from file `%s` failed: %v", fileName, err)
				} else {
					loadOk = true
					cache[script.OriginFileName] = script
					loadWithDep(script)
				}
			}
			if !loadOk {
				script, err := NewNaslScriptObjectFromDb(fileName)
				if err != nil {
					return nil, fmt.Errorf("Load script `%s` from db and file failed", fileName)
				} else {
					cache[script.OriginFileName] = script
					loadWithDep(script)
				}
			}
		}
	case []string:
		cachedScript := []*NaslScriptInfo{}
		unLoadedScript := []string{}
		for _, fileName := range ret {
			if v, ok := cache[fileName]; ok {
				cachedScript = append(cachedScript, v)
			} else {
				unLoadedScript = append(unLoadedScript, fileName)
			}
		}
		db := consts.GetGormProfileDatabase()
		scriptsModel := []*yakit.NaslScript{}
		scripts := cachedScript
		if len(unLoadedScript) > 0 {
			if err := db.Where("origin_file_name in (?)", unLoadedScript).Unscoped().Find(&scriptsModel).Error; err != nil {
				return nil, err
			}
			if len(scriptsModel) != len(unLoadedScript) {
				failedSript := []any{}
				for _, script := range unLoadedScript {
					found := false
					for _, scriptModel := range scriptsModel {
						if scriptModel.OriginFileName == script {
							found = true
							break
						}
					}
					if !found {
						failedSript = append(failedSript, script)
					}
				}
				if len(failedSript) > 0 {
					return nil, fmt.Errorf("load scripts `%v` from db failed: not found", failedSript)
				}
			}
			for _, scriptModel := range scriptsModel {
				script, err := NewNaslScriptObjectFromDb(scriptModel.OriginFileName)
				if err != nil {
					return nil, fmt.Errorf("load script `%s` from db failed: %v", scriptModel.OriginFileName, err)
				}
				scripts = append(scripts, script)
			}
		}

		for _, script := range scripts {
			cache[script.OriginFileName] = script
		}
		for _, script := range scripts {
			if err := loadWithDep(script); err != nil {
				return nil, fmt.Errorf("load script `%s` failed: %v", script.OriginFileName, err)
			}
		}
	case *NaslScriptInfo:
		cache[ret.OriginFileName] = ret
		loadWithDep(ret)
	default:
		return nil, fmt.Errorf("invalid script type")
	}
	if loadWithDepError != nil {
		return nil, loadWithDepError
	}
	return loadedScripts.List(), nil
}
func (engine *ScriptEngine) LoadScript(script any) bool {
	scriptIns, err := engine.tryLoadScript(script, engine.scriptCache, map[string]struct{}{})
	if err != nil {
		log.Error(err)
		return false
	}
	for _, script := range scriptIns {
		engine.scripts[script.OriginFileName] = script
	}
	return true
}
func (e *ScriptEngine) LoadFamilys(family string) {
	if family == "" {
		return
	}
	e.LoadWithConditions(map[string]any{
		"family": family,
	})
}
func (e *ScriptEngine) LoadWithConditions(conditions map[string]any) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	allowedConditions := make(map[string]any)
	for k, v := range conditions {
		switch k {
		case "family", "category", "origin_file_name":
			allowedConditions[k] = v
		}
	}
	if len(allowedConditions) == 0 {
		return
	}
	if family, ok := conditions["family"].(string); ok && family != "" {
		if family != "Web Servers" {
			return
		}
		if family == "Web Servers" {
			db = db.
				Where("script_name like '%apache%' OR script_name like '%nginx%' OR script_name like '%jetty%' OR script_name like '%websphere%' OR script_name like '%Lighttpd%' OR script_name like '%tomcat%' OR script_name like '% (HTTP)' OR script_name like '%weblogic%'").Unscoped()
		}
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
	return e.Scan(host, fmt.Sprint(port))
}
func (e *ScriptEngine) GetRootScripts() map[string]*NaslScriptInfo {
	//忽略了循环依赖
	rootScripts := make(map[string]*NaslScriptInfo)
	tmp := map[string]struct{}{}
	for k, info := range e.scripts {
		if _, ok := e.dependencyScripts[k]; ok {
			continue
		}
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
			e.Kbs.SetKB(fmt.Sprintf("Ports/%s/%d", result.GetProto(), result.Port), 1)
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
	//swg := utils.NewSizedWaitGroup(e.goroutineNum)
	//errorsMux := sync.Mutex{}
	// 创建执行引擎
	newEngineByConfig := func() *Engine {
		engine := NewWithKbs(e.Kbs)
		engine.preferences = e.config.preference
		engine.host = host
		engine.SetProxies(e.proxies...)
		engine.SetIncludePath(e.naslLibsPath)
		engine.SetDependenciesPath(e.dependenciesPath)
		engine.InitBuildInLib()
		engine.Debug(e.debug)
		for _, hook := range e.engineHooks {
			hook(engine)
		}
		return engine
	}

	executedScripts := map[string]struct{}{}
	var allErrors utils.MergeErrors
	var runScriptWithDep func(script *NaslScriptInfo) error
	runScriptWithDep = func(script *NaslScriptInfo) error {
		if _, ok := executedScripts[script.OriginFileName]; ok {
			return nil
		}
		executedScripts[script.OriginFileName] = struct{}{}
		if _, ok := e.excludeScripts[script.OID]; ok {
			return fmt.Errorf("script %s is excluded", script.OriginFileName)
		}
		if len(script.Dependencies) > 0 {
			for _, dependency := range script.Dependencies {
				if dependency == "toolcheck.nasl" {
					continue
				}
				dependencyScript, ok := e.scripts[dependency]
				if !ok {
					return fmt.Errorf("script %s dependency %s not found", script.OriginFileName, dependency)
				}
				err := runScriptWithDep(dependencyScript)
				if err != nil {
					return err
				}
			}
		}
		return newEngineByConfig().RunScript(script)
	}
	for _, script := range rootScripts {
		err := runScriptWithDep(script)
		if err != nil {
			log.Errorf("run script %s met error: %s", script.OriginFileName, err)
			allErrors = append(allErrors, err)
		}
	}
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
