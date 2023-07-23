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
			}
		}
	}
	for i := 0; ; i++ {
		oneRes := ""
		ok := false
		for _, node := range res {
			d, err := node.GenerateOne(i)
			if err == nil {
				ok = true
			}
			oneRes += d
		}
		if !ok && i > 10*10000 {
			break
		}
	}
	return nil, nil
}
