package ssa

func BindingGenericTypeWithRealType(real, generic Type, symbolsTypeMap map[string]Type) (errMsg string) {
	setBinding := func(genericSymbol *GenericType, bindType Type) {
		if existed, ok := symbolsTypeMap[genericSymbol.symbol]; ok {
			if !TypeCompare(existed, bindType) {
				errMsg = GenericTypeError(genericSymbol, generic, existed, bindType)
				// fallback symbol to any
				symbolsTypeMap[genericSymbol.symbol] = GetAnyType()
			}
		} else {
			symbolsTypeMap[genericSymbol.symbol] = bindType
		}
	}

	switch real.GetTypeKind() {
	case BytesTypeKind:
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.FieldType) {
			setBinding(t.FieldType.(*GenericType), GetByteType())
		}
		// if T is bytes
		fallthrough
	case StringTypeKind, NumberTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind, InterfaceTypeKind:
		if isGenericType(generic) {
			setBinding(generic.(*GenericType), real)
		}
	case ChanTypeKind:
		if t, ok := generic.(*ChanType); ok && isGenericType(t.Elem) {
			setBinding(t.Elem.(*GenericType), real.(*ChanType).Elem)
		}
	case SliceTypeKind:
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.FieldType) {
			setBinding(t.FieldType.(*GenericType), real.(*ObjectType).FieldType)
		}
	case MapTypeKind:
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.KeyTyp) {
			setBinding(t.KeyTyp.(*GenericType), real.(*ObjectType).KeyTyp)
		}
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.FieldType) {
			setBinding(t.FieldType.(*GenericType), real.(*ObjectType).FieldType)
		}
	case TupleTypeKind:
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.FieldType) {
			setBinding(t.FieldType.(*GenericType), real.(*ObjectType).FieldType)
		}
	case FunctionTypeKind:
		if t, ok := generic.(*FunctionType); ok {
			rt := real.(*FunctionType)
			for i, typ := range t.Parameter {
				if !isGenericType(typ) {
					continue
				}
				setBinding(typ.(*GenericType), rt.Parameter[i])
			}
			if isGenericType(t.ReturnType) {
				setBinding(t.ReturnType.(*GenericType), rt.ReturnType)
			}
		}
	}
	return
}

func ApplyGenericType(raw Type, symbolsTypeMap map[string]Type) Type {
	cloned, ok := CloneType(raw)
	if !ok {
		return raw
	}

	switch raw.GetTypeKind() {
	case GenericTypeKind:
		bindType, ok := symbolsTypeMap[raw.(*GenericType).symbol]
		if !ok {
			return cloned
		}
		if new, ok := CloneType(bindType); ok {
			return new
		}
	case ChanTypeKind:
		t := cloned.(*ChanType)
		t.Elem = ApplyGenericType(t.Elem, symbolsTypeMap)
		return cloned
	case SliceTypeKind:
		t := cloned.(*ObjectType)
		t.FieldType = ApplyGenericType(t.FieldType, symbolsTypeMap)
		// hook bytes
		if t.FieldType.GetTypeKind() == ByteTypeKind {
			cloned = GetBytesType()
		}
	case MapTypeKind:
		t := cloned.(*ObjectType)
		t.KeyTyp = ApplyGenericType(t.KeyTyp, symbolsTypeMap)
		t.FieldType = ApplyGenericType(t.FieldType, symbolsTypeMap)
	case TupleTypeKind:
		t := cloned.(*ObjectType)
		t.FieldType = ApplyGenericType(t.FieldType, symbolsTypeMap)
	case FunctionTypeKind:
		t := cloned.(*FunctionType)
		for i, typ := range t.Parameter {
			t.Parameter[i] = ApplyGenericType(typ, symbolsTypeMap)
		}
		t.ReturnType = ApplyGenericType(t.ReturnType, symbolsTypeMap)
	}

	return cloned
}
