package fuzztagx

func ExecuteWithStringHandler(source string, param map[string]func(s string) []string) ([]string, error) {
	param1 := make(map[string]BuildInTagFun)
	for k, v := range param {
		param1[k] = v
	}
	dataCtx := &MethodContext{
		methodTable: param1,
		labelTable:  make(map[string][]*FuzzTagMethod),
	}
	res, err := Parse(source, dataCtx)
	if err != nil {
		return nil, err
	}
	generator := NewGenerator(res)
	result := []string{}
	for {
		s, ok := generator.Generate()
		if !ok {
			break
		}
		result = append(result, s)
	}
	return result, nil
}
