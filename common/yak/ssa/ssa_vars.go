package ssa

// func (f *Function) GetAllSymbols() map[string]Values {
// 	ret := make(map[string]Values, 0)
// 	tmp := make(map[string]map[Value]struct{})

// 	f.EachScope(func(s *Scope) {
// 		for name, variables := range s.VarMap {
// 			retList, ok := ret[name]
// 			if !ok {
// 				retList = make(Values, 0, 1)
// 			}
// 			tmpList, ok := tmp[name]
// 			if !ok {
// 				tmpList = make(map[Value]struct{})
// 			}
// 			for _, variable := range variables {
// 				if _, ok := tmpList[variable.value]; !ok {
// 					retList = append(retList, variable.value)
// 				}
// 			}
// 			ret[name] = retList
// 		}
// 	})

// 	return ret
// }

// func (f *Function) GetValuesByName(name string) InstructionNodes {
// 	ret := make([]InstructionNode, 0)
// 	tmp := make(map[Instruction]struct{})

// 	f.EachScope(func(s *Scope) {
// 		if vs, ok := s.VarMap[name]; ok {
// 			for _, v := range vs {
// 				if _, ok := tmp[v.value]; !ok {
// 					tmp[v.value] = struct{}{}
// 					ret = append(ret, v.value)
// 				}
// 			}
// 		}
// 	})

// 	if v, ok := f.externInstance[name]; ok {
// 		ret = append(ret, v)
// 	}
// 	return ret
// }

// func (f *Function) EachScope(handler func(*Scope)) {
// 	var handlerScope func(*Scope)
// 	handlerScope = func(s *Scope) {
// 		if s == nil {
// 			return
// 		}
// 		handler(s)
// 		for _, child := range s.Children {
// 			handlerScope(child)
// 		}
// 	}
// 	handlerScope(f.GetScope())
// }
