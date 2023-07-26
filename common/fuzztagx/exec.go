package fuzztagx

func ExecuteWithStringHandler(source string, param map[string]BuildInTagFun) ([]string, error) {
	res, err := Parse(source, &param)
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
