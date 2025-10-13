package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

const (
	MAXTYPELEVEL = 15
)

func (b *FunctionBuilder) WithExternSideEffect(es map[string][]uint) {
	m := make(map[string]func(b *FunctionBuilder, id string, v any) (value Value))

	b.GetProgram().ExternSideEffect = es
	for n, _ := range es {
		m[n] = BuildFunctionWithSideEffect
	}
	b.WithExternBuildValueHandler(m)
}

func (b *FunctionBuilder) WithExternValue(vs map[string]any) {
	b.GetProgram().ExternInstance = vs
}

func (b *FunctionBuilder) WithExternLib(lib map[string]map[string]any) {
	b.GetProgram().ExternLib = lib
}

func (b *FunctionBuilder) WithExternBuildValueHandler(m map[string]func(b *FunctionBuilder, id string, v any) (value Value)) {
	b.GetProgram().externBuildValueHandler = m
}

func (b *FunctionBuilder) WithExternMethod(builder MethodBuilder) {
	ExternMethodBuilder = builder
}

func (b *FunctionBuilder) WithDefineFunction(defineFunc map[string]any) {
	b.DefineFunc = defineFunc
}

func TryGetSimilarityKey(table []string, name string) string {
	var score float64
	var ret string
	for _, libKey := range table {
		if strings.EqualFold(libKey, name) {
			// if strings.ToLower(libKey) == strings.ToLower(name) {
			return libKey
		}
		s := utils.CalcSimilarity([]byte(name), []byte(libKey))
		if score < s {
			score = s
			ret = libKey
		}
	}
	return ret
}

func (ex *ExternLib) BuildField(key string) Value {
	if id, ok := ex.MemberMap[key]; ok {
		if value, ok := ex.GetValueById(id); ok {
			return value
		}
	}
	b := ex.builder
	if v, ok := ex.table[key]; ok {
		v := b.BuildValueFromAny(ex.GetName()+"."+key, v)
		ex.MemberMap[key] = v.GetId()
		ex.Member = append(ex.Member, v.GetId())
		return v
	}
	return nil
}

func (prog *Program) TryGetSimilarityKey(name, key string) string {
	if prog.ExternLib == nil {
		return ""
	}
	var ret string
	if table, ok := prog.ExternLib[name]; ok {
		ret = TryGetSimilarityKey(lo.Keys(table), key)
	}
	return ret
}

func (b *FunctionBuilder) TryBuildExternLibValue(extern *ExternLib, key Value) Value {
	// write to extern Lib
	name := getExternLibMemberCall(extern, key)
	// read from scope, if assign to this library-value, return this value
	if ret := ReadVariableFromScopeAndParent(b.CurrentBlock.ScopeTable, name); !utils.IsNil(ret) {
		return ret.Value
	}
	// try build field
	if ret := extern.BuildField(key.String()); ret != nil {
		// set program offsetMap for extern value
		b.GetProgram().SetOffsetValue(ret, b.CurrentRange)

		// create variable for extern value
		variable := ret.GetVariable(name)
		if variable == nil {
			ret.AddVariable(b.CreateMemberCallVariable(extern, key))
		} else {
			variable.AddRange(b.CurrentRange, true)
		}

		// set member call
		setMemberCallRelationship(extern, key, ret)
		return ret
	}

	if lib, _ := b.GetProgram().GetLibrary(extern.GetName()); !utils.IsNil(lib) {
		text := key.GetName()
		if text == "" {
			text = key.String()
		}
		if g, ok := lib.GetGlobalVariable(text); ok {
			return g
		}
	}

	// try build field for phi
	if phiIns, ok := ToPhi(key); ok {
		if ret := b.tryBuildExternFieldForPhi(extern, phiIns, make(map[int64]struct{})); ret != nil {
			return ret
		} else {
			if m, ok := extern.GetMember(key); ok {
				return m
			}
		}
	} else {
		want := b.TryGetSimilarityKey(extern.GetName(), key.String())
		b.NewErrorWithPos(Error, SSATAG, b.CurrentRange, ExternFieldError("Lib", extern.GetName(), key.String(), want))
	}
	un := b.EmitUndefined(name)
	un.Kind = UndefinedMemberInValid
	un.SetExtern(true)
	setMemberCallRelationship(extern, key, un)
	return un
}

func (b *FunctionBuilder) tryBuildExternFieldForPhi(extern *ExternLib, phiIns *Phi, m map[int64]struct{}) Value {
	errorNotice := func(v Value) {
		want := b.TryGetSimilarityKey(extern.GetName(), v.String())
		b.NewErrorWithPos(Error, SSATAG, b.CurrentRange, ExternFieldError("Lib", extern.GetName(), v.String(), want))
		b.NewErrorWithPos(Error, SSATAG, v.GetRange(), ExternFieldError("Lib", extern.GetName(), v.String(), want))
	}

	if _, ok := m[phiIns.GetId()]; ok {
		return nil
	} else {
		m[phiIns.GetId()] = struct{}{}
	}

	var possibleRet Value // use last possibleRet as return value
	for _, possibleKey := range phiIns.GetValues() {
		// skip empty phi
		if possibleKey.GetId() == -1 && len(possibleKey.GetValues()) == 0 {
			continue
		}
		// recursive check phi
		if phiKey, ok := ToPhi(possibleKey); ok {
			if len(phiKey.Edge) == 0 {
				continue
			}
			if ret := b.tryBuildExternFieldForPhi(extern, phiKey, m); ret == nil {
				errorNotice(possibleKey)
				return possibleRet
			} else {
				possibleRet = ret
			}
		} else {
			if ret := extern.BuildField(possibleKey.String()); ret == nil {
				errorNotice(possibleKey)
				return possibleRet
			} else {
				possibleRet = ret
			}
		}
	}
	// all possibleKey exist
	return possibleRet
}

func (prog *Program) TryBuildExternValue(b *FunctionBuilder, id string) Value {
	getExternValue := func(id string) Value {
		if v, ok := prog.cacheExternInstance[id]; ok {
			return v
		}

		if prog.ExternInstance == nil {
			return nil
		}
		v, ok := prog.ExternInstance[id]
		if !ok {
			return nil
		}
		ret := prog.BuildValueFromAny(b, id, v)
		prog.cacheExternInstance[id] = ret
		return ret
	}

	getExternLib := func(id string) *ExternLib {
		if v, ok := prog.cacheExternInstance[id]; ok {
			if ex, ok := v.(*ExternLib); ok {
				return ex
			}
			return nil
		}

		if prog.ExternLib == nil {
			return nil
		}
		table, ok := prog.ExternLib[id]
		if !ok {
			return nil
		}
		ex := NewExternLib(id, b, table)
		ex.SetExtern(true)
		prog.cacheExternInstance[id] = ex
		return ex
	}

	getExternInstance := func(id string) Value {
		if ret := getExternValue(id); ret != nil {
			return ret
		}
		if ret := getExternLib(id); ret != nil {
			return ret
		}
		return nil
	}

	getExternField := func(lib string, key string) Value {
		if v, ok := prog.cacheExternInstance[id]; ok {
			return v
		}

		extern := getExternLib(lib)
		if extern == nil {
			return nil
		}
		ret := extern.BuildField(key)
		if ret != nil {
			prog.cacheExternInstance[id] = ret
		}
		return ret
	}

	if str := strings.Split(id, "."); len(str) == 1 {
		return getExternInstance(id)
	} else if len(str) == 2 {
		return getExternField(str[0], str[1])
	} else {
		return nil
	}
}

func (prog *Program) BuildValueFromAny(b *FunctionBuilder, id string, v any) (value Value) {
	itype := reflect.TypeOf(v)
	if itype == nil {
		return nil
	}
	str := id
	if handler, ok := prog.externBuildValueHandler[id]; ok && b != nil {
		value = handler(b, id, v)
	} else {
		switch itype.Kind() {
		case reflect.Func:
			funcType := prog.CoverReflectFunctionType(itype, 0)
			value = NewFunctionWithType(str, funcType)
		default:
			value = NewUndefined(str)
			value.SetType(prog.handlerType(itype, 0))
		}
		if b != nil {
			value.SetRange(b.CurrentRange)
		}
	}
	value.SetExtern(true)
	value.SetProgram(prog)
	prog.SetVirtualRegister(value)
	prog.SetInstructionWithName(str, value)
	return
}

func (prog *Program) CoverReflectFunctionType(itype reflect.Type, level int) *FunctionType {
	params := make([]Type, 0)
	returns := make([]Type, 0)
	isVariadic := itype.IsVariadic()
	// parameter
	for i := 0; i < itype.NumIn(); i++ {
		params = append(params, prog.handlerType(itype.In(i), level))
	}
	// return
	for i := 0; i < itype.NumOut(); i++ {
		returns = append(returns, prog.handlerType(itype.Out(i), level))
	}
	return NewFunctionTypeDefine(itype.String(), params, returns, isVariadic)
}

func (prog *Program) handlerType(typ reflect.Type, level int) Type {
	Name := typ.String()
	if Name == "[]uint8" {
		Name = "bytes"
	}

	// base type
	if t := GetTypeByStr(Name); t != nil {
		return t
	}

	_, pkgPathName := yakdoc.GetTypeNameWithPkgPath(typ)

	if hijackType, ok := prog.externType[Name]; ok {
		return hijackType
	}

	var ret Type

	// alias type
	if t := GetTypeByStr(typ.Kind().String()); t != nil {
		ret = NewAliasType(Name, pkgPathName, t)
	}

	// before this check, code will not recursive.
	// check level
	if level >= MAXTYPELEVEL {
		return CreateAnyType()
	}
	level++
	// below this code, will recursive

	isInterface := false
	// complex type
	if ret == nil {
		switch typ.Kind() {
		case reflect.Array, reflect.Slice:
			ret = NewSliceType(prog.handlerType(typ.Elem(), level))
		case reflect.Map:
			ret = NewMapType(prog.handlerType(typ.Key(), level), prog.handlerType(typ.Elem(), level))
		case reflect.Struct:
			structType := NewStructType()
			prog.externType[Name] = structType
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				fieldType := prog.handlerType(field.Type, level)
				structType.AddField(NewConst(field.Name), fieldType)
				if field.Anonymous && IsObjectType(fieldType) {
					structType.AnonymousField[Name] = fieldType.(*ObjectType)
				}
			}
			structType.Finish()
			ret = structType
		case reflect.Func:
			ret = prog.CoverReflectFunctionType(typ, level)
		case reflect.Pointer:
			// t := NewPointerType()
			// t.SetName("Pointer")
			// return t
			ret = prog.handlerType(typ.Elem(), level)
			return ret
		case reflect.UnsafePointer:
			obj := NewObjectType()
			obj.SetName(Name)
			ret = obj
		case reflect.Interface:
			isInterface = true
			ret = NewInterfaceType(Name, pkgPathName)
		case reflect.Chan:
			ret = NewChanType(prog.handlerType(typ.Elem(), level))
		default:
			if ret == nil {
				log.Errorf("cannot handler this type: %s", pkgPathName)
				ret = NewObjectType()
			}
		}
	}

	if ret != nil {
		prog.externType[Name] = ret
		if ityp, ok := ret.(*ObjectType); ok {
			ityp.SetName(Name)
			ityp.SetPkgPath(pkgPathName)
		}
	}

	// handler method
	ret.SetMethodGetter(func() map[string]*Function {
		pTyp := reflect.PointerTo(typ)
		methods := make(map[string]*Function, typ.NumMethod()+pTyp.NumMethod())
		handlerMethod := func(typ reflect.Type) {
			for i := 0; i < typ.NumMethod(); i++ {
				method := typ.Method(i)
				funTyp := prog.CoverReflectFunctionType(method.Type, level)
				if isInterface {
					funTyp.Parameter = utils.InsertSliceItem(funTyp.Parameter, ret, 0)
				}
				funTyp.SetName(fmt.Sprintf("%s.%s", pkgPathName, method.Name))
				methods[method.Name] = NewFunctionWithType(method.Name, funTyp)
			}
		}
		handlerMethod(typ)
		handlerMethod(pTyp)
		return methods
	})

	return ret
}

func (b *FunctionBuilder) TryGetSimilarityKey(name, key string) string {
	return b.GetProgram().TryGetSimilarityKey(name, key)
}

func (b *FunctionBuilder) TryBuildExternValue(id string) Value {
	return b.GetProgram().TryBuildExternValue(b, id)
}

func (b *FunctionBuilder) BuildValueFromAny(id string, v any) (value Value) {
	return b.GetProgram().BuildValueFromAny(b, id, v)
}

func (b *FunctionBuilder) CoverReflectFunctionType(itype reflect.Type, level int) *FunctionType {
	return b.GetProgram().CoverReflectFunctionType(itype, level)
}

func (b *FunctionBuilder) handlerType(typ reflect.Type, level int) Type {
	return b.GetProgram().handlerType(typ, level)
}

func (b *FunctionBuilder) TryBuildValueWithoutParent(name string, value Value) {
	scope := b.CurrentBlock.ScopeTable
	head := scope.GetHead()
	if scope.GetParent() == nil || value == nil {
		return
	}
	parentVariable := scope.GetParent().ReadVariable(name)
	if parentVariable == nil {
		variable := head.CreateVariable(name, false)
		head.AssignVariable(variable, value)
	}
}

func BuildFunctionWithSideEffect(b *FunctionBuilder, id string, v any) (value Value) {
	itype := reflect.TypeOf(v)
	prog := b.GetProgram()
	str := id

	switch itype.Kind() {
	case reflect.Func:
		funcType := prog.CoverReflectFunctionType(itype, 0)
		value = NewFunctionWithType(str, funcType)
		if se, ok := prog.ExternSideEffect[str]; ok {
			var modify Value
			for i, k := range se {
				switch k {
				case uint(SideEffectIn):
					p := b.NewParam("in")
					p.FormalParameterIndex = i
					modify = p
				}
			}
			for i, k := range se {
				if modify == nil {
					modify = b.EmitMakeBuildWithType(CreateAnyType(), nil, nil)
				}
				switch k {
				case uint(SideEffectOut):
					funcType.SideEffects = append(funcType.SideEffects, &FunctionSideEffect{
						Name:        "out",
						VerboseName: "",
						Modify:      modify.GetId(),
						Variable:    b.CreateVariable("out"),
						forceCreate: true,
						Kind:        PointerSideEffect,
						parameterMemberInner: &parameterMemberInner{
							MemberCallKind:        ParameterCall,
							MemberCallObjectIndex: i,
						},
					},
					)
				}
			}
		}
	}
	if b != nil {
		value.SetRange(b.CurrentRange)
	}
	return
}
