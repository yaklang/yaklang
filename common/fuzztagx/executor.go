package fuzztagx

import "github.com/yaklang/yaklang/common/fuzztagx/standard-parser"

func ExecuteWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	nodes := standard_parser.Parse(code)
	generator := standard_parser.NewGenerator(nodes, funcMap)
	res := []string{}
	for {
		if v, ok := generator.Generate(); ok {
			res = append(res, v)
		} else {
			break
		}
	}
	return res, nil
}
