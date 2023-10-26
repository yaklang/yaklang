package ssa4analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TITAG = "TypeInference"

type TypeInference struct {
	Finish map[ssa.Value]struct{}
}

func NewTypeInference(config) Analyzer {
	return &TypeInference{
		Finish: make(map[ssa.Value]struct{}),
	}
}

func (t *TypeInference) Run(prog *ssa.Program) {
	for _, pkg := range prog.Packages {
		for _, fun := range pkg.Funcs {
			t.RunOnFunction(fun)
		}
	}
}

func (t *TypeInference) RunOnFunction(fun *ssa.Function) {
	for _, b := range fun.Blocks {
		for _, inst := range b.Insts {
			t.InferenceOnInstruction(inst)
		}
	}
}

func (t *TypeInference) InferenceOnInstruction(inst ssa.Instruction) {

	if iv, ok := inst.(ssa.Value); ok {
		t := iv.GetType()
		if utils.IsNil(t) {
			iv.SetType(ssa.BasicTypes[ssa.Null])
		}
	}

	switch inst := inst.(type) {
	case *ssa.Phi:
		// return t.TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		t.TypeInferenceBinOp(inst)
	case *ssa.Call:
		t.TypeInferenceCall(inst)
	// case *ssa.Return:
	// 	return t.TypeInferenceReturn(inst)
	// case *ssa.Switch:
	// case *ssa.If:
	case *ssa.Next:
		t.TypeInferenceNext(inst)
	case *ssa.Make:
		t.TypeInferenceMake(inst)
	case *ssa.Field:
		t.TypeInferenceField(inst)
	case *ssa.Update:
		// return t.TypeInferenceUpdate(inst)
	}
}

func collectTypeFromValues(values []ssa.Value, skip func(int, ssa.Value) bool) []ssa.Type {
	typMap := make(map[ssa.Type]struct{})
	typs := make([]ssa.Type, 0, len(values))
	for index, v := range values {
		// skip function
		if skip(index, v) {
			continue
		}
		// uniq typ
		typ := v.GetType()
		if _, ok := typMap[typ]; !ok {
			typMap[typ] = struct{}{}
			typs = append(typs, typ)
		}
	}
	return typs
}

// if all finish, return false
func (t *TypeInference) checkValuesNotFinish(vs []ssa.Value) bool {
	for _, v := range vs {
		if _, ok := t.Finish[v]; !ok {
			return true
		}
	}
	return false
}

/*
if v.Type !match typ return true
if v.Type match  typ return false
*/
func checkType(v ssa.Value, typ ssa.Type) bool {
	if v.GetType() == nil {
		v.SetType(typ)
		return false
	}
	v.SetType(typ)
	return true
}

func (t *TypeInference) TypeInferenceNext(next *ssa.Next) {

	/*
		next map[T]U

		{
			key: T
			field: U
			ok: bool
		}
	*/
	typ := ssa.NewStructType()
	typ.AddField(ssa.NewConst("ok"), ssa.BasicTypes[ssa.Boolean])
	if it, ok := next.Iter.GetType().(*ssa.ObjectType); ok {
		switch it.Kind {
		case ssa.Slice:
			if next.InNext {
				typ.AddField(ssa.NewConst("key"), it.FieldType)
				typ.AddField(ssa.NewConst("field"), ssa.BasicTypes[ssa.Null])
			} else {
				typ.AddField(ssa.NewConst("key"), it.KeyTyp)
				typ.AddField(ssa.NewConst("field"), it.FieldType)
			}
		case ssa.Struct:
			typ.AddField(ssa.NewConst("key"), ssa.BasicTypes[ssa.String])
			typ.AddField(ssa.NewConst("field"), ssa.BasicTypes[ssa.Any])
		case ssa.Map:
			typ.AddField(ssa.NewConst("key"), it.KeyTyp)
			typ.AddField(ssa.NewConst("field"), it.FieldType)
		}
		next.SetType(typ)
	}
}

func (t *TypeInference) TypeInferencePhi(phi *ssa.Phi) {

	// check
	// TODO: handler Acyclic graph
	if t.checkValuesNotFinish(phi.Edge) {
		return
	}

	// set type
	typs := collectTypeFromValues(
		phi.Edge,
		// // skip unreachable block
		func(index int, value ssa.Value) bool {
			block := phi.GetBlock().Preds[index]
			return block.Reachable() == -1
		},
	)

	// only first set type, phi will change
	phi.SetType(typs[0])
}

func (t *TypeInference) TypeInferenceBinOp(bin *ssa.BinOp) {
	XTyps := bin.X.GetType()
	YTyps := bin.Y.GetType()

	handlerBinOpType := func(x, y ssa.Type) ssa.Type {
		if x == nil {
			return y
		}
		if x.GetTypeKind() == y.GetTypeKind() {
			return x
		}

		if x.GetTypeKind() == ssa.Any {
			return y
		}
		if y.GetTypeKind() == ssa.Any {
			return x
		}

		// if y.GetTypeKind() == ssa.Null {
		if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
			return ssa.BasicTypes[ssa.Boolean]
		}
		// }
		return nil
	}
	retTyp := handlerBinOpType(XTyps, YTyps)
	if retTyp == nil {
		// bin.NewError(ssa.Error, TITAG, "this expression type error: x[%s] %s y[%s]", XTyps, ssa.BinaryOpcodeName[bin.Op], YTyps)
		return
	}

	// typ := handler
	if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
		bin.SetType(ssa.BasicTypes[ssa.Boolean])
		return
	} else {
		bin.SetType(retTyp)
		return
	}
}

func (t *TypeInference) TypeInferenceMake(i *ssa.Make) {
}

func (t *TypeInference) TypeInferenceField(f *ssa.Field) {
	if t := f.Obj.GetType(); t != nil {
		if methodTyp := t.GetMethod(f.Key.String()); methodTyp != nil && f.GetType() != methodTyp {
			f.SetType(methodTyp)
			f.IsMethod = true
			return
		}
		if utils.IsNil(t) {
			t = ssa.BasicTypes[ssa.Null]
		}
		switch t.GetTypeKind() {
		case ssa.ObjectTypeKind:
			interfaceTyp := f.Obj.GetType().(*ssa.ObjectType)
			fTyp, _ := interfaceTyp.GetField(f.Key)
			if !utils.IsNil(fTyp) {
				f.SetType(fTyp)
				return
			}
		case ssa.String:
			f.SetType(ssa.BasicTypes[ssa.Number])
			return
		case ssa.Any:
			//pass
			f.SetType(ssa.BasicTypes[ssa.Any])
			return
		default:
		}
		if c, ok := ssa.ToCall(f.Obj); ok && c.Unpack {
			// pass
		} else {
			text := ""
			if ci, ok := ssa.ToConst(f.Key); ok {
				text = ci.String()
				want := ssa.TryGetSimilarityKey(t.GetAllKey(), text)
				if want != "" {
					f.NewError(ssa.Error, TITAG, ssa.ExternFieldError("Type", t.String(), text, want))
					return
				}
			}
			if text == "" {
				list := strings.Split(f.GetPosition().SourceCode, ".")
				text = list[len(list)-1]
			}
			f.Key.NewError(ssa.Error, TITAG, InvalidField(t.String(), text))
		}
	}
	// use update
	// vs := lo.FilterMap(f.GetValues(), func(v ssa.Value, i int) (ssa.Value, bool) {
	// 	// switch v := v.(type) {
	// 	// // case *ssa.Update:
	// 	// // 	return v.Value, true
	// 	// default:
	// 	// 	return nil, false
	// 	// }
	// })

	// // check value finish
	// // TODO: handler Acyclic Graph
	// if t.checkValuesNotFinish(vs) {
	// 	return
	// }

	// ts := collectTypeFromValues(
	// 	// f.Update,
	// 	vs,
	// 	func(i int, v ssa.Value) bool { return false },
	// )
	// if len(ts) == 0 {
	// 	f.SetType(ssa.BasicTypes[ssa.Null])
	// } else if len(ts) == 1 {
	// 	f.SetType(ts[0])
	// } else {
	// 	f.SetType(ssa.BasicTypes[ssa.Any])
	// }
}
func (t *TypeInference) TypeInferenceCall(c *ssa.Call) {

	// handler call method
	if field, ok := c.Method.(*ssa.Field); ok && field.IsMethod {
		if v, ok := field.Obj.(ssa.Value); ok {
			c.Args = utils.InsertSliceItem(c.Args, v, 0)
		}
	}

	// get function type
	funcTyp, ok := ssa.ToFunctionType(c.Method.GetType())
	if !ok {
		return
	}

	// handle FreeValue
	if funcTyp.FreeValue != nil {
		c.HandleFreeValue(funcTyp.FreeValue)
	}

	// handle ellipsis, unpack argument
	if c.IsEllipsis {
		// getField := func(object ssa.User, key ssa.Value) *ssa.Field {
		// 	var f *ssa.Field
		// 	if f = ssa.GetField(object, key); f == nil {
		// 		f = ssa.NewFieldOnly(key, object, c.Block)
		// 		ssa.EmitBefore(c, f)
		// 	}
		// 	return f
		// }
		// obj := c.Args[len(c.Args)-1].(ssa.User)
		// num := len(ssa.GetFields(obj))
		// if t, ok := obj.GetType().(*ssa.ObjectType); ok {
		// 	if t.Kind == ssa.Slice {
		// 		num = len(t.Key)
		// 	}
		// }

		// // fields := ssa.GetFields(obj)
		// c.Args[len(c.Args)-1] = getField(obj, ssa.NewConst(0))
		// for i := 1; i < num; i++ {
		// 	c.Args = append(c.Args, getField(obj, ssa.NewConst(i)))
		// }
	}

	// inference call instruction type
	if c.IsDropError {
		if t, ok := funcTyp.ReturnType.(*ssa.ObjectType); ok {
			if t.Combination && t.FieldTypes[len(t.FieldTypes)-1].GetTypeKind() == ssa.ErrorType {
				// if len(t.FieldTypes) == 1 {
				// 	c.SetType(ssa.BasicTypes[ssa.Null])
				// } else if len(t.FieldTypes) == 2 {
				if len(t.FieldTypes) == 2 {
					c.SetType(t.FieldTypes[0])
				} else {
					ret := ssa.NewStructType()
					ret.FieldTypes = t.FieldTypes[:len(t.FieldTypes)-1]
					ret.Key = t.Key[:len(t.Key)-1]
					ret.KeyTyp = t.KeyTyp
					ret.Combination = true
					c.SetType(ret)
				}
				return
			}
		} else if t, ok := funcTyp.ReturnType.(*ssa.BasicType); ok && t.Kind == ssa.ErrorType {
			// pass
			c.SetType(ssa.BasicTypes[ssa.Null])
			c.GetFunc().NewErrorWithPos(ssa.Error, TITAG, c.GetLeftPosition(), ValueIsNull())
			return
		}
		c.NewError(ssa.Error, TITAG, FunctionContReturnError())
	} else {
		c.SetType(funcTyp.ReturnType)
	}
}
