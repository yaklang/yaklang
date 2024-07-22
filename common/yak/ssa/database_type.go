package ssa

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveTypeToDB(typ Type) int {
	if typ == nil {
		return -1
	}
	kind := int(typ.GetTypeKind())

	str := typ.String()

	param := make(map[string]any)
	switch t := typ.(type) {
	case *FunctionType:
		param["name"] = t.Name
	case *ObjectType:
		param["name"] = t.Name
	default:
	}
	extra, err := json.Marshal(param)
	if err != nil {
		log.Errorf("SaveTypeToDB: %v: param: %v", err, param)
	}

	return ssadb.SaveType(kind, str, utils.UnsafeBytesToString(extra))
}

func GetTypeFromDB(id int) Type {

	kind, str, extra, err := ssadb.GetType(id)
	if err != nil {
		return nil
	}

	_ = str
	_ = extra
	params := make(map[string]any)
	if err := json.Unmarshal(utils.UnsafeStringToBytes(extra), &params); err != nil {
		log.Errorf("GetTypeFromDB: %v: extra: %v", err, extra)
	}

	switch TypeKind(kind) {
	case FunctionTypeKind:
		typ := &FunctionType{}
		if raw, ok := params["return_value"].(string); ok {
			_ = raw
			// typ.ReturnValue = lo.FilterMap(strings.Split(raw, ","), func(s string, _ int) (*Return, bool) {
			// 	id, err := strconv.ParseInt(s, 10, 64)
			// 	if err != nil {
			// 		return nil, false
			// 	}
			// 	r, err := NewInstructionFromLazy(id, ToReturn)
			// 	if err != nil {
			// 		return nil, false
			// 	}
			// 	return r, true
			// })
		}
		return typ
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, TupleTypeKind, StructTypeKind:
		typ := &ObjectType{}
		typ.Name = params["name"].(string)
		typ.Kind = TypeKind(kind)
		return typ
	default:

	}

	return nil
}
