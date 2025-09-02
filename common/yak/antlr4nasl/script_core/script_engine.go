package script_core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
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
		MethodHook: make(map[string]func(origin NaslBuildInMethod, engine *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error)),
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

// loadScriptWithDependencies 加载脚本并处理其依赖关系和偏好设置
func (engine *ScriptEngine) loadScriptWithDependencies(script *NaslScriptInfo, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	if _, ok := loadedScript[script.OriginFileName]; ok {
		return nil
	}

	if !engine.scriptFilter(script) {
		return fmt.Errorf("script filtered")
	}

	scriptInstances := utils.NewSet[*NaslScriptInfo]()

	// 处理依赖加载
	if engine.config.autoLoadDependencies {
		dependencies := []string{}
		for _, dependency := range script.Dependencies {
			if dependency == "toolcheck.nasl" { // 不使用nasl内置的工具，所以跳过
				continue
			}
			if _, ok := engine.scripts[dependency]; ok {
				continue
			}
			dependencies = append(dependencies, path.Join(engine.dependenciesPath, dependency))
		}
		if len(dependencies) > 0 {
			scripts, err := engine.tryLoadScript(dependencies, cache, loadedScript)
			if err != nil {
				return fmt.Errorf("load `%s` dependencies failed: %s", script.OriginFileName, err)
			}
			scriptInstances.AddList(scripts)
			for _, info := range scripts {
				engine.dependencyScripts[info.OriginFileName] = struct{}{}
			}
		}
	}

	// 处理偏好设置
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

// loadScriptFromAbsolutePath 从绝对路径加载脚本
func (engine *ScriptEngine) loadScriptFromAbsolutePath(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	if utils.IsDir(fileName) {
		raw, err := utils.ReadFilesRecursively(fileName)
		if err != nil {
			return fmt.Errorf("Load script from dir `%s` failed: %v", fileName, err)
		}
		scripts, err := engine.tryLoadScript(raw, cache, loadedScript)
		if err != nil {
			return err
		}
		loadedScripts.AddList(scripts)
		return nil
	} else if utils.IsFile(fileName) {
		script, err := engine.loadScriptFromSource(false, fileName)
		if err != nil {
			return fmt.Errorf("Load script from file `%s` failed: %v", fileName, err)
		}
		cache[script.OriginFileName] = script
		return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
	} else {
		return fmt.Errorf("Load script `%s` failed: file not exists", fileName)
	}
}

// loadScriptFromRelativePath 从相对路径加载脚本，按优先级尝试：1.本地文件 2.sourcePath路径 3.数据库
func (engine *ScriptEngine) loadScriptFromRelativePath(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	// 1. 优先从当前目录的相对路径加载
	if utils.IsFile(fileName) {
		script, err := engine.loadScriptFromSource(false, fileName)
		if err != nil {
			return fmt.Errorf("Load script from file `%s` failed: %v", fileName, err)
		}
		cache[script.OriginFileName] = script
		return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
	}

	// 2. 尝试从 sourcePath 路径加载
	for _, sourcePath := range engine.config.sourcePath {
		sourcePathFile := path.Join(sourcePath, fileName)
		if utils.IsFile(sourcePathFile) {
			script, err := engine.loadScriptFromSource(false, sourcePathFile)
			if err != nil {
				return fmt.Errorf("Load script from source path `%s` failed: %v", sourcePathFile, err)
			}
			cache[script.OriginFileName] = script
			return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
		}
	}

	// 3. 最后从数据库加载
	script, err := engine.loadScriptFromSource(true, fileName)
	if err != nil {
		return fmt.Errorf("Load script `%s` from local file, source path and db all failed", fileName)
	}
	cache[script.OriginFileName] = script
	return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
}

// loadScriptsFromStringArray 从字符串数组批量加载脚本
func (engine *ScriptEngine) loadScriptsFromStringArray(scriptNames []string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	cachedScript := []*NaslScriptInfo{}
	unLoadedScript := []string{}

	// 分离已缓存和未加载的脚本
	for _, fileName := range scriptNames {
		if v, ok := cache[fileName]; ok {
			cachedScript = append(cachedScript, v)
		} else {
			unLoadedScript = append(unLoadedScript, fileName)
		}
	}

	scripts := cachedScript

	// 从数据库加载未缓存的脚本
	if len(unLoadedScript) > 0 {
		db := consts.GetGormProfileDatabase()
		scriptsModel := []*schema.NaslScript{}
		if err := db.Where("origin_file_name in (?)", unLoadedScript).Unscoped().Find(&scriptsModel).Error; err != nil {
			return err
		}

		// 检查是否所有脚本都找到了
		if len(scriptsModel) != len(unLoadedScript) {
			failedScript := []any{}
			for _, script := range unLoadedScript {
				found := false
				for _, scriptModel := range scriptsModel {
					if scriptModel.OriginFileName == script {
						found = true
						break
					}
				}
				if !found {
					failedScript = append(failedScript, script)
				}
			}
			if len(failedScript) > 0 {
				return fmt.Errorf("load scripts `%v` from db failed: not found", failedScript)
			}
		}

		// 加载脚本模型
		for _, scriptModel := range scriptsModel {
			script, err := engine.loadScriptFromSource(true, scriptModel.OriginFileName)
			if err != nil {
				return fmt.Errorf("load script `%s` from db failed: %v", scriptModel.OriginFileName, err)
			}
			scripts = append(scripts, script)
		}
	}

	// 缓存所有脚本
	for _, script := range scripts {
		cache[script.OriginFileName] = script
	}

	// 加载所有脚本及其依赖
	for _, script := range scripts {
		if err := engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts); err != nil {
			return fmt.Errorf("load script `%s` failed: %v", script.OriginFileName, err)
		}
	}

	return nil
}

// loadScriptFromInfo 从 NaslScriptInfo 对象加载脚本
func (engine *ScriptEngine) loadScriptFromInfo(scriptInfo *NaslScriptInfo, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	cache[scriptInfo.OriginFileName] = scriptInfo
	return engine.loadScriptWithDependencies(scriptInfo, cache, loadedScript, loadedScripts)
}

// loadScriptWithMode 根据配置的加载模式加载脚本
func (engine *ScriptEngine) loadScriptWithMode(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	switch engine.config.loadMode {
	case LoadModeFileOnly:
		return engine.loadScriptFileOnly(fileName, cache, loadedScript, loadedScripts)
	case LoadModeDBOnly:
		return engine.loadScriptDBOnly(fileName, cache, loadedScript, loadedScripts)
	case LoadModeFileFirst:
		return engine.loadScriptFileFirst(fileName, cache, loadedScript, loadedScripts)
	case LoadModeDBFirst:
		return engine.loadScriptDBFirst(fileName, cache, loadedScript, loadedScripts)
	case LoadModeAuto:
		fallthrough
	default:
		// 使用原有的加载逻辑
		if path.IsAbs(fileName) {
			return engine.loadScriptFromAbsolutePath(fileName, cache, loadedScript, loadedScripts)
		} else {
			return engine.loadScriptFromRelativePath(fileName, cache, loadedScript, loadedScripts)
		}
	}
}

// loadScriptFileOnly 仅从文件加载脚本
func (engine *ScriptEngine) loadScriptFileOnly(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	var scriptPath string
	if path.IsAbs(fileName) {
		scriptPath = fileName
	} else {
		// 对于相对路径，首先尝试当前目录
		if utils.IsFile(fileName) {
			scriptPath = fileName
		} else if len(engine.config.sourcePath) > 0 {
			// 尝试 sourcePath
			for _, sourcePath := range engine.config.sourcePath {
				sourcePathFile := path.Join(sourcePath, fileName)
				if utils.IsFile(sourcePathFile) {
					scriptPath = sourcePathFile
					break
				}
			}
		}
	}

	if scriptPath == "" || !utils.IsFile(scriptPath) {
		return fmt.Errorf("script file `%s` not found in file system", fileName)
	}

	script, err := engine.loadScriptFromSource(false, scriptPath)
	if err != nil {
		return fmt.Errorf("load script from file `%s` failed: %v", scriptPath, err)
	}
	cache[script.OriginFileName] = script
	return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
}

// loadScriptDBOnly 仅从数据库加载脚本
func (engine *ScriptEngine) loadScriptDBOnly(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	script, err := engine.loadScriptFromSource(true, fileName)
	if err != nil {
		return fmt.Errorf("load script `%s` from database failed: %v", fileName, err)
	}
	cache[script.OriginFileName] = script
	return engine.loadScriptWithDependencies(script, cache, loadedScript, loadedScripts)
}

// loadScriptFileFirst 优先文件，失败后数据库
func (engine *ScriptEngine) loadScriptFileFirst(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	// 尝试文件加载
	err := engine.loadScriptFileOnly(fileName, cache, loadedScript, loadedScripts)
	if err == nil {
		return nil
	}

	// 文件加载失败，尝试数据库
	return engine.loadScriptDBOnly(fileName, cache, loadedScript, loadedScripts)
}

// loadScriptDBFirst 优先数据库，失败后文件
func (engine *ScriptEngine) loadScriptDBFirst(fileName string, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}, loadedScripts *utils.Set[*NaslScriptInfo]) error {
	// 尝试数据库加载
	err := engine.loadScriptDBOnly(fileName, cache, loadedScript, loadedScripts)
	if err == nil {
		return nil
	}

	// 数据库加载失败，尝试文件
	return engine.loadScriptFileOnly(fileName, cache, loadedScript, loadedScripts)
}

// tryLoadScript 根据不同的输入类型和配置的加载模式分发到对应的加载函数
func (engine *ScriptEngine) tryLoadScript(script any, cache map[string]*NaslScriptInfo, loadedScript map[string]struct{}) ([]*NaslScriptInfo, error) {
	loadedScripts := utils.NewSet[*NaslScriptInfo]()

	switch ret := script.(type) {
	case string:
		fileName := ret
		// 使用配置的加载模式
		if err := engine.loadScriptWithMode(fileName, cache, loadedScript, loadedScripts); err != nil {
			return nil, err
		}
	case []string:
		// 字符串数组批量加载 - 对每个脚本应用加载模式
		for _, fileName := range ret {
			if err := engine.loadScriptWithMode(fileName, cache, loadedScript, loadedScripts); err != nil {
				return nil, fmt.Errorf("load script `%s` failed: %v", fileName, err)
			}
		}
	case *NaslScriptInfo:
		// 脚本对象加载
		if err := engine.loadScriptFromInfo(ret, cache, loadedScript, loadedScripts); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid script type")
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
		log.Errorf("load script %s failed: %v", script, err)
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
