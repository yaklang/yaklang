package parser

func ExecuteWithStringHandler(code string, funcMap map[string]func(string2 string) []string) ([]string, error) {
	nodes := Parse(code)
	generator := NewGenerator(nodes, funcMap)
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
