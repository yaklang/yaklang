package ssa

// TODO: remove this value
// this value will create in init function, is error
var (
	NextOk    = NewConst("ok")
	NextField = NewConst("field")
	NextKey   = NewConst("key")
)

func newNextType(iterType Type, isIn bool) Type {
	typ := NewStructType()
	typ.AddField(NextOk, CreateBooleanType())

	switch iterType.GetTypeKind() {
	case SliceTypeKind:
		it, ok := ToObjectType(iterType)
		if !ok {
			return typ
		}
		if isIn {
			typ.AddField(NextKey, it.FieldType)
			typ.AddField(NextField, CreateNullType())
		} else {
			typ.AddField(NextKey, it.KeyTyp)
			typ.AddField(NextField, it.FieldType)
		}
	case MapTypeKind:
		it, ok := ToObjectType(iterType)
		if !ok {
			return typ
		}
		typ.AddField(NextKey, it.KeyTyp)
		typ.AddField(NextField, it.FieldType)
	case ChanTypeKind:
		it := iterType.(*ChanType)
		typ.AddField(NextKey, it.Elem)
		typ.AddField(NextField, CreateNullType())
	default:
		typ.AddField(NextKey, CreateAnyType())
		typ.AddField(NextField, CreateAnyType())
	}

	return typ
}
