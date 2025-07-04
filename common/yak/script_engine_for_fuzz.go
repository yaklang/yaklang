package yak

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	yaklangspec "github.com/yaklang/yaklang/common/yak/yaklang/spec"
)

var _codeMutateRegexp = regexp.MustCompile(`(?s){{yak\d*(\(.*\))}}`)

func MutateHookCaller(ctx context.Context, raw string, caller YakitCallerIf, params ...*ypb.ExecParamItem) (
	func(https bool, originReq []byte, req []byte) []byte,
	func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte,
	func([]byte, []byte, map[string]string) map[string]string,
	func(bool, int, []byte, []byte, func(...[]byte)),
	func(bool, []byte, []byte, func(string)),
) {
	// 发送数据包之前的 hook
	scriptEngine := NewScriptEngine(2)
	var engine *antlr4yak.Engine
	var err error

	yakitContext := CreateYakitPluginContext("").WithContext(ctx)
	if caller != nil {
		client := yaklib.NewVirtualYakitClient(caller)
		db := consts.GetGormProjectDatabase()
		scriptEngine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
			engine.OverrideRuntimeGlobalVariables(map[string]any{
				"yakit_output": FeedbackFactory(db, caller, false, "default"),
				"yakit_save":   FeedbackFactory(db, caller, true, "default"),
				"yakit_status": func(id string, i interface{}) {
					FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
						Id: id, Data: fmt.Sprint(i),
					})
				},
				"yakit": yaklib.GetExtYakitLibByClient(client),
			})
			return nil
		})
		yakitContext = yakitContext.WithYakitClient(client)
	}

	if len(params) > 0 {
		args := []string{}
		for _, param := range params {
			args = append(args, "--"+param.GetKey(), fmt.Sprintf("%s", param.GetValue()))
		}
		app := GetHookCliApp(args)
		yakitContext.WithCliApp(app)
	}

	scriptEngine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		BindYakitPluginContextToEngine(engine, yakitContext)
		return nil
	})

	engine, err = scriptEngine.ExecuteEx(raw, make(map[string]interface{}))
	if err != nil {
		log.Errorf("eval hookCode failed: %s", err)
		return nil, nil, nil, nil, nil
	}

	before, beforeRequestOk := engine.GetVar("beforeRequest")
	after, afterRequestOk := engine.GetVar("afterRequest")

	var legacyBeforeRequest bool
	beforeFunction, ok := before.(*yakvm.Function)
	if !ok {
		beforeRequestOk = false
	} else {
		legacyBeforeRequest = beforeFunction.GetNumIn() == 1
	}

	var legacyAfterRequest bool
	afterFunction, ok := after.(*yakvm.Function)
	if !ok {
		afterRequestOk = false
	} else {
		legacyAfterRequest = afterFunction.GetNumIn() == 1
	}

	mirrorHandlerInstance, mirrorFlowOK := engine.GetVar("mirrorHTTPFlow")
	mirrorHTTPFlowNumIn := 2
	if ret, ok := mirrorHandlerInstance.(*yakvm.Function); ok {
		mirrorHTTPFlowNumIn = ret.GetNumIn()
	}

	retryHandlerInstance, retryHandlerOk := engine.GetVar("retryHandler")
	retryHandlerInstanceNumIn := 3
	if ret, ok := retryHandlerInstance.(*yakvm.Function); ok {
		retryHandlerInstanceNumIn = ret.GetNumIn()
	}

	customFailureCheckerInstance, customFailureCheckerOk := engine.GetVar("customFailureChecker")
	customFailureCheckerInstanceNumIn := 4
	if ret, ok := customFailureCheckerInstance.(*yakvm.Function); ok {
		customFailureCheckerInstanceNumIn = ret.GetNumIn()
	}
	hookLock := new(sync.Mutex)

	var hookBefore func(https bool, originReq []byte, req []byte) []byte = nil
	var hookAfter func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte = nil
	var mirrorFlow func(req []byte, rsp []byte, handle map[string]string) map[string]string = nil
	var retryHandler func(https bool, retryCount int, req []byte, rsp []byte, retryFunc func(...[]byte)) = nil
	var customFailureChecker func(https bool, req []byte, rsp []byte, fail func(string)) = nil

	if beforeRequestOk {
		hookBefore = func(https bool, originReq []byte, req []byte) []byte {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("beforeRequest(ORIGIN) panic: %s", err)
				}
			}()
			if engine != nil {
				var resultRequest any
				if legacyBeforeRequest {
					resultRequest, err = engine.CallYakFunction(context.Background(), "beforeRequest", []interface{}{req})
				} else {
					resultRequest, err = engine.CallYakFunction(context.Background(), "beforeRequest", []interface{}{https, originReq, req})
				}

				if err != nil {
					log.Infof("eval beforeRequest hook failed: %s", err)
				}
				switch ret := resultRequest.(type) {
				case string:
					return []byte(ret)
				case []byte:
					return ret
				case []rune:
					return []byte(string(ret))
				}
			}
			return req
		}
	}

	if afterRequestOk {
		hookAfter = func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("afterRequest(RESPONSE) panic: %s", err)
				}
			}()

			if engine != nil {

				var resultResponse any
				if legacyAfterRequest {
					resultResponse, err = engine.CallYakFunction(context.Background(), "afterRequest", []interface{}{rsp})
				} else {
					resultResponse, err = engine.CallYakFunction(context.Background(), "afterRequest", []interface{}{https, originReq, req, originRsp, rsp})
				}
				if err != nil {
					log.Infof("eval afterRequest hook failed: %s", err)
				}
				switch ret := resultResponse.(type) {
				case string:
					return []byte(ret)
				case []byte:
					return ret
				case []rune:
					return []byte(string(ret))
				}
			}
			return rsp
		}
	}

	if mirrorFlowOK {
		mirrorFlow = func(req []byte, rsp []byte, existed map[string]string) map[string]string {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("mirrorHTTPFlow(request, response) data panic: %s", err)
				}
			}()

			result := make(map[string]string)
			if engine != nil {
				params := []any{req, rsp}
				if mirrorHTTPFlowNumIn > 2 {
					params = []any{req, rsp, existed}
				}
				mirrorResult, err := engine.CallYakFunction(context.Background(), "mirrorHTTPFlow", params)
				if err != nil {
					log.Infof("eval afterRequest hook failed: %s", err)
				}

				if ret := utils.InterfaceToMap(mirrorResult); ret != nil {
					for k, v := range ret {
						result[k] = strings.Join(v, ",")
					}
				}
			}
			return result
		}
	}

	if retryHandlerOk {
		retryHandler = func(https bool, retryCount int, req []byte, rsp []byte, retryFunc func(...[]byte)) {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("retryHandler(request, response) data panic: %s", err)
				}
			}()

			if engine != nil {
				params := []any{https, retryCount, req, rsp, retryFunc}
				if retryHandlerInstanceNumIn == 4 {
					params = []any{retryCount, req, rsp, retryFunc}
				} else if retryHandlerInstanceNumIn == 3 {
					params = []any{req, rsp, retryFunc}
				}
				_, err := engine.CallYakFunction(context.Background(), "retryHandler", params)
				if err != nil {
					log.Infof("eval retryHandler hook failed: %s", err)
				}
			}
			return
		}
	}

	if customFailureCheckerOk {
		customFailureChecker = func(https bool, req []byte, rsp []byte, fail func(string)) {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("customFailureChecker(request, response) data panic: %s", err)
				}
			}()

			if engine != nil {
				params := []any{https, req, rsp, fail}
				if customFailureCheckerInstanceNumIn == 3 {
					params = []any{req, rsp, fail}
				} else if customFailureCheckerInstanceNumIn == 2 {
					params = []any{rsp, fail}
				}
				_, err := engine.CallYakFunction(context.Background(), "customFailureChecker", params)
				if err != nil {
					log.Infof("eval customFailureChecker hook failed: %s", err)
				}
			}
		}
	}
	return hookBefore, hookAfter, mirrorFlow, retryHandler, customFailureChecker
}

func MutateWithParamsGetter(raw string) func() *mutate.RegexpMutateCondition {
	return func() (finalCondition *mutate.RegexpMutateCondition) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("__getParams__() panic: %s", err)
				finalCondition = nil
			}
		}()

		if raw == "" {
			return nil
		}
		entry := NewScriptEngine(1)
		entry.HookOsExit()
		engine, err := entry.ExecuteEx(raw, make(map[string]interface{}))
		if err != nil {
			log.Errorf("execute yak hook failed: %s", err)
			return nil
		}
		err = engine.SafeEval(context.Background(), `result = __getParams__()`)
		if err != nil {
			log.Errorf("eval __getParams__() failed: %s", err)
		}
		i, ok := engine.GetVar("result")
		if !ok {
			return nil
		}
		return mutate.MutateWithExtraParams(utils.InterfaceToMap(i))
	}
}

func MutateWithYaklang(raw string) *mutate.RegexpMutateCondition {
	entry := NewScriptEngine(10)
	entry.HookOsExit()
	engine, err := entry.ExecuteEx(raw, make(map[string]interface{}))
	if err != nil {
		log.Errorf("load yak hook failed: %s", err)
		engine = nil
	}
	if len(raw) > 0 {
		log.Infof("create YAK_HOOK rule[clen:%v]", len(raw))
	}

	return &mutate.RegexpMutateCondition{
		Verbose: "YAK_HOOK",
		TagName: "yak",
		// Regexp:  _codeMutateRegexp,
		Handle: func(db *gorm.DB, s string) (finalResult []string, finalError error) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("mutate.yaklang panic: %s", err)
					// finalResult = []string{s}
					// finalError = nil
					finalError = fmt.Errorf("mutate.yaklang panic: %s", err)
					finalResult = nil
				}
			}()
			if engine == nil {
				panic("load yak hook failed")
			}
			if raw == "" {
				return []string{s}, nil
			}

			result := strings.Split(s, "|")
			if len(result) <= 0 {
				return []string{s}, nil
			}

			var funcName string
			var params []string
			if len(result) == 0 {
				panic("mutate.yaklang panic: not found function name")
			}
			if len(result) == 1 {
				funcName = result[0]
			} else {
				funcName = result[0]
				params = result[1:]
			}
			log.Infof("start to call funcName: %v in {{yak(...)}}", funcName)
			iparam := []interface{}{}
			for _, param := range params {
				iparam = append(iparam, param)
			}
			res, err := engine.CallYakFunction(context.Background(), funcName, iparam)
			if err != nil {
				panic(err)
			}
			if res == yaklangspec.Undefined {
				return []string{""}, nil
			} else {
				refV := reflect.ValueOf(res)
				if refV.Type().Kind() == reflect.Slice || refV.Type().Kind() == reflect.Array {
					for i := 0; i < refV.Len(); i++ {
						finalResult = append(finalResult, utils.InterfaceToString(refV.Index(i).Interface()))
					}
					return finalResult, nil
				} else {
					return []string{utils.InterfaceToString(res)}, nil
				}
			}
		},
	}
}
