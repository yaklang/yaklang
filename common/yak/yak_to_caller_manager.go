package yak

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/fuzztag"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/synscanx"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/yak/httptpl"

	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/simulator"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/utils/lowhttp/http_struct"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/utils/yakgit"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
)

func init() {
	lock := new(sync.Mutex)
	mutate.InitCodecCaller(func(name string, s interface{}) (string, error) {
		lock.Lock()
		defer lock.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic from fuzz.codec.caller: %s", err)
			}
		}()

		db := consts.GetGormProfileDatabase()
		if db == nil {
			return "", utils.Errorf("no database connection for codec caller")
		}
		script, err := yakit.GetYakScriptByName(db, name)
		if err != nil {
			return "", utils.Errorf("query plugin[%v] failed: %s", name, err)
		}
		if script.Type != "codec" {
			return "", utils.Errorf("plugin %v is not codec plugin", script.ScriptName)
		}

		engineRoot := NewScriptEngine(1)
		engineRoot.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
			engine.SetVars(map[string]any{
				"scriptName": script.ScriptName,
				"param":      utils.InterfaceToString(s),
			})
			return nil
		})
		engineRoot.HookOsExit()
		engine, err := engineRoot.ExecuteWithoutCache(`
plugin,err = db.GetYakitPluginByName(scriptName)
var result
if err {
    die("query plugin failed: %v"%err)
}
if plugin.Type != "codec"{
    die("only support codec plugin")
}

eval(plugin.Content)
if handle{
    result = handle(param)
}else{
    die("not found handle function in script %s"%scriptName)
}
`, map[string]interface{}{})
		if err != nil {
			return "", utils.Errorf("load engine and execute codec script error: %s", err)
		}

		result, ok := engine.GetVar("result")
		if !ok {
			return "", utils.Error("fuzz.codec no result")
		}
		return utils.InterfaceToString(result), nil
	})
}

var HooksExports = map[string]interface{}{
	"NewManager":                   NewYakToCallerManager,
	"NewMixPluginCaller":           NewMixPluginCaller,
	"NewMixPluginCallerWithFilter": NewMixPluginCallerWithFilter,
	"RemoveYakitPluginByName":      removeScriptByNameCtx,
	"LoadYakitPluginContext":       loadScriptCtx,
	"LoadYakitPlugin":              loadScript,
	"LoadYakitPluginByName":        loadScriptByName,
	"LoadYakitPluginByID":          loadScriptByID,
	"LoadYakitPluginByIDContext":   loadScriptByIDCtx,
	"CallYakitPluginFunc":          CallYakitPluginFunc,
}

var (
	currentCoreEngineMutext  = new(sync.Mutex)
	currentCoreEngine        *antlr4yak.Engine
	haveSetCurrentCoreEngine bool
)

func setCurrentCoreEngine(e *antlr4yak.Engine) {
	currentCoreEngineMutext.Lock()
	defer currentCoreEngineMutext.Unlock()

	if currentCoreEngine == nil {
		currentCoreEngine = e
	} else {
		haveSetCurrentCoreEngine = true
	}
}

func unsetCurrentCoreEngine(e *antlr4yak.Engine) {
	currentCoreEngineMutext.Lock()
	defer currentCoreEngineMutext.Unlock()

	if currentCoreEngine == e {
		currentCoreEngine = nil
		haveSetCurrentCoreEngine = false
	}
}

func FeedbackFactory(db *gorm.DB, caller func(result *ypb.ExecResult) error, saveToDb bool, yakScriptName string) func(i interface{}, items ...interface{}) {
	return func(i interface{}, items ...interface{}) {
		if caller == nil {
			return
		}

		//defer func() {
		//	if err := recover(); err != nil {
		//		log.Errorf("yakit_output/save failed: %s", err)
		//	}
		//}()

		var str string
		if len(items) > 0 {
			str = fmt.Sprintf(utils.InterfaceToString(i), items...)
		} else {
			str = utils.InterfaceToString(i)
		}

		t, msg := yaklib.MarshalYakitOutput(str)
		if t == "" {
			return
		}
		ylog := &yaklib.YakitLog{
			Level:     t,
			Data:      msg,
			Timestamp: time.Now().Unix(),
		}
		raw, err := yaklib.YakitMessageGenerator(ylog)
		if err != nil {
			return
		}

		result := &ypb.ExecResult{
			IsMessage: true,
			Message:   raw,
		}
		if saveToDb {
			// mitmSaveToDBLock.Lock()
			// yakit.SaveExecResult(db, yakScriptName, result)
			// mitmSaveToDBLock.Unlock()
		}

		err = caller(result)
		if err != nil {
			if strings.Contains(err.Error(), "feedback handler not set") {
				return
			}
			log.Warn(err)
			return
		}
		return
	}
}

type YakitCallerIf func(result *ypb.ExecResult) error

func (y YakitCallerIf) Send(i *ypb.ExecResult) error {
	return y(i)
}

type callArgumentHookFunc func(name string, numIn int, args []any) []any

type fetchFuncFromSrcCodeConfig struct {
	script            *schema.YakScript
	code              string
	engineHook        func(e *antlr4yak.Engine) error
	callArgumentHooks map[string]callArgumentHookFunc
	functionNames     []string
}

type fetchFuncFromSrcCodeOptions func(*fetchFuncFromSrcCodeConfig)

type CallerHookDescription struct {
	// 这两个是
	YakScriptId   string
	YakScriptName string
	VerboseName   string
}

type CallerHooks struct {
	HookName string

	Hooks []*CallerHookDescription
}

type CallConfig struct {
	runtimeCtx context.Context
	callback   func()
	pluginId   string
	items      []interface{}
	itemFuncs  []func() interface{}
	forceSync  bool
}

func NewCallConfig() *CallConfig {
	return &CallConfig{
		runtimeCtx: context.Background(),
	}
}

type CallOpt func(*CallConfig)

func WithCallConfigRuntimeCtx(ctx context.Context) CallOpt {
	return func(c *CallConfig) {
		c.runtimeCtx = ctx
	}
}

func WithCallConfigForceSync(forceSync bool) CallOpt {
	return func(c *CallConfig) {
		c.forceSync = forceSync
	}
}

func WithCallConfigPluginId(pluginId string) CallOpt {
	return func(c *CallConfig) {
		c.pluginId = pluginId
	}
}

func WithCallConfigCallback(callback func()) CallOpt {
	return func(c *CallConfig) {
		c.callback = callback
	}
}

func WithCallConfigItems(items ...interface{}) CallOpt {
	return func(c *CallConfig) {
		c.items = items
	}
}

func WithCallConfigItemFuncs(itemFuncs ...func() interface{}) CallOpt {
	return func(c *CallConfig) {
		c.itemFuncs = itemFuncs
	}
}

type YakFunctionCaller struct {
	Handler func(callback func(*yakvm.Frame), args ...any) any
}

type Caller struct {
	Core    *YakFunctionCaller
	Hash    string
	Id      string
	Verbose string
	Engine  *antlr4yak.Engine
	// NativeFunction *exec.Function
}

// YakToCallerManager 是 Yaklang 中用于管理插件函数调用的核心结构体。
type YakToCallerManager struct {
	// 存储插件函数调用表
	table *sync.Map
	// 设置单个插件的并发限制
	swg                *utils.SizedWaitGroup
	baseWaitGroup      *sync.WaitGroup
	dividedContext     bool
	loadTimeout        time.Duration
	callTimeout        time.Duration
	runtimeId          string
	proxy              string
	vulFilter          filter.Filterable
	ContextCancelFuncs *sync.Map
	Err                error

	// 插件执行跟踪器
	executionTracker *PluginExecutionTracker
	enableTracing    bool

	// 插件长时间运行阈值（秒），用于 trace 推送判断
	longRunningThreshold int
}

func NewYakToCallerManager() *YakToCallerManager {
	caller := &YakToCallerManager{
		table:                new(sync.Map),
		baseWaitGroup:        new(sync.WaitGroup),
		loadTimeout:          10 * time.Second,
		callTimeout:          time.Duration(consts.GetGlobalCallerCallPluginTimeout() * float64(time.Second)),
		ContextCancelFuncs:   new(sync.Map),
		executionTracker:     NewPluginExecutionTracker(),
		enableTracing:        false,
		longRunningThreshold: consts.PluginCallDurationThresholdSeconds, // 默认使用全局常量
	}
	return caller
}

func (y *YakToCallerManager) WithVulFilter(filter filter.Filterable) *YakToCallerManager {
	y.vulFilter = filter
	return y
}

func (y *YakToCallerManager) SetLoadPluginTimeout(i float64) {
	y.loadTimeout = time.Duration(i * float64(time.Second))
}

func (y *YakToCallerManager) SetCallPluginTimeout(i float64) {
	y.callTimeout = time.Duration(i * float64(time.Second))
}

func (y *YakToCallerManager) SetDividedContext(b bool) {
	y.dividedContext = b
}

// SetLongRunningThreshold 设置插件长时间运行阈值（秒）
func (y *YakToCallerManager) SetLongRunningThreshold(seconds int) {
	y.longRunningThreshold = seconds
}

// GetLongRunningThreshold 获取插件长时间运行阈值（秒）
func (y *YakToCallerManager) GetLongRunningThreshold() int {
	return y.longRunningThreshold
}

func (y *YakToCallerManager) SetConcurrent(i int) error {
	if y.swg != nil {
		err := utils.Error("cannot set swg for YakToCallerManager: existed swg")
		log.Error(err)
		return err
	}
	swg := utils.NewSizedWaitGroup(i)
	y.swg = swg
	return nil
}

func (y *YakToCallerManager) GetWaitingEventCount() int {
	if y.swg != nil {
		return int(y.swg.WaitingEventCount.Load())
	}
	return 0
}

func (y *YakToCallerManager) GetCurrentHooksGRPCModel() []*ypb.YakScriptHooks {
	var items []*ypb.YakScriptHooks
	for _, i := range y.GetCurrentHooks() {
		ins := &ypb.YakScriptHooks{
			HookName: i.HookName,
		}
		for _, hook := range i.Hooks {
			ins.Hooks = append(ins.Hooks, &ypb.YakScriptHookItem{
				YakScriptName: hook.YakScriptName,
				Verbose:       hook.VerboseName,
			})
		}
		items = append(items, ins)
	}
	return items
}

func (y *YakToCallerManager) GetCurrentHooks() []*CallerHooks {
	var allHooks []*CallerHooks

	y.table.Range(func(key, value interface{}) bool {
		hookName := key.(string)
		hooks := value.([]*Caller)

		hooksInstance := &CallerHooks{
			HookName: hookName,
		}
		for _, h := range hooks {
			verbose := h.Verbose
			if verbose == "" {
				verbose = "default"
			}
			hooksInstance.Hooks = append(hooksInstance.Hooks, &CallerHookDescription{
				YakScriptName: h.Verbose,
				VerboseName:   verbose,
			})
		}

		if hooksInstance.Hooks != nil {
			allHooks = append(allHooks, hooksInstance)
		}
		return true
	})
	return allHooks
}

func (y *YakToCallerManager) SetForYakit(
	ctx context.Context,
	code string,
	paramMap map[string]any, callerIf interface {
		Send(result *ypb.ExecResult) error
	},
	hooks ...string,
) error {
	caller := func(result *ypb.ExecResult) error {
		return callerIf.Send(result)
	}
	db := consts.GetGormProjectDatabase()
	return y.Set(ctx, code, paramMap, func(engine *antlr4yak.Engine) error {
		engine.OverrideRuntimeGlobalVariables(map[string]any{
			"yakit_output": FeedbackFactory(db, caller, false, "default"),
			"yakit_save":   FeedbackFactory(db, caller, true, "default"),
			"yakit_status": func(id string, i interface{}) {
				FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
					Id:   id,
					Data: fmt.Sprint(i),
				})
			},
			"yakit": yaklib.GetExtYakitLibByClient(yaklib.NewVirtualYakitClient(caller)),
		})
		return nil
	}, hooks...)
}

func (y *YakToCallerManager) getYakitPluginContext(ctx ...context.Context) *YakitPluginContext {
	var finalCtx context.Context
	if len(ctx) > 0 {
		finalCtx = ctx[0]
	}

	canFunc, ok := finalCtx.Value("cancel").(context.CancelFunc)
	if !ok {
		finalCtx, canFunc = context.WithCancel(finalCtx)
		finalCtx = context.WithValue(finalCtx, "cancel", canFunc) // 维护一个 cancel
	}

	return CreateYakitPluginContext(y.runtimeId).WithProxy(y.proxy).WithContext(finalCtx).WithVulFilter(y.getVulFilter()).WithContextCancel(canFunc)
}

func (y *YakToCallerManager) Set(ctx context.Context, code string, paramMap map[string]any, hook func(engine *antlr4yak.Engine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	args := []string{}
	for key, value := range paramMap {
		args = append(args, "--"+key, fmt.Sprintf("%s", value))
	}
	app := GetHookCliApp(args)
	var engine *antlr4yak.Engine
	cTable, err := y.fetchFunctionFromSourceCode(
		y.getYakitPluginContext(ctx).WithCliApp(app),
		WithFetchCode(code),
		WithFetchEngineHook(func(e *antlr4yak.Engine) error {
			if engine == nil {
				engine = e
			}
			if hook != nil {
				return hook(e)
			}
			return nil
		}),
		WithFetchFunctionNames(funcName...))
	if err != nil {
		return err
	}

	if y.table == nil {
		y.table = new(sync.Map)
	}

	for name, caller := range cTable {
		y.table.Store(name, []*Caller{
			{
				Core:   caller,
				Hash:   utils.CalcSha1(code, name),
				Engine: engine,
				// NativeFunction: caller.NativeYakFunction,
			},
		})
	}
	return nil
}

func (y *YakToCallerManager) Remove(params *ypb.RemoveHookParams) {
	if y.table == nil || params == nil {
		return
	}

	var keys []string
	y.table.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})

	if params.HookName == nil {
		params.HookName = keys
	}

	for _, k := range params.HookName {

		res, ok := y.table.Load(k)
		if !ok {
			continue
		}
		var existedCallers []*Caller
		list := res.([]*Caller)
		for _, l := range list {
			if params.ClearAll || utils.StringArrayContains(params.RemoveHookID, l.Id) {
				if k == HOOK_CLAER {
					y.CallPluginKeyByName(l.Id, HOOK_CLAER)
				}
				if iCancelFunc, ok := y.ContextCancelFuncs.Load(l.Id); ok {
					if cancelFunc, ok := iCancelFunc.(context.CancelFunc); ok {
						cancelFunc()
					}
				}

				// 如果启用了执行跟踪，清理相关的跟踪记录
				//if y.enableTracing {
				// 删除该插件和Hook的所有跟踪记录
				//removed := y.executionTracker.RemoveTracesByPluginAndHook(l.Id, k)
				//if removed > 0 {
				//	log.Debugf("清理插件执行跟踪: 插件[%s], Hook[%s], 删除了%d个跟踪记录", l.Id, k, removed)
				//}
				//}

				continue
			}
			existedCallers = append(existedCallers, l)
		}
		y.table.Store(k, existedCallers)
	}
}

func GetHookCliApp(tempArg []string) *cli.CliApp {
	app := cli.NewCliApp()
	app.SetArgs(tempArg)
	return app
}

//func HookCliArgs(nIns *antlr4yak.Engine, tempArgs []string) *cli.CliApp {
//	app := cli.NewCliApp()
//	app.SetArgs(tempArgs)
//	nIns.GetVM().SetVars(map[string]any{
//		"cli": cli.GetCliExportMapByCliApp(app),
//	})
//	return app
//	// nIns.GetVM().RegisterGlobalVariableFallback(h func(string) interface{})
//	// hook := func(f interface{}) interface{} {
//	// 	funcValue := reflect.ValueOf(f)
//	// 	funcType := funcValue.Type()
//	// 	hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
//	// 		TempParams := []cli.SetCliExtraParam{cli.SetTempArgs(tempArgs)}
//	// 		index := len(args) - 1 // 获取 option 参数的 index
//	// 		interfaceValue := args[index].Interface()
//	// 		args = args[:index]
//	// 		cliExtraParams, ok := interfaceValue.([]cli.SetCliExtraParam)
//	// 		if ok {
//	// 			TempParams = append(TempParams, cliExtraParams...)
//	// 		}
//	// 		for _, p := range TempParams {
//	// 			args = append(args, reflect.ValueOf(p))
//	// 		}
//	// 		res := funcValue.Call(args)
//	// 		return res
//	// 	})
//	// 	return hookFunc.Interface()
//	// }
//
//	// hookFuncList := []string{
//	// 	"String",
//	// 	"Bool",
//	// 	"Have",
//	// 	"Int",
//	// 	"Integer",
//	// 	"Float",
//	// 	"Double",
//	// 	"YakitPlugin",
//	// 	"Urls",
//	// 	"Url",
//	// 	"Ports",
//	// 	"Port",
//	// 	"Hosts",
//	// 	"Host",
//	// 	"Network",
//	// 	"Net",
//	// 	"File",
//	// 	"FileOrContent",
//	// 	"LineDict",
//	// 	"StringSlice",
//	// 	"FileNames",
//	// }
//	// for _, name := range hookFuncList {
//	// 	nIns.GetVM().RegisterMapMemberCallHandler("cli", name, hook)
//	// }
//}

func (y *YakToCallerManager) AddForYakit(
	ctx context.Context, script *schema.YakScript,
	paramMap map[string]any,
	code string, callerIf interface {
		Send(result *ypb.ExecResult) error
	},
	hooks ...string,
) error {
	caller := func(result *ypb.ExecResult) error {
		return callerIf.Send(result)
	}
	db := consts.GetGormProjectDatabase()
	return y.Add(ctx, script, paramMap, code, func(engine *antlr4yak.Engine) error {
		scriptName := script.ScriptName
		engine.OverrideRuntimeGlobalVariables(map[string]any{
			"yakit":           yaklib.GetExtYakitLibByClient(yaklib.NewVirtualYakitClient(caller)),
			"RUNTIME_ID":      y.runtimeId,
			"YAKIT_PLUGIN_ID": scriptName,
			"yakit_output":    FeedbackFactory(db, caller, false, scriptName),
			"yakit_save":      FeedbackFactory(db, caller, true, scriptName),
			"yakit_status": func(id string, i interface{}) {
				FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
					Id:   id,
					Data: fmt.Sprint(i),
				})
			},
		})
		return nil
	}, hooks...)
}

var fetchFilterMutex = new(sync.Mutex)

func (y *YakToCallerManager) getVulFilter() filter.Filterable {
	fetchFilterMutex.Lock()
	defer fetchFilterMutex.Unlock()
	if y.vulFilter != nil {
		return y.vulFilter
	}
	y.vulFilter = filter.NewMapFilter()
	return y.vulFilter
}

func (y *YakToCallerManager) Add(ctx context.Context, script *schema.YakScript, paramMap map[string]any, code string, hook func(*antlr4yak.Engine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	var (
		engine  *antlr4yak.Engine
		ctxInfo map[string]any
		ok      bool
	)
	id := script.ScriptName
	iCtxInfo := ctx.Value("ctx_info")
	ctxInfo, ok = iCtxInfo.(map[string]any)
	if !ok {
		ctxInfo = make(map[string]any)
	}
	if _, ok := ctxInfo["isNaslScript"]; ok {
		if v, ok := y.table.Load(HOOK_LoadNaslScriptByNameFunc); ok {
			v.(func(string))(id)
			return nil
		}
	}

	if y.dividedContext {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		ctx = context.WithValue(ctx, "cancel", cancel)
		y.ContextCancelFuncs.Store(id, cancel)
	}
	args := []string{}
	for key, value := range paramMap {
		args = append(args, "--"+key, fmt.Sprintf("%s", value))
	}
	app := GetHookCliApp(args)
	cTable, err := y.fetchFunctionFromSourceCode(y.getYakitPluginContext(ctx).WithCliApp(app),
		WithFetchScript(script),
		WithFetchCode(code),
		WithFetchEngineHook(func(e *antlr4yak.Engine) error {
			if engine == nil {
				engine = e
			}
			e.SetVars(map[string]any{
				"MITM_PARAMS": paramMap,
				"MITM_PLUGIN": id,
			})

			if hook != nil {
				return hook(e)
			}
			return nil
		}),
		WithFetchFunctionNames(funcName...),
	)

	if err != nil {
		return err
	}

	// 对于nasl插件还需要提取加载函数
	if _, ok := ctxInfo["isNaslScript"]; ok {
		f := func(name string) {
			if !strings.HasSuffix(strings.ToLower(name), ".nasl") {
				log.Errorf("call hook function `%v` of `%v` plugin failed: %s", HOOK_LoadNaslScriptByNameFunc, id, "nasl script name must end with .nasl")
				return
			}
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("call hook function `%v` of `%v` plugin failed: %s", HOOK_LoadNaslScriptByNameFunc, id, err)
					fmt.Println()
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			engine.CallYakFunction(ctx, HOOK_LoadNaslScriptByNameFunc, []any{name})
			if err != nil {
				log.Errorf("call YakFunction (DividedCTX) error: \n%v", err)
			}
		}
		f(id)
		y.table.Store(HOOK_LoadNaslScriptByNameFunc, f)
	}
	if y.table == nil {
		y.table = new(sync.Map)
	}

	for name, caller := range cTable {
		ins := &Caller{
			Core:   caller,
			Hash:   utils.CalcSha1(code, name, id),
			Id:     id,
			Engine: engine,
			// NativeFunction: caller.NativeYakFunction,
			Verbose: id,
		}

		// 如果启用了执行跟踪，为每个Hook函数创建跟踪记录
		// 注意：这里不再创建跟踪记录，而是在实际调用时创建，以确保每次调用都有独立的跟踪记录
		if y.enableTracing {
			log.Debugf("插件[%s] Hook[%s]已注册，将在执行时创建跟踪记录", id, name)
		}

		res, ok := y.table.Load(name)
		if !ok {
			y.table.Store(name, []*Caller{ins})
			continue
		}

		callerList := res.([]*Caller)
		currentIndex := -1
		for index, existed := range callerList {
			if existed.Id == id {
				currentIndex = index
				break
			}
		}
		if currentIndex >= 0 {
			callerList[currentIndex] = ins
		} else {
			callerList = append(callerList, ins)
		}

		y.table.Store(name, callerList)
	}
	return nil
}

func (y *YakToCallerManager) ShouldCallByName(name string, callbacks ...func()) (ret bool) {
	defer func() {
		if !ret {
			if len(callbacks) > 0 && callbacks[0] != nil {
				callbacks[0]() // 如果不需要执行，就执行结果回调
			}
		}
	}()
	if y.table == nil {
		return false
	}

	caller, ok := y.table.Load(name)
	if !ok {
		return false
	}

	c, ok := caller.([]*Caller)
	return ok && len(c) > 0
}

func (y *YakToCallerManager) CallByName(name string, items ...interface{}) {
	y.CallPluginKeyByName("", name, items...)
}

func (y *YakToCallerManager) CallByNameSync(name string, items ...interface{}) {
	y.CallPluginKeyByNameSyncWithCallback("", name, nil, items...)
}

func (y *YakToCallerManager) CallByNameWithCallback(name string, callback func(), items ...interface{}) {
	y.CallPluginKeyByNameWithCallback("", name, callback, items...)
}

func (y *YakToCallerManager) CallByNameExSync(name string, items ...func() interface{}) {
	y.SyncCallPluginKeyByNameEx("", name, nil, items...)
}

func (y *YakToCallerManager) CallPluginKeyByName(pluginId string, name string, items ...interface{}) {
	y.CallPluginKeyByNameWithCallback(pluginId, name, nil, items...)
}

func (y *YakToCallerManager) CallPluginKeyByNameWithCallback(pluginId string, name string, callback func(), items ...interface{}) {
	interfaceToClojure := func(i interface{}) func() interface{} {
		return func() interface{} {
			return i
		}
	}
	itemsFunc := funk.Map(items, interfaceToClojure).([]func() interface{})
	y.CallPluginKeyByNameEx(pluginId, name, callback, itemsFunc...)
}

func (y *YakToCallerManager) CallPluginKeyByNameSyncWithCallback(pluginId string, name string, callback func(), items ...interface{}) {
	interfaceToClojure := func(i interface{}) func() interface{} {
		return func() interface{} {
			return i
		}
	}
	itemsFunc := funk.Map(items, interfaceToClojure).([]func() interface{})
	y.SyncCallPluginKeyByNameEx(pluginId, name, callback, itemsFunc...)
}

func (y *YakToCallerManager) SyncCallPluginKeyByNameEx(pluginId string, name string, callback func(), itemsFuncs ...func() interface{}) {
	y.CallPluginKeyByNameExWithAsync(context.Background(), true, pluginId, name, callback, itemsFuncs...)
}

func (y *YakToCallerManager) CallPluginKeyByNameEx(pluginId string, name string, callback func(), itemsFuncs ...func() interface{}) {
	y.CallPluginKeyByNameExWithAsync(context.Background(), false, pluginId, name, callback, itemsFuncs...)
}

func (y *YakToCallerManager) CallPluginKeyByNameExWithAsync(runtimeCtx context.Context, forceSync bool, pluginId string, name string, callback func(), itemsFuncs ...func() interface{}) {
	y.Call(name,
		WithCallConfigRuntimeCtx(runtimeCtx),
		WithCallConfigForceSync(forceSync),
		WithCallConfigPluginId(pluginId),
		WithCallConfigCallback(callback),
		WithCallConfigItemFuncs(itemsFuncs...),
	)
}

// only forceSync can get results
func (y *YakToCallerManager) Call(name string, opts ...CallOpt) (results []any) {
	config := NewCallConfig()
	for _, opt := range opts {
		opt(config)
	}
	var (
		runtimeCtx = config.runtimeCtx
		forceSync  = config.forceSync
		pluginId   = config.pluginId
		callback   = config.callback
		items      = config.items
		itemsFuncs = config.itemFuncs
	)
	if len(itemsFuncs) == 0 && len(items) > 0 {
		itemsFuncs = lo.Map(items, func(i any, _ int) func() any {
			return func() any {
				return i
			}
		})
	}

	if y.table == nil {
		y.table = new(sync.Map)
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("call [%v] failed: %v， stack: %v", name, err, utils.ErrorStack(err))
			return
		}
	}()

	taskWG := new(sync.WaitGroup)
	isSync := y.swg == nil || forceSync
	y.baseWaitGroup.Add(1)
	defer func() {
		if !isSync {
			taskWG.Wait()
		}
		y.baseWaitGroup.Done()
		if callback != nil {
			callback()
		}
	}()

	caller, ok := y.table.Load(name)
	if !ok {
		utils.Debug(func() {
			log.Errorf("load[%s] hook failed: %s", name, "empty callers")
		})
		return
	}

	ins, ok := caller.([]*Caller)
	if !ok {
		utils.Debug(func() {
			log.Errorf("load[%s] hook failed: %s", name, "parse callers to []*Caller failed")
		})
		return
	}

	call := func(pluginRuntimeID string, i *Caller) any {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call failed: \n%v\n, stack: %v", err, utils.ErrorStack(err))
			}
		}()
		if (pluginId == "" /*执行所有该类型的插件*/) || (i.Id == pluginId /*执行当前插件*/) {
			var items []interface{}
			for _, i := range itemsFuncs {
				i := i
				items = append(items, i())
			}

			// 如果启用了执行跟踪，为每次调用创建新的跟踪记录
			var trace *PluginExecutionTrace
			if y.enableTracing {
				// 为每次调用创建新的跟踪记录
				trace = y.executionTracker.CreateTrace(i.Id, name, runtimeCtx)
				// 开始执行
				y.executionTracker.StartExecution(trace.TraceID, items)
				// 使用跟踪的上下文替换原始上下文
				runtimeCtx = trace.RuntimeCtx
				log.Debugf("创建新的插件执行跟踪: 插件[%s], Hook[%s], TraceID[%s]", i.Id, name, trace.TraceID)
			}

			log.Debugf("call %v.%v(params...)", i.Id, name)

			// 执行插件函数并处理跟踪
			var result any
			var execErr error
			func() {
				defer func() {
					if err := recover(); err != nil {
						// 区分不同类型的错误
						switch e := err.(type) {
						case error:
							execErr = e
						case string:
							execErr = fmt.Errorf("plugin execution error: %s", e)
						default:
							execErr = fmt.Errorf("plugin execution panic: %v", err)
						}

						// 记录详细的错误信息
						log.Errorf("插件[%s] Hook[%s]执行失败: %v", i.Id, name, execErr)

						if trace != nil {
							// 根据错误类型设置不同的状态
							if errors.Is(execErr, context.Canceled) {
								y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusCancelled, nil, execErr)
							} else {
								y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusFailed, nil, execErr)
							}
						}
					}
				}()

				// 检查是否被取消
				select {
				case <-runtimeCtx.Done():
					execErr = runtimeCtx.Err()
					if trace != nil {
						y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusCancelled, nil, execErr)
					}
					return
				default:
				}

				result = i.Core.Handler(
					func(frame *yakvm.Frame) {
						frame.GlobalVariables = frame.GlobalVariables.Deep1Clone()
						frame.GlobalVariables.Store(consts.PLUGIN_CONTEXT_KEY_RUNTIME_ID, pluginRuntimeID)
					}, items...)
			}()

			// 更新跟踪状态
			if trace != nil {
				if execErr != nil {
					if errors.Is(execErr, context.Canceled) {
						y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusCancelled, result, execErr)
					} else {
						y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusFailed, result, execErr)
					}
				} else {
					y.executionTracker.UpdateTraceStatus(trace.TraceID, PluginStatusCompleted, result, nil)
				}
			}

			return result
		}
		return nil
	}

	pluginRuntimeID := utils.InterfaceToString(runtimeCtx.Value(consts.PLUGIN_CONTEXT_KEY_RUNTIME_ID))
	for _, iRaw := range ins {
		verbose := iRaw.Verbose
		if iRaw.Id != verbose {
			verbose = fmt.Sprintf("%v[%v]", iRaw.Id, iRaw.Verbose)
		}

		// 没有设置并发控制，就直接顺序执行，需要处理上下文
		if isSync {
			log.Debugf("Start Call Plugin: %v", verbose)
			result := call(pluginRuntimeID, iRaw)
			results = append(results, result)
			continue
		} else {
			taskWG.Add(1)
		}

		// 设置了并发控制就这样
		i := iRaw
		go func() {
			y.swg.Add()
			go func() {
				defer func() {
					taskWG.Done()
					y.swg.Done()
					if err := recover(); err != nil {
						log.Errorf("panic from call[%v]: %v\nstack: %v", verbose, err, utils.ErrorStack(err))
					}
				}()
				if verbose != "" {
					log.Debugf("Start to Call Async Verbose: %v", verbose)
				}
				call(pluginRuntimeID, i)
				if verbose != "" {
					log.Debugf("Finished Calling Async Verbose: %v", verbose)
				}
			}()
		}()
	}
	return results
}

func (y *YakToCallerManager) Wait() {
	defer func() {
		y.vulFilter.Close()
		if r := recover(); r != nil {
			if errMsg := utils.InterfaceToString(r); errMsg != "" {
				log.Error(errMsg)
			}
		}
	}()

	if y.swg == nil {
		return
	}

	count := 0
	for {
		y.baseWaitGroup.Wait()
		y.swg.Wait()
		count++
		time.Sleep(300 * time.Millisecond)
		if count > 8 {
			break
		}
	}
}

func (y *YakToCallerManager) LoadPlugin(t string, hooks ...string) error {
	return loadScript(y, t, hooks...)
}

func (y *YakToCallerManager) LoadPluginContext(ctx context.Context, t string, hooks ...string) error {
	return loadScriptCtx(y, ctx, t, hooks...)
}

// LoadPluginByID loads a plugin by its database ID or UUID
// If id is int/int64, it will be treated as database ID
// If id is string, it will be treated as UUID
func (y *YakToCallerManager) LoadPluginByID(id interface{}, hooks ...string) error {
	return loadScriptByID(y, id, hooks...)
}

// LoadPluginByIDContext loads a plugin by its database ID or UUID with context
func (y *YakToCallerManager) LoadPluginByIDContext(ctx context.Context, id interface{}, hooks ...string) error {
	return loadScriptByIDCtx(y, ctx, id, hooks...)
}

// EnableExecutionTracing 启用插件执行跟踪
func (y *YakToCallerManager) EnableExecutionTracing(enable bool) {
	if !enable && y.enableTracing {
		y.GetExecutionTracker().CleanupCompletedTraces(-time.Minute * 5)
	}
	y.enableTracing = enable
}

// IsExecutionTracingEnabled 检查是否启用了插件执行跟踪
func (y *YakToCallerManager) IsExecutionTracingEnabled() bool {
	return y.enableTracing
}

// GetExecutionTracker 获取执行跟踪器
func (y *YakToCallerManager) GetExecutionTracker() *PluginExecutionTracker {
	return y.executionTracker
}

// AddExecutionTraceCallback 添加执行跟踪回调
func (y *YakToCallerManager) AddExecutionTraceCallback(callback func(*PluginExecutionTrace)) (callbackID string, remove func()) {
	return y.executionTracker.AddCallback(callback)
}

// GetAllExecutionTraces 获取所有执行跟踪
func (y *YakToCallerManager) GetAllExecutionTraces() []*PluginExecutionTrace {
	return y.executionTracker.GetAllTraces()
}

// GetExecutionTracesByPlugin 根据插件ID获取执行跟踪
func (y *YakToCallerManager) GetExecutionTracesByPlugin(pluginID string) []*PluginExecutionTrace {
	return y.executionTracker.GetTracesByPlugin(pluginID)
}

// GetExecutionTracesByHook 根据Hook名获取执行跟踪
func (y *YakToCallerManager) GetExecutionTracesByHook(hookName string) []*PluginExecutionTrace {
	return y.executionTracker.GetTracesByHook(hookName)
}

// GetRunningExecutionTraces 获取正在运行的执行跟踪
func (y *YakToCallerManager) GetRunningExecutionTraces() []*PluginExecutionTrace {
	return y.executionTracker.GetRunningTraces()
}

// CancelExecutionTrace 取消指定的执行跟踪
func (y *YakToCallerManager) CancelExecutionTrace(traceID string) bool {
	return y.executionTracker.CancelTrace(traceID)
}

// CancelAllExecutionTraces 取消所有执行跟踪
func (y *YakToCallerManager) CancelAllExecutionTraces() {
	y.executionTracker.CancelAllTraces()
}

// CleanupCompletedExecutionTraces 清理已完成的执行跟踪
func (y *YakToCallerManager) CleanupCompletedExecutionTraces(olderThan time.Duration) {
	y.executionTracker.CleanupCompletedTraces(olderThan)
}

func (y *YakToCallerManager) fetchFunctionFromSourceCode(pluginContext *YakitPluginContext, opts ...fetchFuncFromSrcCodeOptions) (map[string]*YakFunctionCaller, error) {
	cfg := NewYakitFetchFuncFromSrcCodeConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	engineHook := cfg.engineHook
	callArgumentHooks := cfg.callArgumentHooks
	script := cfg.script
	code := cfg.code
	functionNames := cfg.functionNames

	fTable := map[string]*YakFunctionCaller{}
	engine := NewScriptEngine(1) // 因为需要在 hook 里传回执行引擎, 所以这里不能并发
	engine.RegisterEngineHooks(engineHook)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		if script != nil {
			pluginContext.PluginName = script.ScriptName
			pluginContext.PluginUUID = script.Uuid
		}
		BindYakitPluginContextToEngine(engine, pluginContext)
		return nil
	})
	// engine.HookOsExit()
	// timeoutCtx, cancel := context.WithTimeout(ctx, loadTimeout)
	// defer func() { cancel() }()
	scriptName := ""
	if script != nil {
		scriptName = script.ScriptName
	}

	loadCtx, cancel := context.WithTimeout(pluginContext.Ctx, y.loadTimeout)
	defer cancel()
	ins, err := engine.ExecuteExWithContext(loadCtx, code, map[string]interface{}{
		"ROOT_CONTEXT": loadCtx,
		"YAK_FILENAME": scriptName,
	})
	if err != nil {
		log.Errorf("init execute plugin finished: %s", err)
		return nil, utils.Errorf("load plugin failed: %s", err)
	}

	for _, funcName := range functionNames {
		funcName := funcName
		//switch funcName {
		//case "execNuclei":
		//	log.Debugf("in execNuclei: %v", runtimeId)
		//}
		raw, ok := ins.GetVar(funcName)
		if !ok {
			continue
		}
		f, tOk := raw.(*yakvm.Function)
		if !tOk {
			continue
		}

		numIn := f.GetNumIn()

		nIns := ins
		fTable[funcName] = &YakFunctionCaller{
			Handler: func(callback func(*yakvm.Frame), args ...any) any {
				subCtx, cancel := context.WithTimeout(pluginContext.Ctx, y.callTimeout)
				defer cancel()
				if callArgHook, ok := callArgumentHooks[funcName]; ok {
					args = callArgHook(funcName, numIn, args)
				}

				errCh := make(chan error)
				valueCh := make(chan any)
				go func() {
					defer func() {
						if err := recover(); err != nil {
							fmtErr := utils.Errorf("call hook function `%v` of `%v` plugin failed: %s", funcName, scriptName, err)
							y.Err = fmtErr
							//log.Error(fmtErr)
							//fmt.Println()
							errCh <- fmtErr
							if os.Getenv("YAK_IN_TERMINAL_MODE") == "" {
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}
						close(errCh)
					}()
					value, err := nIns.CallYakFunctionNativeWithFrameCallback(subCtx, callback, f, args...)
					if err != nil {
						errCh <- err
					} else {
						valueCh <- value
					}
				}()

				select {
				case value := <-valueCh:
					return value
				case err := <-errCh:
					//if err != nil && !errors.Is(err, context.Canceled) {
					//	log.Errorf("call YakFunction (DividedCTX) error: \n%v", err)
					//}
					// 重要：抛出错误而不是返回nil，这样Call方法就能捕获到错误
					panic(err)
				case <-subCtx.Done():
					err := subCtx.Err()
					if errors.Is(err, context.DeadlineExceeded) {
						log.Errorf("call %s YakFunction timeout after %v seconds", scriptName, y.callTimeout)
					} else if errors.Is(err, context.Canceled) {

					} else {
						log.Errorf("call %s YakFunction done by unknown error: %v", scriptName, err)
					}
					// 重要：抛出超时或取消错误
					panic(err)
				}
				return nil
			},
		}

	}
	return fTable, nil
}

func BindYakitPluginContextToEngine(nIns *antlr4yak.Engine, pluginContext *YakitPluginContext) {
	if nIns == nil {
		return
	}
	var pluginName, runtimeId, proxy, pluginUUID string
	if pluginContext == nil {
		return
	}
	runtimeId = pluginContext.RuntimeId
	pluginName = pluginContext.PluginName
	pluginUUID = pluginContext.PluginUUID
	proxy = pluginContext.Proxy

	streamContext := context.Background()
	if pluginContext.Ctx != nil {
		streamContext = pluginContext.Ctx
	}

	cancel := pluginContext.Cancel
	if cancel == nil {
		streamContext, cancel = context.WithCancel(streamContext)
	}

	cliApp := cli.DefaultCliApp
	if pluginContext.CliApp != nil {
		cliApp = pluginContext.CliApp
	}
	cliApp.SetCliCheckCallback(func() {
		panic("cli check fail")
	})

	for _, mod := range []string{"db", "yakit"} {
		nIns.GetVM().RegisterMapMemberCallHandler(mod, "SavePortFromResult", func(i interface{}) interface{} {
			originFunc, ok := i.(func(u any, runtimeIds ...string) error)
			if ok {
				return func(u any, runtimeIds ...string) error {
					if len(runtimeIds) > 0 {
						runtimeIds = append(runtimeIds, runtimeId)
						return originFunc(u, runtimeIds...)
					}
					return originFunc(u, runtimeId)
				}
			}
			return i
		})
	}

	// db http flow
	nIns.GetVM().RegisterMapMemberCallHandler("db", "SaveHTTPFlowFromRawWithOption", func(i interface{}) interface{} {
		originFunc, ok := i.(func(url string, req, rsp []byte, exOption ...yakit.CreateHTTPFlowOptions) error)
		if ok {
			return func(url string, req, rsp []byte, exOption ...yakit.CreateHTTPFlowOptions) error {
				exOption = append(exOption, yakit.CreateHTTPFlowWithSource("scan"))
				exOption = append(exOption, yakit.CreateHTTPFlowWithRuntimeID(runtimeId))
				exOption = append(exOption, yakit.CreateHTTPFlowWithFromPlugin(pluginName))
				return originFunc(url, req, rsp, exOption...)
			}
		}
		return i
	})

	// poc
	hookPocFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			pocContextOpt := []poc.PocConfigOption{
				// poc.WithSource(pluginName),
				poc.WithFromPlugin(pluginName),
				poc.WithRuntimeId(runtimeId),
				poc.WithProxy(strings.Split(proxy, ",")...),
				poc.WithContext(streamContext),
			}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			pocExtraOpts, ok := interfaceValue.([]poc.PocConfigOption)
			if ok {
				pocExtraOpts = append(pocContextOpt, pocExtraOpts...)
			}
			for _, p := range pocExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}
	pocFuncList := []string{"Get", "Post", "Head", "Delete", "Options", "Do", "Websocket", "HTTP", "HTTPEx", "BuildRequest"}
	for _, funcName := range pocFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("poc", funcName, hookPocFunc)
	}

	// http
	hookHTTPFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			httpContextOpt := []http_struct.HttpOption{
				// yakhttp.WithSource(pluginName),
				yakhttp.WithFromPlugin(pluginName),
				yakhttp.WithRuntimeID(runtimeId),
				yakhttp.WithProxy(strings.Split(proxy, ",")...),
				yakhttp.WithContext(streamContext),
			}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			httpExtraOpts, ok := interfaceValue.([]http_struct.HttpOption)
			if ok {
				httpExtraOpts = append(httpContextOpt, httpExtraOpts...)
			}
			for _, p := range httpExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}
	httpFuncList := []string{"Get", "Post", "Request", "NewRequest", "RequestFaviconHash", "RequestToMD5", "RequestToSha1", "RequestToMMH3Hash128", "RequestToMMH3Hash128x64", "RequestToSha256"}
	for _, funcName := range httpFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("http", funcName, hookHTTPFunc)
	}

	nIns.GetVM().RegisterMapMemberCallHandler("nuclei", "Scan", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target any, opts ...any) (chan *tools.PocVul, error))
		if ok {
			return func(target any, opts ...any) (chan *tools.PocVul, error) {
				if runtimeId != "" {
					opts = append(opts, httptpl.WithHttpTplRuntimeId(runtimeId))
				}
				if streamContext != nil {
					opts = append(opts, httptpl.WithContext(streamContext))
				}
				opts = append(opts, httptpl.WithCustomVulnFilter(pluginContext.vulFilter))
				opts = append(opts, lowhttp.WithFromPlugin(pluginName))
				opts = append(opts, lowhttp.WithProxy(strings.Split(proxy, ",")...))
				return originFunc(target, opts...)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("nuclei", "ScanAuto", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target any, opts ...any))
		if ok {
			return func(target any, opts ...any) {
				if runtimeId != "" {
					opts = append(opts, httptpl.WithHttpTplRuntimeId(runtimeId))
				}
				if streamContext != nil {
					opts = append(opts, httptpl.WithContext(streamContext))
				}
				opts = append(opts, httptpl.WithCustomVulnFilter(pluginContext.vulFilter))
				opts = append(opts, lowhttp.WithFromPlugin(pluginName))
				opts = append(opts, lowhttp.WithProxy(strings.Split(proxy, ",")...))
				originFunc(target, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("fuzz", "HTTPRequest", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error))
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error) {
				opts = append([]mutate.BuildFuzzHTTPRequestOption{mutate.OptContext(pluginContext.Ctx)}, opts...)
				opts = append(opts, mutate.OptFromPlugin(pluginName))
				if runtimeId != "" {
					opts = append(opts, mutate.OptRuntimeId(runtimeId))
				}
				opts = append(opts, mutate.OptProxy(proxy))
				return originFunc(i, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("fuzz", "MustHTTPRequest", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest)
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest {
				opts = append([]mutate.BuildFuzzHTTPRequestOption{mutate.OptContext(pluginContext.Ctx)}, opts...)
				opts = append(opts, mutate.OptFromPlugin(pluginName))
				opts = append(opts, mutate.OptProxy(proxy))
				if runtimeId != "" {
					opts = append(opts, mutate.OptRuntimeId(runtimeId))
				}
				return originFunc(i, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("risk", "NewRisk", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target string, opts ...yakit.RiskParamsOpt))
		if ok {
			return func(target string, opts ...yakit.RiskParamsOpt) {
				opts = append(opts, yakit.WithRiskParam_FromScript(pluginName), yakit.WithRiskParam_YakScriptUUID(pluginUUID))
				if runtimeId != "" {
					opts = append(opts, yakit.WithRiskParam_RuntimeId(runtimeId))
				}
				originFunc(target, opts...)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("risk", "Save", func(i interface{}) interface{} {
		originFunc, ok := i.(func(r *schema.Risk) error)
		if ok {
			return func(r *schema.Risk) error {
				if runtimeId != "" {
					r.RuntimeId = runtimeId
				}
				return originFunc(r)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("risk", "CheckDNSLogByToken", func(i interface{}) interface{} {
		return yakit.YakitNewCheckDNSLogByToken(pluginContext.YakitPluginInfo)
	})

	nIns.GetVM().RegisterMapMemberCallHandler("risk", "CheckHTTPLogByToken", func(i interface{}) interface{} {
		return yakit.YakitNewCheckHTTPLogByToken(pluginContext.YakitPluginInfo)
	})

	nIns.GetVM().RegisterMapMemberCallHandler("risk", "CheckRandomTriggerByToken", func(i interface{}) interface{} {
		return yakit.YakitNewCheckRandomTriggerByToken(pluginContext.YakitPluginInfo)
	})

	nIns.GetVM().RegisterMapMemberCallHandler("risk", "CheckICMPTriggerByLength", func(i interface{}) interface{} {
		return yakit.YakitNewCheckICMPTriggerByLength(pluginContext.YakitPluginInfo)
	})

	nIns.GetVM().RegisterMapMemberCallHandler("crawler", "Start", func(i interface{}) interface{} {
		originFunc, ok := i.(func(string, ...crawler.ConfigOpt) (chan *crawler.Req, error))
		if ok {
			return func(url string, opts ...crawler.ConfigOpt) (chan *crawler.Req, error) {
				opts = append(opts, crawler.WithRuntimeID(runtimeId)) // add runtimeID for crawler
				return originFunc(url, opts...)
			}
		}
		log.Errorf("BUG: crawler.Start 's signature is override")
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("crawlerx", "StartCrawler", func(i interface{}) interface{} {
		originFunc, ok := i.(func(string, ...crawlerx.ConfigOpt) (chan crawlerx.ReqInfo, error))
		if ok {
			return func(url string, opts ...crawlerx.ConfigOpt) (chan crawlerx.ReqInfo, error) {
				opts = append(opts, crawlerx.WithRuntimeID(runtimeId))
				return originFunc(url, opts...)
			}
		}
		log.Errorf("BUG: crawlerx.StartCrawler 's signature is override")
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("simulator", "HttpBruteForce", func(i interface{}) interface{} {
		originFunc, ok := i.(func(string, ...simulator.BruteConfigOpt) (chan simulator.Result, error))
		if ok {
			return func(url string, opts ...simulator.BruteConfigOpt) (chan simulator.Result, error) {
				opts = append(opts, simulator.WithRuntimeID(runtimeId))
				return originFunc(url, opts...)
			}
		}
		log.Errorf("BUG: simulator.HttpBruteForce 's signature is override")
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("hook", "NewMixPluginCaller", func(i interface{}) interface{} {
		origin, ok := i.(func() (*MixPluginCaller, error))
		if ok {
			return func() (*MixPluginCaller, error) {
				manager, err := origin()
				if err != nil {
					return nil, err
				}
				log.Debugf("bind hook.NewMixPluginCaller to runtime: %v", runtimeId)
				manager.SetRuntimeId(runtimeId)
				manager.SetProxy(strings.Split(proxy, ",")...)
				manager.SetCtx(streamContext)
				if pluginContext.YakitClient != nil {
					manager.SetFeedback(func(i *ypb.ExecResult) error {
						return pluginContext.YakitClient.RawSend(i)
					})
				} else {
					manager.SetFeedback(func(result *ypb.ExecResult) error { // 临时解决方案
						yakitLib, ok := nIns.GetVar("yakit")
						if ok && yakitLib != nil {
							if v, ok := yakitLib.(map[string]interface{}); ok {
								if v2, ok := v["Output"]; ok {
									if v3, ok := v2.(func(i interface{}) error); ok {
										return v3(result)
									} else {
										return fmt.Errorf("yakit.Output is not func(i interface{}) error")
									}
								}
							}
						}
						return fmt.Errorf("not found current engine yakit.Output")
					})
				}
				return manager, nil
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("hook", "NewMixPluginCallerWithFilter", func(i interface{}) interface{} {
		origin, ok := i.(func(filterable filter.Filterable) (*MixPluginCaller, error))
		if ok {
			return func(filterable filter.Filterable) (*MixPluginCaller, error) {
				manager, err := origin(filterable)
				if err != nil {
					return nil, err
				}
				log.Debugf("bind hook.NewMixPluginCallerWithFilter to runtime: %v", runtimeId)
				manager.SetRuntimeId(runtimeId)
				manager.SetProxy(strings.Split(proxy, ",")...)
				manager.SetCtx(streamContext)
				if pluginContext.YakitClient != nil {
					manager.SetFeedback(func(i *ypb.ExecResult) error {
						return pluginContext.YakitClient.RawSend(i)
					})
				} else {
					manager.SetFeedback(func(result *ypb.ExecResult) error { // 临时解决方案
						yakitLib, ok := nIns.GetVar("yakit")
						if ok && yakitLib != nil {
							if v, ok := yakitLib.(map[string]interface{}); ok {
								if v2, ok := v["Output"]; ok {
									if v3, ok := v2.(func(i interface{}) error); ok {
										return v3(result)
									} else {
										return fmt.Errorf("yakit.Output is not func(i interface{}) error")
									}
								}
							}
						}
						return fmt.Errorf("not found current engine yakit.Output")
					})
				}
				return manager, nil
			}
		}
		return i
	})
	// context hook

	// new context
	nIns.GetVM().RegisterMapMemberCallHandler("context", "Seconds", func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			ctx, err := context.WithTimeout(streamContext, time.Duration(args[0].Float())*time.Second)
			if err != nil {
				log.Errorf("hook context Seconds failed: %v", err)
			}
			return []reflect.Value{reflect.ValueOf(ctx)}
		})
		return hookFunc.Interface()
	})

	newContextHook := func(f interface{}) interface{} {
		return func() context.Context {
			return streamContext
		}
	}
	nIns.GetVM().RegisterMapMemberCallHandler("context", "New", newContextHook)
	nIns.GetVM().RegisterMapMemberCallHandler("context", "Background", newContextHook)

	// hook sync context
	nIns.GetVM().RegisterMapMemberCallHandler("sync", "NewWaitGroup", func(f interface{}) interface{} {
		originFunc, ok := f.(func(ctxs ...context.Context) *yaklib.WaitGroupProxy)
		if ok {
			return func(ctxs ...context.Context) *yaklib.WaitGroupProxy {
				ctxs = append(ctxs, streamContext)
				return originFunc(ctxs...)
			}
		}
		return f
	})

	nIns.GetVM().RegisterMapMemberCallHandler("sync", "NewSizedWaitGroup", func(f interface{}) interface{} {
		originFunc, ok := f.(func(limit int, ctxs ...context.Context) *utils.SizedWaitGroup)
		if ok {
			return func(limit int, ctxs ...context.Context) *utils.SizedWaitGroup {
				ctxs = append(ctxs, streamContext)
				return originFunc(limit, ctxs...)
			}
		}
		return f
	})

	// hook httpserver context
	nIns.GetVM().RegisterMapMemberCallHandler("httpserver", "Serve", func(f interface{}) interface{} {
		originFunc, ok := f.(func(host string, port int, opts ...yaklib.HttpServerConfigOpt) error)
		if ok {
			return func(host string, port int, opts ...yaklib.HttpServerConfigOpt) error {
				opts = append([]yaklib.HttpServerConfigOpt{yaklib.HTTPServer_ServeOpt_Context(streamContext)}, opts...)
				return originFunc(host, port, opts...)
			}
		}
		return f
	})

	// hook traceroute context
	nIns.GetVM().RegisterMapMemberCallHandler("traceroute", "Diagnostic", func(f interface{}) interface{} {
		originFunc, ok := f.(func(host string, opts ...pingutil.TracerouteConfigOption) (chan *pingutil.TracerouteResponse, error))
		if ok {
			return func(host string, opts ...pingutil.TracerouteConfigOption) (chan *pingutil.TracerouteResponse, error) {
				opts = append([]pingutil.TracerouteConfigOption{pingutil.WithCtx(streamContext)}, opts...)
				return originFunc(host, opts...)
			}
		}
		return f
	})

	// hook udp context
	nIns.GetVM().RegisterMapMemberCallHandler("udp", "Serve", func(f interface{}) interface{} {
		originFunc, ok := f.(func(host string, port interface{}, opts ...yaklib.UdpServerOpt) error)
		if ok {
			return func(host string, port interface{}, opts ...yaklib.UdpServerOpt) error {
				opts = append([]yaklib.UdpServerOpt{yaklib.UdpWithContext(streamContext)}, opts...)
				return originFunc(host, port, opts...)
			}
		}
		return f
	})

	// hook tcp context
	nIns.GetVM().RegisterMapMemberCallHandler("tcp", "Serve", func(f interface{}) interface{} {
		originFunc, ok := f.(func(host interface{}, port int, opts ...yaklib.TcpServerConfigOpt) error)
		if ok {
			return func(host interface{}, port int, opts ...yaklib.TcpServerConfigOpt) error {
				opts = append([]yaklib.TcpServerConfigOpt{yaklib.Tcp_Server_Context(streamContext)}, opts...)
				return originFunc(host, port, opts...)
			}
		}
		return f
	})

	// hook mitm start context
	nIns.GetVM().RegisterMapMemberCallHandler("mitm", "Start", func(f interface{}) interface{} {
		originFunc, ok := f.(func(port int, opts ...yaklib.MitmConfigOpt) error)
		if ok {
			return func(port int, opts ...yaklib.MitmConfigOpt) error {
				opts = append([]yaklib.MitmConfigOpt{yaklib.MitmConfigContext(streamContext)}, opts...)
				return originFunc(port, opts...)
			}
		}
		return f
	})
	nIns.GetVM().RegisterMapMemberCallHandler("mitm", "Bridge", func(f interface{}) interface{} {
		originFunc, ok := f.(func(port interface{}, downstreamProxy string, opts ...yaklib.MitmConfigOpt) error)
		if ok {
			return func(port interface{}, downstreamProxy string, opts ...yaklib.MitmConfigOpt) error {
				opts = append([]yaklib.MitmConfigOpt{yaklib.MitmConfigContext(streamContext)}, opts...)
				return originFunc(port, downstreamProxy, opts...)
			}
		}
		return f
	})

	// hook git context
	hookGitFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			gitContextOpt := []yakgit.Option{yakgit.WithContext(streamContext)}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			gitExtraOpts, ok := interfaceValue.([]yakgit.Option)
			if ok {
				gitExtraOpts = append(gitContextOpt, gitExtraOpts...)
			}
			for _, p := range gitExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}
	gitFuncList := []string{"GitHack", "Clone", "Pull", "Fetch", "Checkout", "IterateCommit"}
	for _, funcName := range gitFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("git", funcName, hookGitFunc)
	}

	// hook http_pool context
	nIns.GetVM().RegisterMapMemberCallHandler("httpool", "Pool", func(f interface{}) interface{} {
		originFunc, ok := f.(func(i interface{}, opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error))
		if ok {
			return func(i interface{}, opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error) {
				opts = append([]mutate.HttpPoolConfigOption{mutate.WithPoolOpt_Context(streamContext)}, opts...)
				return originFunc(i, opts...)
			}
		}
		return f
	})

	// os.
	nIns.GetVM().RegisterMapMemberCallHandler("os", "Exit", func(f interface{}) interface{} {
		return func(code int) {
			cancel()
		}
	})

	// cli hook
	nIns.GetVM().SetVars(map[string]any{
		"cli": cli.GetCliExportMapByCliApp(cliApp),
	})

	// hook webservice runtime id
	hookServiceScanFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			serviceScanOpt := []fp.ConfigOption{fp.WithRuntimeId(runtimeId), fp.WithCtx(streamContext)}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			serviceScanExtraOpts, ok := interfaceValue.([]fp.ConfigOption)
			if ok {
				serviceScanExtraOpts = append(serviceScanOpt, serviceScanExtraOpts...)
			}
			for _, p := range serviceScanExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}

	ServiceScanFuncList := []string{"Scan", "ScanOne", "ScanFromSynResult", "ScanFromSpaceEngine", "ScanFromPing"}
	for _, funcName := range ServiceScanFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("servicescan", funcName, hookServiceScanFunc)
	}

	// hook synscan runtime id
	hookSynScanFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			synScanOpt := []synscanx.SynxConfigOption{synscanx.WithRuntimeId(runtimeId), synscanx.WithCtx(streamContext)}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			synScanExtraOpts, ok := interfaceValue.([]synscanx.SynxConfigOption)
			if ok {
				synScanExtraOpts = append(synScanOpt, synScanExtraOpts...)
			}
			for _, p := range synScanExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}

	SynScanFuncList := []string{"Scan", "ScanFromPing"}
	for _, funcName := range SynScanFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("synscan", funcName, hookSynScanFunc)
	}

	hookBruteFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			bruteOpt := []tools.BruteOpt{tools.WithBruteRuntimeId(runtimeId), tools.WithBruteCtx(streamContext)}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			bruteExtraOpts, ok := interfaceValue.([]tools.BruteOpt)
			if ok {
				bruteExtraOpts = append(bruteOpt, bruteExtraOpts...)
			}
			for _, p := range bruteExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}

	nIns.GetVM().RegisterMapMemberCallHandler("brute", "New", hookBruteFunc)

	hookPingScanFunc := func(f interface{}) interface{} {
		funcValue := reflect.ValueOf(f)
		funcType := funcValue.Type()
		hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
			pingScanOpt := []tools.PingConfigOpt{tools.WithPingRuntimeId(runtimeId), tools.WithPingCtx(streamContext)}
			index := len(args) - 1 // 获取 option 参数的 index
			interfaceValue := args[index].Interface()
			args = args[:index]
			pingScanExtraOpts, ok := interfaceValue.([]tools.PingConfigOpt)
			if ok {
				pingScanExtraOpts = append(pingScanOpt, pingScanExtraOpts...)
			}
			for _, p := range pingScanExtraOpts {
				args = append(args, reflect.ValueOf(p))
			}
			res := funcValue.Call(args)
			return res
		})
		return hookFunc.Interface()
	}

	PingScanFuncList := []string{"Scan", "Ping"}
	for _, funcName := range PingScanFuncList {
		nIns.GetVM().RegisterMapMemberCallHandler("ping", funcName, hookPingScanFunc)
	}
}

//func (y *YakToCallerManager) GetCurrentHookStack() {
//	if (y.table) == nil {
//		return
//	}
//
//	y.table.Range(func(key, value interface{}) bool {
//		hookName := key.(string)
//		callers := value.([]*Caller)
//		for _, i := range callers {
//			i.NativeFunction.Call()
//		}
//		return true
//	})
//}

func loadScript(mng *YakToCallerManager, scriptType string, hookNames ...string) error {
	return loadScriptCtx(mng, context.Background(), scriptType, hookNames...)
}

func loadScriptByName(mng *YakToCallerManager, scriptName string, hookNames ...string) error {
	return loadScriptByNameCtx(mng, context.Background(), scriptName, hookNames...)
}

func CallYakitPluginFunc(scriptName string, hookName string) (interface{}, error) {
	if currentCoreEngine == nil {
		return nil, utils.Error("call cross plugin need engine preset(yak your-file.yak only)")
	}
	if haveSetCurrentCoreEngine {
		log.Warn("DO NOT USE THIS FUNC: hook.CallYakitPluginFunc in HotPatch (MITM/WebFuzzer)!")
		log.Warn("DO NOT USE THIS FUNC: hook.CallYakitPluginFunc in HotPatch (MITM/WebFuzzer)!")
		log.Warn("DO NOT USE THIS FUNC: hook.CallYakitPluginFunc in HotPatch (MITM/WebFuzzer)!")
		return nil, utils.Error("current engine have been changed.")
	}
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), scriptName)
	if err != nil {
		log.Errorf("load yak script failed: %s", err)
		return nil, err
	}

	value, err := ImportVarFromScript(currentCoreEngine, script.Content, hookName)
	if err != nil {
		return nil, err
	}
	//if !value.Callable() {
	//	return nil, utils.Errorf("%v' %v is not callable", scriptName, hookName)
	//}
	return value, nil
}

func loadScriptCtx(mng *YakToCallerManager, ctx context.Context, scriptType string, hookNames ...string) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	db = db.Model(&schema.YakScript{}).Where("type = ?", scriptType)
	counter := 0
	for script := range yakit.YieldYakScripts(db, ctx) {
		counter++
		err := mng.AddForYakit(ctx, script, nil, script.Content, YakitCallerIf(func(result *ypb.ExecResult) error {
			return nil
		}), hookNames...)
		if err != nil {
			return err
		}
	}

	if counter <= 0 {
		return utils.Error("no script loading")
	}
	return nil
}

func removeScriptByNameCtx(mng *YakToCallerManager, scriptNames ...string) {
	mng.Remove(&ypb.RemoveHookParams{
		RemoveHookID: scriptNames,
	})
}

func loadScriptByNameCtx(mng *YakToCallerManager, ctx context.Context, scriptName string, hookNames ...string) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	db = db.Model(&schema.YakScript{}).Where("script_name = ?", scriptName)
	counter := 0
	for script := range yakit.YieldYakScripts(db, ctx) {
		counter++
		err := mng.AddForYakit(ctx, script, nil, script.Content, YakitCallerIf(func(result *ypb.ExecResult) error {
			return nil
		}), hookNames...)
		if err != nil {
			return err
		}
	}

	if counter <= 0 {
		return utils.Error("no script loading")
	}
	return nil
}

// loadScriptByID loads a plugin by its database ID or UUID
// If id is int64, it will be treated as database ID
// If id is string, it will be treated as UUID
func loadScriptByID(mng *YakToCallerManager, id interface{}, hookNames ...string) error {
	return loadScriptByIDCtx(mng, context.Background(), id, hookNames...)
}

func loadScriptByIDCtx(mng *YakToCallerManager, ctx context.Context, id interface{}, hookNames ...string) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("database connection is nil")
	}

	var script *schema.YakScript
	var err error

	switch v := id.(type) {
	case int:
		script, err = yakit.GetYakScript(db, int64(v))
	case int64:
		script, err = yakit.GetYakScript(db, v)
	case string:
		script, err = yakit.GetYakScriptByUUID(db, v)
	default:
		return utils.Errorf("unsupported id type: %T, expected int64 (database ID) or string (UUID)", id)
	}

	if err != nil {
		return utils.Errorf("failed to get script by id: %v", err)
	}

	if script == nil {
		return utils.Error("script not found")
	}

	// use script name to load the plugin
	return loadScriptByNameCtx(mng, ctx, script.ScriptName, hookNames...)
}

func NewFetchFuncFromSrcCodeConfig() *fetchFuncFromSrcCodeConfig {
	return &fetchFuncFromSrcCodeConfig{}
}

func DefaultBeforeRequestCallArgumentHook(name string, numIn int, args []any) []any {
	// fallback
	if len(args) < 3 {
		return args
	}
	if numIn == 3 {
		// isHttps, originReq, req
		// full version, return
		return args
	} else if numIn == 1 {
		// req
		// short version
		args = []any{args[2]}
	}
	return args
}

func DefaultAfterRequestCallArgumentHook(name string, numIn int, args []any) []any {
	// fallback
	if len(args) < 5 {
		return args
	}
	if numIn == 5 {
		// isHttps, originReq, req, originRsp, rsp
		// full version, return
		return args
	} else if numIn == 1 {
		// rsp
		// short version
		args = []any{args[4]}
	}
	return args
}

func NewYakitFetchFuncFromSrcCodeConfig() *fetchFuncFromSrcCodeConfig {
	cfg := NewFetchFuncFromSrcCodeConfig()
	cfg.callArgumentHooks = make(map[string]callArgumentHookFunc)
	cfg.callArgumentHooks[HOOK_BeforeRequest] = DefaultBeforeRequestCallArgumentHook
	cfg.callArgumentHooks[HOOK_AfterRequest] = DefaultAfterRequestCallArgumentHook
	return cfg
}

func WithFetchScript(script *schema.YakScript) fetchFuncFromSrcCodeOptions {
	return func(c *fetchFuncFromSrcCodeConfig) {
		c.script = script
	}
}

func WithFetchCode(code string) fetchFuncFromSrcCodeOptions {
	return func(c *fetchFuncFromSrcCodeConfig) {
		c.code = code
	}
}

func WithFetchEngineHook(hook func(e *antlr4yak.Engine) error) fetchFuncFromSrcCodeOptions {
	return func(c *fetchFuncFromSrcCodeConfig) {
		c.engineHook = hook
	}
}

func WithFetchCallArgumentHook(name string, hook callArgumentHookFunc) fetchFuncFromSrcCodeOptions {
	return func(c *fetchFuncFromSrcCodeConfig) {
		if c.callArgumentHooks == nil {
			c.callArgumentHooks = make(map[string]callArgumentHookFunc)
		}
		c.callArgumentHooks[name] = hook
	}
}

func WithFetchFunctionNames(names ...string) fetchFuncFromSrcCodeOptions {
	return func(c *fetchFuncFromSrcCodeConfig) {
		c.functionNames = names
	}
}

func buildHotpatchHandler(ctx context.Context, code string) func(s string, yield func(s string)) (err error) {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	engine := NewScriptEngine(1)
	codeEnv, err := engine.ExecuteExWithContext(ctx, code, make(map[string]interface{}))
	if err != nil {
		log.Errorf("load hotPatch code error: %s", err)
		return nil
	}

	return func(s string, yield func(s string)) (err error) {
		handle, params, _ := strings.Cut(s, "|")
		logAndWrapError := func(errStr string) error {
			errInfo := fmt.Sprintf("%s%s", fuzztag.YakHotPatchErr, errStr)
			log.Errorf("call hotPatch code error: %v", errStr)
			return utils.Error(errInfo)
		}

		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(*yakvm.VMPanic); ok {
					log.Errorf("call hotPatch code error: %v", e.GetData())
					err = fmt.Errorf("%v", e.GetData())
				}
			}
		}()
		yakVar, ok := codeEnv.GetVar(handle)
		if !ok {
			return logAndWrapError(fmt.Sprintf("function %s not found", handle))
		}
		yakFunc, ok := yakVar.(*yakvm.Function)
		if !ok {
			return logAndWrapError(fmt.Sprintf("function %s not found", handle))
		}
		iparams := make([]any, 0, 1)
		numIn := yakFunc.GetNumIn()
		if numIn == 1 {
			// func handle(params) params , return []string
			iparams = append(iparams, params)
			data, err := codeEnv.CallYakFunction(ctx, handle, iparams)
			if err != nil {
				return logAndWrapError(err.Error())
			}
			if data == nil {
				return logAndWrapError("return nil")
			}

			res := utils.InterfaceToStringSlice(data)
			for _, item := range res {
				yield(item)
			}
			return nil
		} else if numIn == 2 {
			// func handle(params, yield), return nil
			iparams = append(iparams, params)
			iparams = append(iparams, yield)
			_, err := codeEnv.CallYakFunction(ctx, handle, iparams)
			if err != nil {
				return logAndWrapError(err.Error())
			}
			return nil
		}
		return logAndWrapError("invalid function params")
	}
}

func Fuzz_WithDynHotPatch(ctx context.Context, code string) mutate.FuzzConfigOpt {
	if hotPatchFunc := buildHotpatchHandler(ctx, code); hotPatchFunc != nil {
		return mutate.Fuzz_WithExtraFuzzTag("yak:dyn", mutate.HotPatchDynFuzztag(hotPatchFunc))
	}
	return mutate.Fuzz_WithExtraFuzzTagHandler("yak:dyn", func(s string) []string {
		return []string{s}
	})
}

func Fuzz_WithHotPatch(ctx context.Context, code string) mutate.FuzzConfigOpt {
	if hotPatchFunc := buildHotpatchHandler(ctx, code); hotPatchFunc != nil {
		return mutate.Fuzz_WithExtraFuzzTag("yak", mutate.HotPatchFuzztag(hotPatchFunc))
	}
	return mutate.Fuzz_WithExtraFuzzTagHandler("yak", func(s string) []string {
		return []string{s}
	})
}

func Fuzz_WithAllHotPatch(ctx context.Context, code string) []mutate.FuzzConfigOpt {
	if hotPatchFunc := buildHotpatchHandler(ctx, code); hotPatchFunc != nil {
		return []mutate.FuzzConfigOpt{
			mutate.Fuzz_WithExtraFuzzTag("yak", mutate.HotPatchFuzztag(hotPatchFunc)),
			mutate.Fuzz_WithExtraFuzzTag("yak:dyn", mutate.HotPatchDynFuzztag(hotPatchFunc)),
		}
	}
	return []mutate.FuzzConfigOpt{
		mutate.Fuzz_WithExtraFuzzTagHandler("yak", func(s string) []string {
			return []string{s}
		}),
		mutate.Fuzz_WithExtraFuzzTagHandler("yak:dyn", func(s string) []string {
			return []string{s}
		}),
	}
}
