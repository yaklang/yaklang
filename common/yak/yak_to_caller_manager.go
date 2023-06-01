package yak

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
)

const HOOK_CLAER = "clear"

type YakFunctionCaller struct {
	Handler func(args ...interface{})
}

func FetchFunctionFromSourceCode(ctx context.Context, timeout time.Duration, id string, code string, hook func(e yaklang.YaklangEngine) error, functionNames ...string) (map[string]*YakFunctionCaller, error) {
	var fTable = map[string]*YakFunctionCaller{}

	engine := NewScriptEngine(100)
	engine.RegisterEngineHooks(func(engine yaklang.YaklangEngine) error {
		if hook != nil {
			return hook(engine)
		}
		return nil
	})
	engine.HookOsExit()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer func() { cancel() }()
	ins, err := engine.ExecuteExWithContext(timeoutCtx, code, map[string]interface{}{
		"ROOT_CONTEXT": ctx,
	})
	if err != nil {
		log.Errorf("init execute plugin finished: %s", err)
		return nil, utils.Errorf("load plugin failed: %s", err)
	}

	for _, funcName := range functionNames {
		funcName := funcName
		raw, ok := ins.GetVar(funcName)
		if !ok {
			continue
		}
		f, tOk := raw.(*yakvm.Function)
		if !tOk {
			continue
		}

		nIns, eOk := ins.(*antlr4yak.Engine)
		if eOk {
			fTable[funcName] = &YakFunctionCaller{
				Handler: func(args ...interface{}) {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("call [%v] yakvm native function failed: %s", funcName, err)
							fmt.Println()
							utils.PrintCurrentGoroutineRuntimeStack()
						}
					}()

					_, err = nIns.CallYakFunctionNative(ctx, f, args...)
					if err != nil {
						log.Errorf("call YakFunction (DividedCTX) error: \n%v", err)
					}
				},
			}
		}
	}
	return fTable, nil

}

type Caller struct {
	Core    *YakFunctionCaller
	Hash    string
	Id      string
	Verbose string
	Engine  yaklang.YaklangEngine
	// NativeFunction *exec.Function
}

type YakToCallerManager struct {
	table          *sync.Map
	swg            *utils.SizedWaitGroup
	dividedContext bool
	timeout        time.Duration
}

func (c *YakToCallerManager) SetLoadPluginTimeout(i float64) {
	c.timeout = time.Duration(i * float64(time.Second))
}
func (y *YakToCallerManager) SetDividedContext(b bool) {
	y.dividedContext = b
}

func NewYakToCallerManager() *YakToCallerManager {
	return &YakToCallerManager{table: new(sync.Map), timeout: 10 * time.Second}
}

func (m *YakToCallerManager) SetConcurrent(i int) error {
	if m.swg != nil {
		err := utils.Error("cannot set swg for YakToCallerManager: existed swg")
		log.Error(err)
		return err
	}
	swg := utils.NewSizedWaitGroup(i)
	m.swg = &swg
	return nil
}

type CallerHooks struct {
	HookName string

	Hooks []*CallerHookDescription
}

type CallerHookDescription struct {
	// 这两个是
	YakScriptId   string
	YakScriptName string
	VerboseName   string
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
	code string, callerIf interface {
		Send(result *ypb.ExecResult) error
	},
	hooks ...string) error {
	caller := func(result *ypb.ExecResult) error {
		return callerIf.Send(result)
	}
	db := consts.GetGormProjectDatabase()
	return y.Set(ctx, code, func(engine yaklang.YaklangEngine) error {
		antlr4engine, ok := engine.(*antlr4yak.Engine)
		if ok {
			yaklib.SetEngineClient(antlr4engine, yaklib.NewVirtualYakitClient(func(i interface{}) error {
				switch ret := i.(type) {
				case *yaklib.YakitProgress:
					raw, _ := yaklib.YakitMessageGenerator(ret)
					if err := caller(&ypb.ExecResult{
						Hash:       "",
						OutputJson: "",
						Raw:        nil,
						IsMessage:  true,
						Message:    raw,
						Id:         0,
						RuntimeID:  "",
					}); err != nil {
						return err
					}
				case *yaklib.YakitLog:
					raw, _ := yaklib.YakitMessageGenerator(ret)
					if raw != nil {
						if err := caller(&ypb.ExecResult{
							IsMessage: true,
							Message:   raw,
						}); err != nil {
							return err
						}
					}
				}
				return nil
			}))
		}

		engine.SetVar("yakit_output", FeedbackFactory(db, caller, false, "default"))
		engine.SetVar("yakit_save", FeedbackFactory(db, caller, true, "default"))
		engine.SetVar("yakit_status", func(id string, i interface{}) {
			FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
				Id:   id,
				Data: fmt.Sprint(i),
			})
		})
		return nil
	}, hooks...)
}

func (y *YakToCallerManager) Set(ctx context.Context, code string, hook func(engine yaklang.YaklangEngine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	var engine yaklang.YaklangEngine
	var fetchFuncHandler = FetchFunctionFromSourceCode

	cTable, err := fetchFuncHandler(ctx, y.timeout, "", code, func(e yaklang.YaklangEngine) error {
		if engine == nil {
			engine = e
		}
		if hook != nil {
			return hook(e)
		}
		return nil
	}, funcName...)
	if err != nil {
		return utils.Errorf(err.Error())
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
				//NativeFunction: caller.NativeYakFunction,
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
	if params.HookName == nil && params.ClearAll {
		y.CallByName(HOOK_CLAER)
		for _, k := range keys {
			y.table.Delete(k)
		}
		return
	}

	if params.HookName == nil {
		params.HookName = keys
	}

	for _, k := range params.HookName {
		if params.ClearAll {
			if k == HOOK_CLAER {
				y.CallByName(k)
			}
			y.table.Delete(k)
			continue
		}

		res, ok := y.table.Load(k)
		if !ok {
			continue
		}
		var existedCallers []*Caller
		list := res.([]*Caller)
		for _, l := range list {
			if utils.StringArrayContains(params.RemoveHookID, l.Id) {
				if k == HOOK_CLAER {
					y.CallPluginKeyByName(l.Id, HOOK_CLAER)
				}
				continue
			}
			existedCallers = append(existedCallers, l)
		}
		y.table.Store(k, existedCallers)
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
			//mitmSaveToDBLock.Lock()
			//yakit.SaveExecResult(db, yakScriptName, result)
			//mitmSaveToDBLock.Unlock()
		}

		caller(result)
		if err != nil {
			return
		}
		return
	}
}

type YakitCallerIf func(result *ypb.ExecResult) error

func (y YakitCallerIf) Send(i *ypb.ExecResult) error {
	return y(i)
}

func (y *YakToCallerManager) AddGoNative(id string, name string, cb func(...interface{})) {
	if cb == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			//retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	ins := &Caller{
		Core: &YakFunctionCaller{
			Handler: func(args ...interface{}) {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("call go native code failed: %s", err)
					}
				}()
				if cb != nil {
					cb(args...)
					return
				}
			},
			//NativeYakFunction: nil,
		},
		Hash: utils.CalcSha1(name, id),
		Id:   id,
		//NativeFunction: caller.NativeYakFunction,
		Verbose: id,
	}

	res, ok := y.table.Load(name)
	if !ok {
		y.table.Store(name, []*Caller{ins})
		return
	}
	callers := res.([]*Caller)
	var targetIndex = -1
	for index, c := range callers {
		if c.Hash == ins.Hash {
			targetIndex = index
			break
		}
	}
	if targetIndex >= 0 {
		callers[targetIndex] = ins
	}
	y.table.Store(name, callers)
}

func bindYakitPluginToYakEngine(id string, nIns *antlr4yak.Engine) {
	if nIns == nil {
		return
	}
	nIns.GetVM().RegisterMapMemberCallHandler("poc", "HTTP", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...yaklib.PocConfig) ([]byte, []byte, error))
		if ok {
			return func(raw interface{}, opts ...yaklib.PocConfig) ([]byte, []byte, error) {
				opts = append(opts, yaklib.PoCOptWithSource(id))
				return originFunc(raw, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("fuzz", "HTTPRequest", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error))
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error) {
				opts = append(opts, mutate.OptSource(id))
				return originFunc(i, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("fuzz", "MustHTTPRequest", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest)
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest {
				opts = append(opts, mutate.OptSource(id))
				return originFunc(i, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("risk", "NewRisk", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target string, opts ...yakit.RiskParamsOpt))
		if ok {
			return func(target string, opts ...yakit.RiskParamsOpt) {
				opts = append(opts, yakit.WithRiskParam_YakitPluginName(id))
				originFunc(target, opts...)
			}
		}
		return i
	})
}

func (y *YakToCallerManager) AddForYakit(
	ctx context.Context, id string,
	params []*ypb.ExecParamItem,
	code string, callerIf interface {
		Send(result *ypb.ExecResult) error
	},
	hooks ...string) error {
	caller := func(result *ypb.ExecResult) error {
		return callerIf.Send(result)
	}
	db := consts.GetGormProjectDatabase()
	return y.Add(ctx, id, params, code, func(engine yaklang.YaklangEngine) error {
		engine.SetVar("YAKIT_PLUGIN_ID", id)
		engine.SetVar("yakit_output", FeedbackFactory(db, caller, false, id))
		engine.SetVar("yakit_save", FeedbackFactory(db, caller, true, id))
		engine.SetVar("yakit_status", func(id string, i interface{}) {
			FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
				Id:   id,
				Data: fmt.Sprint(i),
			})
		})
		if nIns, ok := engine.(*antlr4yak.Engine); ok && id != "" {
			bindYakitPluginToYakEngine(id, nIns)
		}
		return nil
	}, hooks...)
}

func (y *YakToCallerManager) Add(ctx context.Context, id string, params []*ypb.ExecParamItem, code string, hook func(yaklang.YaklangEngine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	var engine yaklang.YaklangEngine
	var fetchFuncHandler = FetchFunctionFromSourceCode
	//if y.dividedContext {
	//	fetchFuncHandler = FetchFunctionFromCallerDividedContext
	//} else {
	//	fetchFuncHandler = FetchFunctionFromCaller
	//}
	cTable, err := fetchFuncHandler(ctx, y.timeout, id, code, func(e yaklang.YaklangEngine) error {
		if engine == nil {
			engine = e
		}
		var paramMap = make(map[string]string)
		for _, p := range params {
			paramMap[p.Key] = p.Value
		}

		e.SetVar("MITM_PARAMS", paramMap)
		e.SetVar("MITM_PLUGIN", id)

		if hook != nil {
			return hook(e)
		}
		return nil
	}, funcName...)
	if err != nil {
		return utils.Errorf(err.Error())
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
			//NativeFunction: caller.NativeYakFunction,
			Verbose: id,
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

func (y *YakToCallerManager) CallByName(name string, items ...interface{}) {
	y.CallPluginKeyByName("", name, items...)
}

func (y *YakToCallerManager) CallByNameEx(name string, items ...func() interface{}) {
	y.CallPluginKeyByNameEx("", name, items...)
}

func (y *YakToCallerManager) CallByNameExSync(name string, items ...func() interface{}) {
	y.SyncCallPluginKeyByNameEx("", name, items...)
}

func (y *YakToCallerManager) CallPluginKeyByName(pluginId string, name string, items ...interface{}) {
	interfaceToClojure := func(i interface{}) func() interface{} {
		return func() interface{} {
			return i
		}
	}
	itemsFunc := funk.Map(items, interfaceToClojure).([]func() interface{})
	y.CallPluginKeyByNameEx(pluginId, name, itemsFunc...)
}

func (y *YakToCallerManager) SyncCallPluginKeyByNameEx(pluginId string, name string, itemsFuncs ...func() interface{}) {
	y.CallPluginKeyByNameExWithAsync(true, pluginId, name, itemsFuncs...)
}

func (y *YakToCallerManager) CallPluginKeyByNameEx(pluginId string, name string, itemsFuncs ...func() interface{}) {
	y.CallPluginKeyByNameExWithAsync(false, pluginId, name, itemsFuncs...)
}
func (y *YakToCallerManager) CallPluginKeyByNameExWithAsync(forceSync bool, pluginId string, name string, itemsFuncs ...func() interface{}) {
	if y.table == nil {
		y.table = new(sync.Map)
		return
	}

	notified := new(sync.Map)
	_ = notified

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("call [%v] failed: %v", name, err)
			return
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

	call := func(i *Caller) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call failed: \n%v", err)
			}
		}()
		if (pluginId == "" /*执行所有该类型的插件*/) || (i.Id == pluginId /*执行当前插件*/) {
			var items []interface{}
			for _, i := range itemsFuncs {
				i := i
				items = append(items, i())
			}
			log.Debugf("call %v.%v(params...)", i.Id, name)
			i.Core.Handler(items...)
			return
		}
	}

	for _, iRaw := range ins {
		var verbose = iRaw.Verbose
		if iRaw.Id != verbose {
			verbose = fmt.Sprintf("%v[%v]", iRaw.Id, iRaw.Verbose)
		}
		//println(fmt.Sprintf("hook.Caller call [%v]'s %v", verbose, name))

		// 没有设置并发控制，就直接顺序执行
		if y.swg == nil || forceSync {
			log.Infof("Start Call Verbose: %v", verbose)
			call(iRaw)
			continue
		}

		// 设置了并发控制就这样
		i := iRaw
		go func() {
			defer func() {
				if err := recover(); err != nil {
					return
				}
			}()

			y.swg.Add()
			go func() {
				defer y.swg.Done()
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("panic from call[%v]: %v", verbose, err)
					}
				}()
				log.Infof("Start to Call Async Verbose: %v", verbose)
				call(i)
				log.Infof("Finished Calling Async Verbose: %v", verbose)
			}()
		}()
	}
}

func (y *YakToCallerManager) Wait() {
	if y.swg == nil {
		return
	}
	y.swg.Wait()
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

func (y *YakToCallerManager) LoadPlugin(t string, hooks ...string) error {
	return loadScript(y, t, hooks...)
}

func (y *YakToCallerManager) LoadPluginContext(ctx context.Context, t string, hooks ...string) error {
	return loadScriptCtx(y, ctx, t, hooks...)
}

func loadScript(mng *YakToCallerManager, scriptType string, hookNames ...string) error {
	return loadScriptCtx(mng, context.Background(), scriptType, hookNames...)
}

func loadScriptByName(mng *YakToCallerManager, scriptName string, hookNames ...string) error {
	return loadScriptByNameCtx(mng, context.Background(), scriptName, hookNames...)
}

var (
	currentCoreEngineMutext  = new(sync.Mutex)
	currentCoreEngine        yaklang.YaklangEngine
	haveSetCurrentCoreEngine bool
)

func setCurrentCoreEngine(e yaklang.YaklangEngine) {
	currentCoreEngineMutext.Lock()
	defer currentCoreEngineMutext.Unlock()

	if currentCoreEngine == nil {
		currentCoreEngine = e
	} else {
		haveSetCurrentCoreEngine = true
	}
}

func unsetCurrentCoreEngine(e yaklang.YaklangEngine) {
	currentCoreEngineMutext.Lock()
	defer currentCoreEngineMutext.Unlock()

	if currentCoreEngine == e {
		currentCoreEngine = nil
		haveSetCurrentCoreEngine = false
	}
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
	db = db.Model(&yakit.YakScript{}).Where("type = ?", scriptType)
	counter := 0
	for script := range yakit.YieldYakScripts(db, ctx) {
		counter++
		err := mng.AddForYakit(ctx, script.ScriptName, nil, script.Content, YakitCallerIf(func(result *ypb.ExecResult) error {
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
	db = db.Model(&yakit.YakScript{}).Where("script_name = ?", scriptName)
	counter := 0
	for script := range yakit.YieldYakScripts(db, ctx) {
		counter++
		err := mng.AddForYakit(ctx, script.ScriptName, nil, script.Content, YakitCallerIf(func(result *ypb.ExecResult) error {
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

var HooksExports = map[string]interface{}{
	"NewManager":              NewYakToCallerManager,
	"NewMixPluginCaller":      NewMixPluginCaller,
	"RemoveYakitPluginByName": removeScriptByNameCtx,
	"LoadYakitPluginContext":  loadScriptCtx,
	"LoadYakitPlugin":         loadScript,
	"LoadYakitPluginByName":   loadScriptByName,
	"CallYakitPluginFunc":     CallYakitPluginFunc,
}

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
		engineRoot.RegisterEngineHooks(func(engine yaklang.YaklangEngine) error {
			engine.SetVar("scriptName", script.ScriptName)
			engine.SetVar("param", utils.InterfaceToString(s))
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
