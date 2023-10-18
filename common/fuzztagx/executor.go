package fuzztagx

import (
	"errors"
	"github.com/yaklang/yaklang/common/fuzztagx/standard-parser"
	"github.com/yaklang/yaklang/common/utils"
)

func ExecuteWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	nodes, err := ParseFuzztag(code)
	if err != nil {
		return nil, err
	}
	fMap := []*standard_parser.TagMethod{}
	for k, v := range funcMap { // 转换成标准的TagMethod，旧版的TagMethod可以通过panic的方式传递错误信息
		k := k
		v := v
		fMap = append(fMap, &standard_parser.TagMethod{
			Fun: func(s string) (res []*standard_parser.FuzzResult, err error) {
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
					res = append(res, standard_parser.NewFuzzResultWithData(v))
				}
				return
			},
			Name:  k,
			IsDyn: false,
		})
	}
	generator := standard_parser.NewGenerator(nodes, fMap)
	res := []string{}
	for generator.Next() {
		if generator.Error != nil {
			return nil, generator.Error
		}
		res = append(res, string(generator.Result().GetData()))
	}
	return res, nil
}
