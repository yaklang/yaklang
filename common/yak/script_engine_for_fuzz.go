package yak

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mutate"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklang"
	yaklangspec "yaklang.io/yaklang/common/yak/yaklang/spec"
)

var _codeMutateRegexp = regexp.MustCompile(`(?s){{yak\d*(\(.*\))}}`)

func MutateHookCaller(raw string) (func([]byte) []byte, func([]byte) []byte) {
	// 发送数据包之前的 hook
	entry := NewScriptEngine(2)
	entry.HookOsExit()
	var engine yaklang.YaklangEngine
	var err error
	engine, err = entry.ExecuteEx(raw, make(map[string]interface{}))
	if err != nil {
		log.Errorf("eval hookCode failed: %s", err)
	}

	_, beforeRequestOk := engine.GetVar("beforeRequest")
	_, afterRequestOk := engine.GetVar("afterRequest")

	var hookLock = new(sync.Mutex)

	var hookBefore func([]byte) []byte = nil
	var hookAfter func([]byte) []byte = nil

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
				engine.SetVar("ORIGIN", bytes)
				engine.SetVar("result", nil)
				err := engine.SafeEval(context.Background(), ";result = beforeRequest(ORIGIN);")
				if err != nil {
					log.Infof("eval beforeRequest hook failed: %s", err)
				}
				resultRequest, ok := engine.GetVar("result")
				if ok {
					requestRawNew, typeOk := resultRequest.([]byte)
					if typeOk {
						return requestRawNew
					}
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
				engine.SetVar("RESPONSE", bytes)
				engine.SetVar("result", nil)
				err := engine.SafeEval(context.Background(), ";result = afterRequest(RESPONSE);")
				if err != nil {
					log.Infof("eval afterRequest hook failed: %s", err)
				}
				resultResponse, ok := engine.GetVar("result")
				if ok {
					responseNew, typeOk := resultResponse.([]byte)
					if typeOk {
						return responseNew
					}
				}
			}
			return bytes
		}
	}
	return hookBefore, hookAfter
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
	log.Infof("create YAK_HOOK rule[clen:%v]", len(raw))

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
