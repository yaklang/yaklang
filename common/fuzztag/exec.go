package fuzztag

import (
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type FuzzExecResult struct {
	data     []byte
	showInfo []string
}

func NewFuzzExecResult(data []byte, showInfo []string) *FuzzExecResult {
	return &FuzzExecResult{
		data:     data,
		showInfo: showInfo,
	}
}
func ExecuteWithHandler(i interface{}, m map[string]func([]byte) [][]byte) ([]string, error) {
	return ExecuteWithHandlerWithCallback(i, m, nil)
}

func ExecuteWithHandlerWithCallback(i interface{}, m map[string]func([]byte) [][]byte, cb func([]byte, [][]byte) bool) (res []string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	lex := NewFuzzTagLexer(i)
	//for _, t := range lex.tokens {
	//	println(t.Verbose)
	//}
	//lex.ShowTokens()
	ast, err := ParseToFuzzTagAST(lex)
	if err != nil {
		return nil, err
	}

	if m == nil {
		m = make(map[string]func([]byte) [][]byte)
	}
	results, err := ast.ExecuteWithCallBack(m, cb)
	if err != nil {
		return nil, err
	}
	var finalResults = make([]string, len(results))
	for index, data := range results {
		finalResults[index] = string(data)
	}
	return finalResults, nil
}

func ExecuteWithStringHandler(i interface{},
	mStr map[string]func(string) []string,
) ([]string, error) {
	return ExecuteWithStringHandlerWithCallback(i, mStr, nil)
}

func ExecuteWithStringHandlerWithCallback(
	i interface{},
	mStr map[string]func(string) []string,
	cb func(string, []string) bool,
) ([]string, error) {
	return ExecuteWithStringHandlerWithCallbackEx(i, mStr, nil, cb)
}
func ExecuteWithStringHandlerWithCallbackEx(
	i interface{},
	mStr map[string]func(string) []string,
	mStrEx map[string]func(string) []*FuzzExecResult,
	cb func(string, []string) bool,
) ([]string, error) {
	var m map[string]func([]byte) [][]byte
	var payloadVerbose = make(map[string][]string)
	if mStrEx != nil {
		for name, fun := range mStrEx {
			mStr[name] = func(fun func(string) []*FuzzExecResult) func(s string) []string {
				return func(s string) []string {
					var results = fun(s)
					var finalResults = make([]string, len(results))
					for index, data := range results {
						finalResults[index] = string(data.data)
						payloadVerbose[string(data.data)] = data.showInfo
					}
					return finalResults
				}
			}(fun)
		}
	}
	if mStr != nil {
		m = make(map[string]func([]byte) [][]byte, len(mStr))
		for k, v := range mStr {
			v := v
			m[k] = func(bytes []byte) [][]byte {
				if v == nil {
					return [][]byte{bytes}
				}

				var results = v(string(bytes))
				if results == nil {
					return [][]byte{{}}
				}
				//var finalResult = make([][]byte, len(results))
				//for i, f := range results {
				//	finalResult[i] = []byte(f)
				//}
				return funk.Map(results, func(i string) []byte {
					return []byte(i)
				}).([][]byte)
			}
		}
	}

	if cb == nil {
		return ExecuteWithHandler(i, m)
	}
	return ExecuteWithHandlerWithCallback(i, m, func(bytes []byte, i [][]byte) bool {
		var results = make([]string, len(i))
		for index, d := range i {
			if payloadVerbose != nil {
				if v, ok := payloadVerbose[string(d)]; ok {
					//如果在payloadVerbose 去到空，则把实际的输出值展示给用户
					if info := strings.Join(v, ","); info == "" {
						results[index] = string(d)
						continue
					}
					results[index] = fmt.Sprintf("[%s]", strings.Join(v, ","))
					continue
				}
			}
			results[index] = string(d)
		}

		return cb(string(bytes), results)
	})
}
