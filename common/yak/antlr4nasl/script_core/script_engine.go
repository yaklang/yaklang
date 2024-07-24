package script_core

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
	"os"
	"path"
	"strings"
)

type ExecContext struct {
	ctx        *context.Context
	Host       string
	Ports      string
	Kbs        *NaslKBs
	Proxies    []string
	MethodHook map[string]func(origin NaslBuildInMethod, engine *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error)
	ScriptObj  *NaslScriptInfo
	Debug      bool
}

func NewExecContext() *ExecContext {
	return &ExecContext{
		Kbs:       NewNaslKBs(),
		ScriptObj: NewNaslScriptObject(),
	}
}

type ScriptEngine struct {
	*log.Logger
	scriptPatch                    map[string]func(code string) string
	dbCache                        bool
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
	engineHooks                    []func(engine *executor.Executor)
	onScriptLoaded                 []func(info *NaslScriptInfo)
	MethodHook                     map[string]func(origin NaslBuildInMethod, engine *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error)
	//loadedScriptsLock              *sync.Mutex
	//scriptExecMutexs               map[string]*sync.Mutex
	//scriptExecMutexsLock           *sync.Mutex
	config *NaslScriptConfig
}

func NewScriptEngineWithConfig(cfg *NaslScriptConfig) *ScriptEngine {
	engine := &ScriptEngine{
		Logger:         log.GetLogger("NASL Logger"),
		dbCache:        true,
		scripts:        make(map[string]*NaslScriptInfo),
		scriptCache:    make(map[string]*NaslScriptInfo),
		excludeScripts: make(map[string]struct{}),
		goroutineNum:   10,
		scriptPatch:    map[string]func(code string) string{},
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
	if engine.debug {
		engine.Logger.SetLevel("debug")
	}
	engine.config = cfg
	engine.LoadScript(cfg.plugins)
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
func (engine *ScriptEngine) SetNaslLibsPath(path string) {
	engine.naslLibsPath = path
}
func (engine *ScriptEngine) SetGoroutineNum(num int) {
	engine.goroutineNum = num
}
func (engine *ScriptEngine) AddEngineHooks(hooks func(engine *executor.Executor)) {
	engine.engineHooks = append(engine.engineHooks, hooks)
}
func (engine *ScriptEngine) AddScriptLoadedHook(hook func(info *NaslScriptInfo)) {
	engine.onScriptLoaded = append(engine.onScriptLoaded, hook)
}
func (engine *ScriptEngine) SetCache(b bool) {
	engine.dbCache = b
}
func (engine *ScriptEngine) Debug(debug ...bool) {
	if len(debug) > 0 {
		engine.debug = debug[0]
	} else {
		engine.debug = true
	}
	if engine.debug {
		engine.Logger.SetLevel("debug")
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

				script, err := engine.loadScriptFromSource(false, fileName)
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
				script, err := engine.loadScriptFromSource(false, fileName)
				if err != nil {
					return nil, fmt.Errorf("Load script from file `%s` failed: %v", fileName, err)
				} else {
					loadOk = true
					cache[script.OriginFileName] = script
					loadWithDep(script)
				}
			}
			if !loadOk {
				script, err := engine.loadScriptFromSource(true, fileName)
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
		scriptsModel := []*schema.NaslScript{}
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
				script, err := engine.loadScriptFromSource(true, scriptModel.OriginFileName)
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
func (e *ScriptEngine) loadScriptFromSource(fromDb bool, name string) (*NaslScriptInfo, error) {
	hookCode := func(name, code string) (string, error, bool) {
		fileName := path.Base(name)
		if v, ok := e.scriptPatch[fileName]; ok {
			return v(code), nil, true
		}
		return "", nil, false
	}

	if fromDb {
		originName := name
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return nil, utils.Errorf("gorm database is nil")
		}
		var scripts []*schema.NaslScript
		if err := db.Where("origin_file_name = ?", originName).First(&scripts).Error; err != nil {
			log.Error(err)
			return nil, err
		}
		if len(scripts) == 0 {
			return nil, utils.Errorf("script %s not found", originName)
		}
		if len(scripts) > 1 {
			return nil, utils.Errorf("script %s found more than one", originName)
		}
		scirptIns := NewNaslScriptObjectFromNaslScript(scripts[0])
		newCode, err, ok := hookCode(name, scirptIns.Script)
		if err != nil {
			return nil, err
		}
		code := scirptIns.Script
		if ok {
			code = newCode
		}
		if !e.dbCache || ok {
			script, err := e.DescriptionExec(code, name)
			if err != nil {
				return nil, err
			}
			return script, nil
		} else {
			return scirptIns, nil
		}
	} else {
		content, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		code := string(content)
		newCode, err, ok := hookCode(name, code)
		if err != nil {
			return nil, err
		}
		if ok {
			code = newCode
		}
		script, err := e.DescriptionExec(code, name)
		if err != nil {
			return nil, err
		}
		return script, nil
	}
}

func (engine *ScriptEngine) AddMethodHook(name string, f func(origin NaslBuildInMethod, engine *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error)) {
	engine.MethodHook[name] = f
}
func (engine *ScriptEngine) DescriptionExec(code, name string) (*NaslScriptInfo, error) {
	ctx := NewExecContext()
	ctx.Host = ""
	ctx.Proxies = engine.proxies
	ctx.Kbs = NewNaslKBs()
	ctx.ScriptObj.OriginFileName = name
	ctx.ScriptObj.Script = code
	e := engine.NewExecEngine(ctx)
	e.SetVars(map[string]any{
		"description": true,
	})
	err := e.Exec(code, name)
	if err != nil {
		return nil, err
	}
	return ctx.ScriptObj, nil
}
func (engine *ScriptEngine) LoadScript(script any) error {
	scriptIns, err := engine.tryLoadScript(script, engine.scriptCache, map[string]struct{}{})
	if err != nil {
		return err
	}
	for _, script := range scriptIns {
		engine.Debugf("loaded script: %s", script.OriginFileName)
		for _, f := range engine.onScriptLoaded {
			f(script)
		}
		engine.scripts[script.OriginFileName] = script
	}
	return nil
}
func (e *ScriptEngine) SetPreferenceByScriptName(script string, k string, v any) {
	if scriptInfo, ok := e.scripts[script]; ok {
		if scriptInfo.Preferences == nil {
			scriptInfo.Preferences = make(map[string]interface{})
		}
		iv := scriptInfo.Preferences[k]
		if pre, ok := iv.(map[string]any); ok {
			pre["value"] = v
		}
	}
}
func (e *ScriptEngine) SetPreference(oid string, k string, v any) {
	for _, scriptInfo := range e.scripts {
		if scriptInfo.OID == oid {
			if scriptInfo.Preferences == nil {
				scriptInfo.Preferences = make(map[string]interface{})
			}
			iv := scriptInfo.Preferences[k]
			if pre, ok := iv.(map[string]any); ok {
				pre["value"] = v
			}
			break
		}
	}
}
func (e *ScriptEngine) GetAllPreference() map[string][]*Preference {
	res := map[string][]*Preference{}
	for _, scriptInfo := range e.scripts {
		preference := LoadPreferenceFromMap(scriptInfo.Preferences)
		res[scriptInfo.OriginFileName] = append(res[scriptInfo.OriginFileName], preference...)
	}
	return res
}
func (e *ScriptEngine) ShowScriptTree() {
	scripts := e.GetRootScripts()
	var dumpTree func(script *NaslScriptInfo, deep int)
	dumpTree = func(script *NaslScriptInfo, deep int) {
		fmt.Printf("%s- %s\n", strings.Repeat("  ", deep), script.OriginFileName)
		for _, dependency := range script.Dependencies {
			if dep, ok := e.scripts[dependency]; ok {
				dumpTree(dep, deep+1)
			}
		}
	}
	for _, root := range scripts {
		dumpTree(root, 0)
	}
}
func (e *ScriptEngine) LoadCategory(category string) {
	if category == "" {
		return
	}
	e.LoadWithConditions(map[string]any{
		"category": category,
	})
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

	var scripts []*schema.NaslScript
	if db := db.Where(conditions).Find(&scripts); db.Error != nil {
		log.Errorf("load scripts with conditions error: %v", db.Error)
	}
	for _, script := range scripts {
		if _, ok := e.excludeScripts[script.OID]; ok {
			continue
		}
		e.LoadScript(script.OriginFileName)
	}
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
func (e *ScriptEngine) Scan(hosts string, ports string) chan *ExecContext {
	hostList := utils.ParseStringToHosts(hosts)
	res := make(chan *ExecContext)
	swg := utils.NewSizedWaitGroup(e.goroutineNum)
	go func() {
		for _, host := range hostList {
			swg.Add()
			go func() {
				defer swg.Done()
				ctx, err := e.ScanSingle(host, ports)
				if err != nil {
					log.Errorf("scan host %s met error: %s", host, err)
					return
				}
				res <- ctx
			}()
		}
		swg.Wait()
		close(res)
	}()
	return res
}
func (e *ScriptEngine) ScanSingle(host string, ports string) (*ExecContext, error) {
	ctx := NewExecContext()
	ctx.Host = host
	ctx.Ports = ports
	ctx.Proxies = e.proxies
	ctx.Kbs = NewNaslKBs()
	rootScripts := e.GetRootScripts()
	if len(rootScripts) == 0 {
		return nil, utils.Errorf("no scripts to scan")
	}

	log.Infof("start ping scan host: %s, ports: %s", host, ports)
	Ping(ctx)
	log.Infof("start syn scan host: %s, ports: %s", host, ports)
	ServiceScan(ctx)

	//swg := utils.NewSizedWaitGroup(e.goroutineNum)
	//errorsMux := sync.Mutex{}
	// 创建执行引擎

	executedScripts := map[string]struct{}{}
	var allErrors error = nil

	var runScriptWithDep func(script *NaslScriptInfo, ctx *ExecContext) error
	runScriptWithDep = func(script *NaslScriptInfo, ctx *ExecContext) error {
		// check cached
		if _, ok := executedScripts[script.OriginFileName]; ok {
			return nil
		}
		executedScripts[script.OriginFileName] = struct{}{}
		// check exclude script
		if _, ok := e.excludeScripts[script.OID]; ok {
			return fmt.Errorf("script %s is excluded", script.OriginFileName)
		}
		// load depended script
		if len(script.Dependencies) > 0 {
			for _, dependency := range script.Dependencies {
				if dependency == "toolcheck.nasl" {
					continue
				}
				dependencyScript, ok := e.scripts[dependency]
				if !ok {
					return fmt.Errorf("script %s dependency %s not found", script.OriginFileName, dependency)
				}
				err := runScriptWithDep(dependencyScript, ctx)
				if err != nil {
					return err
				}
			}
		}
		// check condition
		for _, key := range script.MandatoryKeys {
			var re string
			if strings.Contains(key, "=") {
				splits := strings.Split(key, "=")
				if len(splits) == 2 {
					re = splits[1]
					key = splits[0]
				}
			}
			if ctx.Kbs.GetKB(key) == nil {
				return utils.Errorf("%w: because the key %s is missing", requirements_error, key)
			}
			if re != "" {
				v := ctx.Kbs.GetKB(key)
				if !utils.MatchAllOfRegexp(utils.InterfaceToString(v), re) {
					return utils.Errorf("%w: because the key %s is not match the regexp %s", requirements_error, key, re)
				}
			}
		}
		for _, key := range script.RequireKeys {
			if ctx.Kbs.GetKB(key) == nil {
				return utils.Errorf("%w: because the key %s is missing", requirements_error, key)
			}
		}
		udpPortOk := false
		for _, port := range script.RequireUdpPorts {
			if ctx.Kbs.GetKB(fmt.Sprintf("Ports/udp/%s", port)) == 1 {
				udpPortOk = true
				break
			}
		}
		if len(script.RequireUdpPorts) > 0 && !udpPortOk {
			return utils.Errorf("%w: none of the required udp ports are open", requirements_error)
		}
		tcpPortOk := false
		for _, port := range script.RequirePorts {
			if ctx.Kbs.GetKB(fmt.Sprintf("Ports/tcp/%s", port)) != 1 {
				tcpPortOk = true
				break
			}
		}
		if len(script.RequirePorts) > 0 && !tcpPortOk {
			return utils.Errorf("%w: none of the required udp ports are open", requirements_error)
		}
		for _, key := range script.ExcludeKeys {
			if ctx.Kbs.GetKB(key) != nil {
				return utils.Errorf("%w: because the key %s is present", requirements_error, key)
			}
		}
		ctx.ScriptObj = script
		return e.NewExecEngine(ctx).Exec(script.Script, script.OriginFileName)
	}
	for _, script := range rootScripts {
		err := runScriptWithDep(script, ctx)
		if err != nil {
			if errors.Is(err, requirements_error) && e.config.ignoreRequirementsError {
				continue
			}
			log.Errorf("run script %s met error: %s", script.OriginFileName, err)
			allErrors = utils.JoinErrors(allErrors, err)
		}
	}
	if !errors.Is(allErrors, nil) {
		return nil, allErrors
	}
	return ctx, nil
}
func (e *ScriptEngine) NewExecEngine(ctx *ExecContext) *executor.Executor {
	engine := executor.NewWithContext()
	engine.SetIncludePath(e.naslLibsPath)
	engine.Debug(e.debug)
	engine.SetLib(GetExtLib(ctx))
	engine.Compiler.SetNaslLib(GetNaslLibKeys())
	for _, hook := range e.engineHooks {
		hook(engine)
	}
	return engine
}

func (e *ScriptEngine) SetIncludePath(p string) {
	e.naslLibsPath = p
}
func (e *ScriptEngine) SetDependencies(p string) {
	e.dependenciesPath = p
}
func (e *ScriptEngine) AddScriptPatch(lib string, handle func(string2 string) string) {
	e.scriptPatch[lib] = handle
}
