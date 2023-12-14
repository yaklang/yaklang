package rules

import (
	"reflect"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func init() {
	plugin_type_analyzer.RegisterSSAOptCollector("yak", YakGetTypeSSAOpt)
}

func YakGetTypeSSAOpt() []ssaapi.Option {
	opts := make([]ssaapi.Option, 0)
	// yak function table
	symbol := yaklang.New().GetFntable()
	valueTable := make(map[string]interface{})
	// libTable := make(map[string]interface{})
	tmp := reflect.TypeOf(make(map[string]interface{}))
	for name, item := range symbol {
		itype := reflect.TypeOf(item)
		if itype == tmp {
			opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]interface{})))
		} else {
			valueTable[name] = item
		}
	}

	// yak-main
	valueTable["YAK_DIR"] = ""
	valueTable["YAK_FILENAME"] = ""
	valueTable["YAK_MAIN"] = false
	valueTable["id"] = ""
	// param
	getParam := func(key string) interface{} {
		return nil
	}
	valueTable["getParam"] = getParam
	valueTable["getParams"] = getParam
	valueTable["param"] = getParam

	opts = append(opts, ssaapi.WithExternValue(valueTable))
	opts = append(opts, ssaapi.WithExternMethod(&Builder{}))
	opts = append(opts, ssaapi.WithExternInfo("plugin-type:yak"))
	return opts
}

// for method Builder
type Builder struct{}

var _ (ssa.MethodBuilder) = (*Builder)(nil)

func (b *Builder) Build(t ssa.Type, s string) *ssa.FunctionType {
	var (
		arg          = []ssa.Type{t}
		ret          = []ssa.Type{}
		IsVariadic   = false
		IsModifySelf = false
		name         = ""
	)

	var (
		StrTyp      = ssa.BasicTypes[ssa.String]
		NumberTyp   = ssa.BasicTypes[ssa.Number]
		BoolTyp     = ssa.BasicTypes[ssa.Boolean]
		AnyTyp      = ssa.BasicTypes[ssa.Any]
		BytesTyp    = ssa.BasicTypes[ssa.Bytes]
		HandlerFunc = func(arg, ret []ssa.Type, isVar bool) ssa.Type {
			return ssa.NewFunctionTypeDefine("handler", arg, ret, isVar)
		}
		SliceTyp = func(t ssa.Type) ssa.Type {
			return ssa.NewSliceType(t)
		}
		MapTyp = func(k ssa.Type, v ssa.Type) ssa.Type {
			return ssa.NewMapType(k, v)
		}
	)

	switch t.GetTypeKind() {
	case ssa.MapTypeKind:
		ot, _ := ssa.ToObjectType(t)
		fieldTyp := ot.FieldType
		keyTyp := ot.KeyTyp
		name += "map." + s
		switch s {
		case "Keys":
			ret = append(ret, SliceTyp(keyTyp))
		case "Values":
			ret = append(ret, SliceTyp(fieldTyp))
		case "Entries", "Item":
			// TODO: handle this return type: the map[T]U return [][1:T, 2:U]
			ret = append(ret, SliceTyp(AnyTyp))
		case "ForEach":
			arg = append(arg, HandlerFunc([]ssa.Type{keyTyp, fieldTyp}, []ssa.Type{}, false))
		case "Set":
			IsModifySelf = true
			arg = append(arg, keyTyp, fieldTyp)
			// TODO: this return value always True
			ret = append(ret, BoolTyp)
		case "Remove", "Delete":
			IsModifySelf = true
			arg = append(arg, keyTyp)
		case "Has", "IsExisted":
			arg = append(arg, keyTyp)
			ret = append(ret, BoolTyp)
		case "Length", "Len":
			ret = append(ret, NumberTyp)
		default:
			name = ""
		}
	case ssa.SliceTypeKind:
		ot, _ := ssa.ToObjectType(t)
		fieldTyp := ot.FieldType
		name = "slice." + s
		switch s {
		case "Append", "Push":
			IsModifySelf = true
			IsVariadic = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Pop":
			IsVariadic = true
			IsModifySelf = true
			ret = append(ret, fieldTyp)
		case "Extend", "Merge":
			IsModifySelf = true
			arg = append(arg, ot)
			ret = append(ret, ot)
		case "Length", "Len":
			ret = append(ret, NumberTyp)
		case "Capability", "Cap":
			ret = append(ret, NumberTyp)
		case "StringSlice":
			ret = append(ret, SliceTyp(StrTyp))
		case "GeneralSlice":
			ret = append(ret, SliceTyp(AnyTyp))
		case "Shift":
			IsModifySelf = true
			ret = append(ret, fieldTyp)
		case "Unshift":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Map":
			arg = append(arg, HandlerFunc([]ssa.Type{fieldTyp}, []ssa.Type{AnyTyp}, false))
			ret = append(ret, SliceTyp(AnyTyp))
		case "Filter":
			arg = append(arg, HandlerFunc([]ssa.Type{fieldTyp}, []ssa.Type{BoolTyp}, false))
			ret = append(ret, ot)
		case "Insert":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Remove":
			IsModifySelf = true
			arg = append(arg, fieldTyp)
			ret = append(ret, ot)
		case "Reverse":
			IsModifySelf = true
			ret = append(ret, ot)
		case "Sort":
			IsModifySelf = true
			IsVariadic = true
			arg = append(arg, BoolTyp)
			ret = append(ret, ot)
		case "Clear":
			IsModifySelf = true
			ret = append(ret, ot)
		case "Count":
			arg = append(arg, fieldTyp)
			ret = append(ret, NumberTyp)
		case "Index":
			arg = append(arg, NumberTyp)
			ret = append(ret, fieldTyp)
		default:
			name = ""
		}
	case ssa.String:
		name += "string." + s
		switch s {
		case "First":
			ret = append(ret, NumberTyp)
		case "Reverse":
			ret = append(ret, StrTyp)
		case "Shuffle":
			ret = append(ret, StrTyp)
		case "Fuzz":
			arg = append(arg, MapTyp(StrTyp, AnyTyp))
			ret = append(ret, SliceTyp(StrTyp))
			IsVariadic = true
		case "Contains":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "IContains":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "ReplaceN":
			arg = append(arg, StrTyp, StrTyp, NumberTyp)
			ret = append(ret, StrTyp)
		case "ReplaceAll", "Replace":
			arg = append(arg, StrTyp, StrTyp)
			ret = append(ret, StrTyp)
		case "Split":
			arg = append(arg, StrTyp)
			ret = append(ret, SliceTyp(StrTyp))
		case "SplitN":
			arg = append(arg, StrTyp, NumberTyp)
			ret = append(ret, SliceTyp(StrTyp))
		case "Join":
			arg = append(arg, SliceTyp(AnyTyp))
			ret = append(ret, StrTyp)
		case "Trim", "TrimLeft", "TrimRight":
			arg = append(arg, StrTyp)
			ret = append(ret, StrTyp)
			IsVariadic = true
		case "HasPrefix", "HasSuffix", "StartsWith", "EndsWith":
			arg = append(arg, StrTyp)
			ret = append(ret, BoolTyp)
		case "RemovePrefix", "RemoveSuffix":
			arg = append(arg, StrTyp)
			ret = append(ret, StrTyp)
		case "Zfill", "Rfill", "Lfill":
			arg = append(arg, NumberTyp)
			ret = append(ret, StrTyp)
		case "Ljust", "Rjust":
			arg = append(arg, NumberTyp, StrTyp)
			ret = append(ret, StrTyp)
			IsVariadic = true
		case "Count", "Find", "RFind", "IndexOf", "LastIndexOf":
			arg = append(arg, StrTyp)
			ret = append(ret, NumberTyp)
		case "Lower", "Upper", "Title":
			ret = append(ret, StrTyp)
		case "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable":
			ret = append(ret, BoolTyp)
		default:
			name = ""
		}
	case ssa.Bytes:
		name = "bytes." + s
		switch s {
		case "First":
			ret = append(ret, NumberTyp)
		case "Reverse", "Shuffle":
			ret = append(ret, BytesTyp)
		case "Fuzz":
			arg = append(arg, MapTyp(StrTyp, AnyTyp))
			ret = append(ret, SliceTyp(StrTyp))
			IsVariadic = true
		case "Contains", "IContains":
			arg = append(arg, BytesTyp)
			ret = append(ret, BoolTyp)
		case "ReplaceN":
			arg = append(arg, BytesTyp, BytesTyp, NumberTyp)
			ret = append(ret, BytesTyp)
		case "ReplaceAll", "Replace":
			arg = append(arg, BytesTyp, BytesTyp)
			ret = append(ret, BytesTyp)
		case "Split":
			arg = append(arg, BytesTyp)
			ret = append(ret, SliceTyp(BytesTyp))
		case "SplitN":
			arg = append(arg, BytesTyp, NumberTyp)
			ret = append(ret, SliceTyp(BytesTyp))
		case "Join":
			arg = append(arg, AnyTyp)
			ret = append(ret, BytesTyp)
		case "Trim", "TrimLeft", "TrimRight":
			IsVariadic = true
			arg = append(arg, BytesTyp)
			ret = append(ret, BytesTyp)
		case "HasPrefix", "HasSuffix", "StartsWith", "EndsWith":
			arg = append(arg, BytesTyp)
			ret = append(ret, BoolTyp)
		case "RemovePrefix", "RemoveSuffix":
			arg = append(arg, BytesTyp)
			ret = append(ret, BytesTyp)
		case "Zfill", "Rzfill":
			arg = append(arg, NumberTyp)
			ret = append(ret, BytesTyp)
		case "Ljust", "Rjust":
			IsVariadic = true
			arg = append(arg, NumberTyp, BytesTyp)
			ret = append(ret, BytesTyp)
		case "Count", "Find", "Rfind", "IndexOf", "LastIndexOf":
			arg = append(arg, BytesTyp)
			ret = append(ret, NumberTyp)
		case "Lower", "Upper", "Title":
			ret = append(ret, BytesTyp)
		case "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable":
			ret = append(ret, BoolTyp)
		default:
			name = ""
		}
	}
	if name != "" {
		f := ssa.NewFunctionTypeDefine(name, arg, ret, IsVariadic)
		f.SetModifySelf(IsModifySelf)
		return f
	} else {
		return nil
	}
}

func (b *Builder) GetMethodNames(t ssa.Type) []string {
	switch t.GetTypeKind() {
	case ssa.SliceTypeKind:
		return []string{
			"Append", "Push", "Pop", "Extend", "Merge", "Length", "Len", "Capability", "Cap", "StringSlice", "GeneralSlice", "Shift", "Unshift", "Map", "Filter", "Insert", "Remove", "Reverse", "Sort", "Clear", "Count", "Index",
		}
	case ssa.String:
		return []string{
			"Reverse", "Shuffle", "Fuzz", "Contains", "IContains", "ReplaceN", "ReplaceAll", "Replace", "Split", "SplitN", "Join", "Trim", "TrimLeft", "TrimRight", "HasPrefix", "HasSuffix", "StartsWith", "EndsWith", "RemovePrefix", "RemoveSuffix", "Zfill", "Rfill", "Lfill", "Ljust", "Rjust", "Count", "Find", "RFind", "IndexOf", "LastIndexOf", "Lower", "Upper", "Title", "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable",
		}
	case ssa.MapTypeKind:
		return []string{
			"Keys", "Values", "Entries", "Item", "ForEach", "Set", "Remove", "Delete", "Has", "IsExisted", "Length", "Len",
		}
	case ssa.Bytes:
		return []string{
			"First", "Reverse", "Shuffle", "FUzz", "Contains", "IContains", "ReplaceN", "ReplaceAll", "Replace", "Split", "SplitN", "Join", "Trim", "TrimLeft", "TrimRight", "HasPrefix", "HasSuffix", "StartsWith", "EndsWith", "RemovePrefix", "RemoveSuffix", "Zfill", "Rzfill", "Ljust", "Rjust", "Count", "Find", "Rfind", "IndexOf", "LastIndexOf", "Lower", "Upper", "Title", "IsLower", "IsUpper", "IsTitle", "IsAlpha", "IsDigit", "IsAlnum", "IsPrintable",
		}
	default:
		return []string{}
	}
}
