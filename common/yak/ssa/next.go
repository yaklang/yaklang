package ssa

var (
	NextOk    = NewConst("ok")
	NextField = NewConst("field")
	NextKey   = NewConst("key")
)

func newNextType(iterType Type, isIn bool) Type {
	typ := NewStructType()
	typ.AddField(NextOk, BasicTypes[BooleanTypeKind])

	switch iterType.GetTypeKind() {
	case SliceTypeKind:
		it := iterType.(*ObjectType)
		if isIn {
			typ.AddField(NextKey, it.FieldType)
			typ.AddField(NextField, BasicTypes[NullTypeKind])
		} else {
			typ.AddField(NextKey, it.KeyTyp)
			typ.AddField(NextField, it.FieldType)
		}
	case MapTypeKind:
		it := iterType.(*ObjectType)
		typ.AddField(NextKey, it.KeyTyp)
		typ.AddField(NextField, it.FieldType)
	case ChanTypeKind:
		it := iterType.(*ChanType)
		typ.AddField(NextKey, it.Elem)
		typ.AddField(NextField, BasicTypes[NullTypeKind])
	default:
		typ.AddField(NextKey, CreateAnyType())
		typ.AddField(NextField, CreateAnyType())
	}

	return typ
}
