package yak

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
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

func MutateHookCaller(raw string) (func([]byte) []byte, func([]byte) []byte, func([]byte, []byte) map[string]string) {
	// 发送数据包之前的 hook
	entry := NewScriptEngine(2)
	entry.HookOsExit()
	var engine *antlr4yak.Engine
	var err error
	engine, err = entry.ExecuteEx(raw, make(map[string]interface{}))
	if err != nil {
		log.Errorf("eval hookCode failed: %s", err)
		return nil, nil, nil
	}

	_, beforeRequestOk := engine.GetVar("beforeRequest")
	_, afterRequestOk := engine.GetVar("afterRequest")
	_, mirrorFlowOK := engine.GetVar("mirrorHTTPFlow")

	var hookLock = new(sync.Mutex)

	var hookBefore func([]byte) []byte = nil
	var hookAfter func([]byte) []byte = nil
	var mirrorFlow func(req []byte, rsp []byte) map[string]string = nil

	if beforeRequestOk {
		hookBefore = func(bytes []byte) []byte {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("beforeRequest(ORIGIN) panic: %s", err)
				}
			}()
			if engine != nil {
				resultRequest, err := engine.CallYakFunction(context.Background(), "beforeRequest", []interface{}{bytes})
				if err != nil {
					log.Infof("eval beforeRequest hook failed: %s", err)
				}
				requestRawNew, typeOk := resultRequest.([]byte)
				if typeOk {
					return requestRawNew
				}
			}
			return bytes
		}
	}

	if afterRequestOk {
		hookAfter = func(bytes []byte) []byte {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("afterRequest(RESPONSE) panic: %s", err)
				}
			}()

			if engine != nil {
				resultResponse, err := engine.CallYakFunction(context.Background(), "afterRequest", []interface{}{bytes})
				if err != nil {
					log.Infof("eval afterRequest hook failed: %s", err)
				}
				responseNew, typeOk := resultResponse.([]byte)
				if typeOk {
					return responseNew
				}
			}
			return bytes
		}
	}

	if mirrorFlowOK {
		mirrorFlow = func(req []byte, rsp []byte) map[string]string {
			hookLock.Lock()
			defer hookLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("mirrorHTTPFlow(request, response) data panic: %s", err)
				}
			}()

			var result = make(map[string]string)
			if engine != nil {
				mirrorResult, err := engine.CallYakFunction(context.Background(), "mirrorHTTPFlow", []interface{}{req, rsp})
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

	return hookBefore, hookAfter, mirrorFlow
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
		var engine, err = entry.ExecuteEx(raw, make(map[string]interface{}))
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
	var engine, err = entry.ExecuteEx(raw, make(map[string]interface{}))
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
		//Regexp:  _codeMutateRegexp,
		Handle: func(db *gorm.DB, s string) (finalResult []string, finalError error) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("mutate.yaklang panic: %s", err)
					//finalResult = []string{s}
					//finalError = nil
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

			var result = strings.Split(s, "|")
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
