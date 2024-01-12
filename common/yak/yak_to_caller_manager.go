package yak

import (
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

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
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
)

const HOOK_CLAER = "clear"

type YakFunctionCaller struct {
	Handler func(args ...interface{})
}

func Fuzz_WithHotPatch(ctx context.Context, code string) mutate.FuzzConfigOpt {
	if strings.TrimSpace(code) == "" {
		return mutate.Fuzz_WithExtraFuzzTagHandler("yak", func(s string) []string {
			return []string{s}
		})
	}
	engine := NewScriptEngine(1)
	codeEnv, err := engine.ExecuteExWithContext(ctx, code, make(map[string]interface{}))
	if err != nil {
		log.Errorf("load hotPatch code error: %s", err)
		return mutate.Fuzz_WithExtraFuzzTagHandler("yak", func(s string) []string {
			return []string{s}
		})
	}
	return mutate.Fuzz_WithExtraFuzzErrorTagHandler("yak", func(s string) (result []*parser.FuzzResult, err error) {
		var handle, params, _ = strings.Cut(s, "|")

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
			errorStr := spew.Sprintf("function %s not found", handle)
			log.Errorf("call hotPatch code error: %s", errorStr)
			return nil, errors.New(errorStr)
		}
		yakFunc, ok := yakVar.(*yakvm.Function)
		if !ok {
			errorStr := spew.Sprintf("function %s not found", handle)
			log.Errorf("call hotPatch code error: %s", errorStr)
			return nil, errors.New(errorStr)
		}
		iparams := []any{}
		if yakFunc.IsVariableParameter() {
			funk.ForEach(strings.Split(params, "|"), func(s any) {
				iparams = append(iparams, s)
			})

		} else {
			paramIn := yakFunc.GetNumIn()
			splits := strings.Split(params, "|")
			for len(splits) < paramIn {
				splits = append(splits, "")
			}
			i := 0
			for ; i < paramIn-1; i++ {
				iparams = append(iparams, splits[i])
			}

			iparams = append(iparams, strings.Join(splits[i:], "|"))
		}
		data, err := codeEnv.CallYakFunction(ctx, handle, iparams)
		if err != nil {
			errInfo := fmt.Sprintf("%s%s", fuzztag.YakHotPatchErr, err.Error())
			log.Errorf("call hotPatch code error: %s", err)
			return nil, errors.New(errInfo)
		}
		if data == nil {
			errInfo := fmt.Sprintf("%s%s", fuzztag.YakHotPatchErr, "return nil")
			log.Errorf("call hotPatch code error: %s", "return nil")
			return result, errors.New(errInfo)
		}
		res := utils.InterfaceToStringSlice(data)
		for _, item := range res {
			result = append(result, parser.NewFuzzResultWithData(item))
		}
		return result, nil
	})
}

func FetchFunctionFromSourceCode(ctx context.Context, pluginContext *YakitPluginContext, timeout time.Duration, id string, code string, hook func(e *antlr4yak.Engine) error, functionNames ...string) (map[string]*YakFunctionCaller, error) {
	var fTable = map[string]*YakFunctionCaller{}
	engine := NewScriptEngine(1) // 因为需要在 hook 里传回执行引擎, 所以这里不能并发
	engine.RegisterEngineHooks(hook)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		if id != "" {
			pluginContext.PluginName = id
		}
		BindYakitPluginContextToEngine(engine, pluginContext)
		HookEngineContext(engine, ctx)
		return nil
	})
	engine.HookOsExit()
	//timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	//defer func() { cancel() }()
	ins, err := engine.ExecuteExWithContext(ctx, code, map[string]interface{}{
		"ROOT_CONTEXT": ctx,
		"YAK_FILENAME": id,
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

		nIns := ins
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
	return fTable, nil

}

type Caller struct {
	Core    *YakFunctionCaller
	Hash    string
	Id      string
	Verbose string
	Engine  *antlr4yak.Engine
	// NativeFunction *exec.Function
}

type YakToCallerManager struct {
	table          *sync.Map
	swg            *utils.SizedWaitGroup
	baseWaitGroup  *sync.WaitGroup
	dividedContext bool
	timeout        time.Duration
	runtimeId      string
	proxy          string
}

func (c *YakToCallerManager) SetLoadPluginTimeout(i float64) {
	c.timeout = time.Duration(i * float64(time.Second))
}
func (y *YakToCallerManager) SetDividedContext(b bool) {
	y.dividedContext = b
}

func NewYakToCallerManager() *YakToCallerManager {
	return &YakToCallerManager{table: new(sync.Map), baseWaitGroup: new(sync.WaitGroup), timeout: 10 * time.Second}
}

func (m *YakToCallerManager) SetConcurrent(i int) error {
	if m.swg != nil {
		err := utils.Error("cannot set swg for YakToCallerManager: existed swg")
		log.Error(err)
		return err
	}
	swg := utils.NewSizedWaitGroup(i)
	m.swg = swg
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
	return y.Set(ctx, code, func(engine *antlr4yak.Engine) error {
		engine.SetVar("yakit_output", FeedbackFactory(db, caller, false, "default"))
		engine.SetVar("yakit_save", FeedbackFactory(db, caller, true, "default"))
		engine.SetVar("yakit_status", func(id string, i interface{}) {
			FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
				Id:   id,
				Data: fmt.Sprint(i),
			})
		})
		engine.ImportSubLibs("yakit", yaklib.GetExtYakitLibByOutput(func(d any) error {
			level, data := yaklib.MarshalYakitOutput(d)
			yakitLog := &yaklib.YakitLog{
				Level:     level,
				Data:      data,
				Timestamp: time.Now().Unix(),
			}
			raw, err := yaklib.YakitMessageGenerator(yakitLog)
			if err != nil {
				return err
			}

			result := &ypb.ExecResult{
				IsMessage: true,
				Message:   raw,
			}
			return caller(result)
		}))
		return nil
	}, hooks...)
}

func (y *YakToCallerManager) Set(ctx context.Context, code string, hook func(engine *antlr4yak.Engine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	var engine *antlr4yak.Engine
	cTable, err := FetchFunctionFromSourceCode(ctx, &YakitPluginContext{
		RuntimeId: y.runtimeId,
		Proxy:     y.proxy,
	}, y.timeout, "", code, func(e *antlr4yak.Engine) error {
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

type YakitPluginContext struct {
	PluginName string
	RuntimeId  string
	Proxy      string
}

func BindYakitPluginContextToEngine(nIns *antlr4yak.Engine, pluginContext *YakitPluginContext) {
	if nIns == nil {
		return
	}
	var pluginName string
	var runtimeId string
	var proxy string
	if pluginContext == nil {
		return
	}

	runtimeId = pluginContext.RuntimeId
	pluginName = pluginContext.PluginName
	proxy = pluginContext.Proxy
	// inject meta vars
	for _, method := range []string{
		"Get", "Post",
	} {
		nIns.GetVM().RegisterMapMemberCallHandler("http", method, func(i interface{}) interface{} {
			if proxy == "" {
				return i
			}
			originFunc, ok := i.(func(u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpResponse, error))
			if ok {
				return func(u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpResponse, error) {
					opts = append(opts, yakhttp.YakHttpConfig_Proxy(proxy))
					return originFunc(u, opts...)
				}
			}
			return i
		})
	}
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

	nIns.GetVM().RegisterMapMemberCallHandler("http", "Request", func(i interface{}) interface{} {
		if proxy == "" {
			return i
		}
		originFunc, ok := i.(func(method, u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpResponse, error))
		if ok {
			return func(method, u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpResponse, error) {
				opts = append(opts, yakhttp.YakHttpConfig_Proxy(proxy))
				return originFunc(method, u, opts...)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("http", "NewRequest", func(i interface{}) interface{} {
		if proxy == "" {
			return i
		}
		originFunc, ok := i.(func(method, u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpRequest, error))
		if ok {
			return func(method, u string, opts ...yakhttp.HttpOption) (*yakhttp.YakHttpRequest, error) {
				opts = append(opts, yakhttp.YakHttpConfig_Proxy(proxy))
				return originFunc(method, u, opts...)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("poc", "HTTP", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...poc.PocConfig) ([]byte, []byte, error))
		if ok {
			return func(raw interface{}, opts ...poc.PocConfig) ([]byte, []byte, error) {
				opts = append(opts, poc.PoCOptWithSource(pluginName))
				opts = append(opts, poc.PoCOptWithFromPlugin(pluginName))
				opts = append(opts, poc.PoCOptWithRuntimeId(runtimeId))
				opts = append(opts, poc.PoCOptWithSaveHTTPFlow(true))
				opts = append(opts, poc.PoCOptWithProxy(proxy))
				return originFunc(raw, opts...)
			}
		}
		log.Errorf("BUG: poc.HTTP 's signature is override")
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("poc", "HTTPEx", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error))
		if ok {
			return func(raw interface{}, opts ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
				opts = append(opts, poc.PoCOptWithSource(pluginName))
				opts = append(opts, poc.PoCOptWithFromPlugin(pluginName))
				opts = append(opts, poc.PoCOptWithRuntimeId(runtimeId))
				opts = append(opts, poc.PoCOptWithSaveHTTPFlow(true))
				opts = append(opts, poc.PoCOptWithProxy(proxy))
				return originFunc(raw, opts...)
			}
		}
		log.Errorf("BUG: poc.HTTPEx 's signature is override")
		return i
	})
	for _, method := range []string{"Get", "Post", "Head", "Delete", "Options"} {
		method := method
		nIns.GetVM().RegisterMapMemberCallHandler("poc", method, func(i interface{}) interface{} {
			origin, ok := i.(func(string, ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error))
			if !ok {
				log.Errorf("BUG: poc.%v 's signature is override", method)
				return i
			}
			return func(u string, opts ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
				opts = append(opts, poc.PoCOptWithSource(pluginName))
				opts = append(opts, poc.PoCOptWithFromPlugin(pluginName))
				opts = append(opts, poc.PoCOptWithRuntimeId(runtimeId))
				opts = append(opts, poc.PoCOptWithSaveHTTPFlow(true))
				opts = append(opts, poc.PoCOptWithProxy(proxy))
				return origin(u, opts...)
			}
		})
	}
	nIns.GetVM().RegisterMapMemberCallHandler("poc", "Do", func(i interface{}) interface{} {
		origin, ok := i.(func(method string, url string, opt ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error))
		if ok {
			return func(method string, url string, opts ...poc.PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
				opts = append(opts, poc.PoCOptWithSource(pluginName))
				opts = append(opts, poc.PoCOptWithFromPlugin(pluginName))
				opts = append(opts, poc.PoCOptWithRuntimeId(runtimeId))
				opts = append(opts, poc.PoCOptWithSaveHTTPFlow(true))
				opts = append(opts, poc.PoCOptWithProxy(proxy))
				return origin(method, url, opts...)
			}
		}
		log.Errorf("BUG: poc.Do 's signature is override")
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("nuclei", "Scan", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target any, opts ...any) (chan *tools.PocVul, error))
		if ok {
			return func(target any, opts ...any) (chan *tools.PocVul, error) {
				if runtimeId != "" {
					opts = append(opts, lowhttp.WithRuntimeId(runtimeId))
				}
				opts = append(opts, lowhttp.WithFromPlugin(pluginName))
				opts = append(opts, lowhttp.WithSaveHTTPFlow(true))
				opts = append(opts, lowhttp.WithProxy(proxy))
				return originFunc(target, opts...)
			}
		}
		return i
	})

	// `

	nIns.GetVM().RegisterMapMemberCallHandler("nuclei", "ScanAuto", func(i interface{}) interface{} {
		originFunc, ok := i.(func(target any, opts ...any))
		if ok {
			return func(target any, opts ...any) {
				opts = append(opts, lowhttp.WithRuntimeId(runtimeId))
				opts = append(opts, lowhttp.WithFromPlugin(pluginName))
				opts = append(opts, lowhttp.WithSaveHTTPFlow(true))
				opts = append(opts, lowhttp.WithProxy(proxy))
				originFunc(target, opts...)
			}
		}
		return i
	})
	nIns.GetVM().RegisterMapMemberCallHandler("fuzz", "HTTPRequest", func(i interface{}) interface{} {
		originFunc, ok := i.(func(interface{}, ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error))
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error) {
				opts = append(opts, mutate.OptSource(pluginName))
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
				opts = append(opts, mutate.OptSource(pluginName))
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
				opts = append(opts, yakit.WithRiskParam_YakitPluginName(pluginName))
				if runtimeId != "" {
					opts = append(opts, yakit.WithRiskParam_RuntimeId(runtimeId))
				}
				originFunc(target, opts...)
			}
		}
		return i
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
				manager.SetProxy(proxy)
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
				return manager, nil
			}
		}
		return i
	})
}

func HookEngineContext(engine *antlr4yak.Engine, streamContext context.Context) {
	engine.GetVM().RegisterMapMemberCallHandler("context", "Seconds", func(f interface{}) interface{} {
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

	// hook new background context
	newContextHook := func(f interface{}) interface{} {
		return func() context.Context {
			return streamContext
		}
	}
	engine.GetVM().RegisterMapMemberCallHandler("context", "New", newContextHook)
	engine.GetVM().RegisterMapMemberCallHandler("context", "Background", newContextHook)

	// hook httpserver context
	engine.GetVM().RegisterMapMemberCallHandler("httpserver", "Serve", func(f interface{}) interface{} {
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
	engine.GetVM().RegisterMapMemberCallHandler("traceroute", "Diagnostic", func(f interface{}) interface{} {
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
	engine.GetVM().RegisterMapMemberCallHandler("udp", "Serve", func(f interface{}) interface{} {
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
	engine.GetVM().RegisterMapMemberCallHandler("tcp", "Serve", func(f interface{}) interface{} {
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
	engine.GetVM().RegisterMapMemberCallHandler("mitm", "Start", func(f interface{}) interface{} {
		originFunc, ok := f.(func(port int, opts ...yaklib.MitmConfigOpt) error)
		if ok {
			return func(port int, opts ...yaklib.MitmConfigOpt) error {
				opts = append([]yaklib.MitmConfigOpt{yaklib.MitmConfigContext(streamContext)}, opts...)
				return originFunc(port, opts...)
			}
		}
		return f
	})
	engine.GetVM().RegisterMapMemberCallHandler("mitm", "Bridge", func(f interface{}) interface{} {
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
		engine.GetVM().RegisterMapMemberCallHandler("git", funcName, hookGitFunc)
	}

	//hook fuzz context
	engine.GetVM().RegisterMapMemberCallHandler("fuzz", "HTTPRequest", func(f interface{}) interface{} {
		originFunc, ok := f.(func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error))
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) (*mutate.FuzzHTTPRequest, error) {
				opts = append([]mutate.BuildFuzzHTTPRequestOption{mutate.OptContext(streamContext)}, opts...)
				return originFunc(i, opts...)
			}
		}
		return f
	})
	engine.GetVM().RegisterMapMemberCallHandler("fuzz", "MustHTTPRequest", func(f interface{}) interface{} {
		originFunc, ok := f.(func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest)
		if ok {
			return func(i interface{}, opts ...mutate.BuildFuzzHTTPRequestOption) *mutate.FuzzHTTPRequest {
				opts = append([]mutate.BuildFuzzHTTPRequestOption{mutate.OptContext(streamContext)}, opts...)
				return originFunc(i, opts...)
			}
		}
		return f
	})

	// hook http_pool context
	engine.GetVM().RegisterMapMemberCallHandler("httpool", "Pool", func(f interface{}) interface{} {
		originFunc, ok := f.(func(i interface{}, opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error))
		if ok {
			return func(i interface{}, opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error) {
				opts = append([]mutate.HttpPoolConfigOption{mutate.WithPoolOpt_Context(streamContext)}, opts...)
				return originFunc(i, opts...)
			}
		}
		return f
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
	return y.Add(ctx, id, params, code, func(engine *antlr4yak.Engine) error {
		engine.SetVar("RUNTIME_ID", y.runtimeId)
		engine.SetVar("YAKIT_PLUGIN_ID", id)
		engine.SetVar("yakit_output", FeedbackFactory(db, caller, false, id))
		engine.SetVar("yakit_save", FeedbackFactory(db, caller, true, id))
		engine.SetVar("yakit_status", func(id string, i interface{}) {
			FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
				Id:   id,
				Data: fmt.Sprint(i),
			})
		})
		engine.ImportSubLibs("yakit", yaklib.GetExtYakitLibByOutput(func(d any) error {
			level, data := yaklib.MarshalYakitOutput(d)
			yakitLog := &yaklib.YakitLog{
				Level:     level,
				Data:      data,
				Timestamp: time.Now().Unix(),
			}
			raw, err := yaklib.YakitMessageGenerator(yakitLog)
			if err != nil {
				return err
			}

			result := &ypb.ExecResult{
				IsMessage: true,
				Message:   raw,
			}
			return caller(result)
		}))
		BindYakitPluginContextToEngine(engine, &YakitPluginContext{
			PluginName: id,
			RuntimeId:  y.runtimeId,
			Proxy:      y.proxy,
		})
		return nil
	}, hooks...)
}

func (y *YakToCallerManager) Add(ctx context.Context, id string, params []*ypb.ExecParamItem, code string, hook func(*antlr4yak.Engine) error, funcName ...string) (retError error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("load caller failed: %v", err)
			retError = utils.Errorf("load caller error: %v", err)
			return
		}
	}()

	var engine *antlr4yak.Engine

	if _, ok := ctx.Value("ctx_info").(map[string]any)["isNaslScript"]; ok {
		if v, ok := y.table.Load(HOOK_LoadNaslScriptByNameFunc); ok {
			v.(func(string))(id)
			return nil
		}
	}
	cTable, err := FetchFunctionFromSourceCode(ctx, &YakitPluginContext{
		RuntimeId: y.runtimeId,
		Proxy:     y.proxy,
	}, y.timeout, id, code, func(e *antlr4yak.Engine) error {
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
	// 对于nasl插件还需要提取加载函数
	if _, ok := ctx.Value("ctx_info").(map[string]any)["isNaslScript"]; ok {
		f := func(name string) {
			if !strings.HasSuffix(strings.ToLower(name), ".nasl") {
				log.Errorf("call [%v] yakvm native function failed: %s", HOOK_LoadNaslScriptByNameFunc, "nasl script name must end with .nasl")
				return
			}
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("call [%v] yakvm native function failed: %s", HOOK_LoadNaslScriptByNameFunc, err)
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

func (y *YakToCallerManager) ShouldCallByName(name string) bool {
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
	y.baseWaitGroup.Add(1)
	defer func() {
		y.baseWaitGroup.Done()
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
			log.Debugf("Start Call Plugin: %v", verbose)
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
				if verbose != "" {
					log.Debugf("Start to Call Async Verbose: %v", verbose)
				}
				call(i)
				if verbose != "" {
					log.Debugf("Finished Calling Async Verbose: %v", verbose)
				}
			}()
		}()
	}
}

func (y *YakToCallerManager) Wait() {
	if y.swg == nil {
		return
	}

	var count = 0
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
		engineRoot.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
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
