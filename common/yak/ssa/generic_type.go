package ssa

func BindingGenericTypeWithRealType(real, generic Type, symbolsTypeMap map[string]Type) (errMsg string) {
	// real is not generic type, return
	if !TypeCompare(real, generic) {
		return
	}

	setBinding := func(genericSymbol *GenericType, bindType Type) {
		if existed, ok := symbolsTypeMap[genericSymbol.symbol]; ok {
			if !TypeCompare(existed, bindType) {
				errMsg = GenericTypeError(genericSymbol, generic, existed, bindType)
				// fallback symbol to any
				symbolsTypeMap[genericSymbol.symbol] = CreateAnyType()
			}
		} else {
			symbolsTypeMap[genericSymbol.symbol] = bindType
		}
	}

	if generic.GetTypeKind() == OrTypeKind {
		for _, t := range generic.(*OrType).types {
			BindingGenericTypeWithRealType(real, t, symbolsTypeMap)
		}
		return
	}

	switch real.GetTypeKind() {
	case BytesTypeKind:
		if t, ok := generic.(*ObjectType); ok && isGenericType(t.FieldType) {
			setBinding(t.FieldType.(*GenericType), CreateByteType())
		}
		// if T is bytes
		fallthrough
	case ChanTypeKind:
		if t, ok := generic.(*ChanType); ok && isGenericType(t.Elem) {
			if isGenericType(t.Elem) {
				setBinding(t.Elem.(*GenericType), real.(*ChanType).Elem)
			} else {
				BindingGenericTypeWithRealType(real.(*ChanType).Elem, t.Elem, symbolsTypeMap)
			}
		}
	case SliceTypeKind, TupleTypeKind:
		if t, ok := generic.(*ObjectType); ok {
			if isGenericType(t.FieldType) {
				setBinding(t.FieldType.(*GenericType), real.(*ObjectType).FieldType)
			} else {
				BindingGenericTypeWithRealType(real.(*ObjectType).FieldType, t.FieldType, symbolsTypeMap)
			}
		}
	case MapTypeKind:
		if t, ok := generic.(*ObjectType); ok {
			if isGenericType(t.KeyTyp) {
				setBinding(t.KeyTyp.(*GenericType), real.(*ObjectType).KeyTyp)
			} else {
				BindingGenericTypeWithRealType(real.(*ObjectType).KeyTyp, t.KeyTyp, symbolsTypeMap)
			}

			if isGenericType(t.FieldType) {
				setBinding(t.FieldType.(*GenericType), real.(*ObjectType).FieldType)
			} else {
				BindingGenericTypeWithRealType(real.(*ObjectType).FieldType, t.FieldType, symbolsTypeMap)
			}
		}
	case FunctionTypeKind:
		if t, ok := generic.(*FunctionType); ok {
			rt := real.(*FunctionType)
			for i, typ := range t.Parameter {
				if isGenericType(typ) {
					setBinding(typ.(*GenericType), rt.Parameter[i])
				} else {
					BindingGenericTypeWithRealType(rt.Parameter[i], typ, symbolsTypeMap)
				}
			}
			if isGenericType(t.ReturnType) {
				setBinding(t.ReturnType.(*GenericType), rt.ReturnType)
			} else {
				BindingGenericTypeWithRealType(rt.ReturnType, t.ReturnType, symbolsTypeMap)
			}
		}
	}

	if isGenericType(generic) {
		setBinding(generic.(*GenericType), real)
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
	case SliceTypeKind, TupleTypeKind:
		t := cloned.(*ObjectType)
		t.FieldType = ApplyGenericType(t.FieldType, symbolsTypeMap)
		// hook bytes
		if t.FieldType.GetTypeKind() == ByteTypeKind {
			cloned = CreateBytesType()
		}
	case MapTypeKind:
		t := cloned.(*ObjectType)
		t.KeyTyp = ApplyGenericType(t.KeyTyp, symbolsTypeMap)
		t.FieldType = ApplyGenericType(t.FieldType, symbolsTypeMap)
	case FunctionTypeKind:
		t := cloned.(*FunctionType)
		for i, typ := range t.Parameter {
			t.Parameter[i] = ApplyGenericType(typ, symbolsTypeMap)
		}
		t.ReturnType = ApplyGenericType(t.ReturnType, symbolsTypeMap)
	case OrTypeKind:
		t := cloned.(*OrType)
		for i, typ := range t.types {
			t.types[i] = ApplyGenericType(typ, symbolsTypeMap)
		}
	}

	return cloned
}
