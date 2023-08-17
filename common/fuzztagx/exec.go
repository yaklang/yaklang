package fuzztagx

type MethodContext struct {
	methodTable map[string]BuildInTagFun
	labelTable  map[string][]*FuzzTagMethod
}

func ExecuteWithStringHandler(source string, param map[string]BuildInTagFun) ([]string, error) {
	dataCtx := &MethodContext{
		methodTable: param,
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
