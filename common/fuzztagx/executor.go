package fuzztagx

import "github.com/yaklang/yaklang/common/fuzztagx/standard-parser"

func ExecuteWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	nodes, err := ParseFuzztag(code)
	if err != nil {
		return nil, err
	}
	fMap := map[string]standard_parser.TagMethod{}
	for k, v := range funcMap { // 转换成标准的TagMethod，旧版的TagMethod可以通过panic的方式传递错误信息
		k := k
		v := v
		fMap[k] = func(s string) (res []standard_parser.FuzzResult, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = r.(error)
				}
			}()
			for _, v := range v(s) {
				res = append(res, standard_parser.FuzzResult(v))
			}
			return
		}
	}
	generator := standard_parser.NewGenerator(nodes, fMap)
	generator.IgnoreError = true
	res := []string{}
	for {
		ok, err := generator.Generate()
		if err != nil {
			return nil, err
		}
		if ok {
			res = append(res, string(generator.Result()))
		} else {
			break
		}
	}
	return res, nil
}
