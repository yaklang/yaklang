package ssa

import "github.com/yaklang/yaklang/common/utils"

func GetScopeFromIrScopeName(programName, ScopeName string) (*ScopeInstance, error) {
	return nil, utils.Errorf("not implemented")
	// insts, err := ssadb.GetScope(programName, ScopeName)
	// if err != nil {
	// 	return nil, err
	// }

	// ret := NewScope(nil, programName)

	// for _, index := range insts {
	// 	variable := ret.CreateVariable(index.VariableName, false)
	// 	value, err := NewLazyValue(index.ValueID)
	// 	if err != nil {
	// 		continue
	// 	}
	// 	ret.AssignVariable(variable, value)
	// }

	// return ret, nil
}

func SyncFromDatabase(s *ScopeInstance) error {
	return nil
	//if !s.ShouldSaveToDatabase() {
	//	return nil
	//}
	//
	//node, err := ssadb.GetIrScope(s.GetPersistentId())
	//if err != nil {
	//	return utils.Wrapf(err, "failed to get tree node")
	//}
	//
	//// handle persistent id
	//var params = make(map[string]any)
	//if err := json.Unmarshal([]byte(node.ExtraInfo), &params); err != nil {
	//	return utils.Wrapf(err, "failed to unmarshal extra info")
	//}
	//
	//// load to database
	//s.SetScopeLevel(utils.MapGetInt(params, "level"))
	//
	//quotedValues := utils.MapGetString(params, "values")
	//quotedVariable := utils.MapGetString(params, "variable")
	//quotedCaptured := utils.MapGetString(params, "captured")
	//quotedIncomingPhi := utils.MapGetString(params, "incomingPhi")
	//s.SetSpinRaw(utils.MapGetBool(params, "spin"))
	//s.SetParentId(utils.MapGetInt64(params, "parent"))
	//
	//values, err := strconv.Unquote(quotedValues)
	//if err != nil {
	//	return utils.Wrapf(err, "unquote values error")
	//}
	//if gres := gjson.Parse(values); gres.IsObject() {
	//	gres.ForEach(func(key, value gjson.Result) bool {
	//		if element := gjson.Parse(value.Raw); element.IsArray() {
	//			m := omap.NewOrderedMap(make(map[string]ssautil.VersionedIF[Value]))
	//			for _, versioned := range element.Array() {
	//				var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
	//				if err := v.UnmarshalJSON([]byte(versioned.Raw)); err != nil {
	//					log.Warnf("BUG: marshal versioned error: %v raw: %v", err, string(versioned.Raw))
	//					return true
	//				}
	//				if v.GetScope() == nil {
	//					v.SetScope(s)
	//				}
	//				m.Push(v)
	//			}
	//			s.GetValues().Set(key.String(), m)
	//		}
	//		return true
	//	})
	//}
	//
	//variable, err := strconv.Unquote(quotedVariable)
	//if err != nil {
	//	return utils.Wrapf(err, "unquote variable error")
	//}
	//if gres := gjson.Parse(variable); gres.IsObject() {
	//	for k, v := range gres.Map() {
	//		symbolId := codec.Atoi(fmt.Sprint(k))
	//		var values []ssautil.VersionedIF[Value]
	//		if arr := gjson.Parse(v.Raw); arr.IsArray() {
	//			for _, result := range arr.Array() {
	//				var versioned = new(ssautil.Versioned[Value])
	//				err := json.Unmarshal([]byte(result.Raw), versioned)
	//				if err != nil {
	//					log.Warnf("failed to unmarshal key(T): %v, data:%v", err, result.Raw)
	//					continue
	//				}
	//				if versioned.GetScope() == nil {
	//					versioned.SetScope(s)
	//				}
	//				values = append(values, versioned)
	//			}
	//		}
	//		lz, err := NewLazyInstruction(int64(symbolId))
	//		if err != nil {
	//			return utils.Wrapf(err, "failed to get lazy instruction [%v]", symbolId)
	//		}
	//		s.GetVariable().Set(lz, values)
	//	}
	//}
	//
	//captured, err := strconv.Unquote(quotedCaptured)
	//if err != nil {
	//	return utils.Wrapf(err, "unquote captured error")
	//}
	//if gres := gjson.Parse(captured); gres.IsObject() {
	//	gres.ForEach(func(key, value gjson.Result) bool {
	//		var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
	//		err := json.Unmarshal([]byte(value.Raw), v)
	//		if err != nil {
	//			return false
	//		}
	//		if v.GetScope() == nil {
	//			v.SetScope(s)
	//		}
	//		s.GetCaptured().Set(key.String(), v)
	//		return true
	//	})
	//}
	//
	//incomingPhi, err := strconv.Unquote(quotedIncomingPhi)
	//if err != nil {
	//	return utils.Wrapf(err, "unquote incomingPhi error")
	//}
	//if gres := gjson.Parse(incomingPhi); gres.IsObject() {
	//	gres.ForEach(func(key, value gjson.Result) bool {
	//		var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
	//		err := json.Unmarshal([]byte(value.Raw), &v)
	//		if err != nil {
	//			return false
	//		}
	//		if v.GetScope() == nil {
	//			v.SetScope(s)
	//		}
	//		s.GetIncomingPhi().Set(key.String(), v)
	//		return true
	//	})
	//}
	//
	//return nil
}
