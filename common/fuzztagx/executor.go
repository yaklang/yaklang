package fuzztagx

import (
	"errors"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
)

func executeWithStringHandler(code string, funcMap map[string]func(string2 string) []string, isSimple bool) ([]string, error) {
	nodes, err := ParseFuzztag(code, isSimple)
	if err != nil {
		return nil, err
	}
	fMap := map[string]*parser.TagMethod{}
	for k, v := range funcMap { // 转换成标准的TagMethod，旧版的TagMethod可以通过panic的方式传递错误信息
		k := k
		v := v
		fMap[k] = &parser.TagMethod{
			Fun: func(s string) (res []*parser.FuzzResult, err error) {
				defer func() {
					if r := recover(); r != nil {
						if v, ok := r.(error); ok {
							err = v
						} else {
							err = errors.New(utils.InterfaceToString(r))
						}
					}
				}()
				for _, v := range v(s) {
					res = append(res, parser.NewFuzzResultWithData(v))
				}
				return
			},
			Name:  k,
			IsDyn: false,
		}
	}
	generator := parser.NewGenerator(nodes, fMap)
	res := []string{}
	for generator.Next() {
		if generator.Error != nil {
			return nil, generator.Error
		}
		res = append(res, string(generator.Result().GetData()))
	}
	return res, nil
}
func ExecuteWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	return executeWithStringHandler(code, funcMap, false)
}
func ExecuteSimpleTagWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	return executeWithStringHandler(code, funcMap, true)
}
