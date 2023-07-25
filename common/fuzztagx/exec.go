package fuzztagx

func ExecuteWithStringHandler(source string, param map[string]func(string) []string) ([]string, error) {
	res, err := Parse(source)
	if err != nil {
		return nil, err
	}
	for _, r := range res {
		if v, ok := r.(*Tag); ok && !v.IsExpTag {
			for _, node := range v.Nodes {
				node1 := node.(*FuzzTagMethod)
				node1.ParseLabel()
				node1.funTable = param
			}
		}
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
