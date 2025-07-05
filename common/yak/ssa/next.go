package ssa

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
		it, _ := ToObjectType(iterType)
		if isIn {
			typ.AddField(NextKey, it.FieldType)
			typ.AddField(NextField, CreateNullType())
		} else {
			typ.AddField(NextKey, it.KeyTyp)
			typ.AddField(NextField, it.FieldType)
		}
	case MapTypeKind:
		it, _ := ToObjectType(iterType)
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
