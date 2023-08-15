package ssa

import (
	"fmt"
	"go/types"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func ParseInterfaceTypes(vs []Value) Types {
	structType := NewStructType()
	for i, v := range vs {
		typs := v.GetType()
		structType.AddField(NewConst(i), typs)
	}
	return Types{structType.Transform()}
}

func ParseTypesFromValues(vs []Value) Types {
	typs := make(Types, 0, len(vs))
	tmp := map[Type]struct{}{}
	for _, v := range vs {
		for _, typ := range v.GetType() {
			if _, ok := tmp[typ]; !ok {
				tmp[typ] = struct{}{}
				typs = append(typs, typ)
			}
		}
	}
	return typs
}

func (phi *Phi) InferenceType() {
	org := phi.typs
	typs := ParseTypesFromValues(phi.Edge)
	phi.typs = typs

	if org.Compare(typs) {
		for _, user := range phi.GetUsers() {
			user.InferenceType()
		}
	}
}

func (i *If) InferenceType() {
	cond := i.Cond

	// type check
	condtyp := cond.GetType()
	if len(condtyp) == 0 {
		fmt.Printf("warn: if cond type is nil\n")
	} else if len(condtyp) == 1 {
		if cond.GetType()[0] != basicTypesKind[Bool] {
			fmt.Printf("warn: if condition must be bool\n")
		}
	} else {
		// handler if multiple possible type
	}

}

func (r *Return) InferenceType() {
	if len(r.Results) == 0 {
		r.typs = nil
	} else if len(r.Results) == 1 {
		// type is this result
		r.typs = r.Results[0].GetType()
	} else {
		// multiple, make a interface_struct
		structType := NewStructType()
		for i, r := range r.Results {
			structType.AddField(NewConst(i), r.GetType())
		}
		r.typs = Types{structType.Transform()}

	}
}

func (c *Call) InferenceType() {
	org := c.typs
	var typs Types
	switch inst := c.Method.(type) {
	case *Field:
	case *Call:
	case *Function:
		ret := inst.Return
		if len(ret) == 0 {
			fmt.Printf("warn: function %s return type is nil\n", inst.name)
			typs = []Type{types.Typ[types.UntypedNil]}
		} else if len(ret) == 1 {
			typs = inst.Return[0].GetType()
		} else {
			//TODO: multiple return
		}

	default:
	}
	if len(typs) == 0 {
		typs = org
	}
	c.typs = typs
	if org.Compare(typs) {
		for _, user := range c.GetUsers() {
			user.InferenceType()
		}
	}
}

func (sw *Switch) InferenceType() {

}

func (b *BinOp) InferenceType() {
	if b.Op >= yakvm.OpGt && b.Op <= yakvm.OpNotEq {
		b.typs = []Type{basicTypesKind[Bool]}
		return
	}
	org := b.typs
	typs := make(Types, 0)
	x, y := b.X.GetType(), b.Y.GetType()

	parseType := func(x, y Type) bool {
		if x.String() == y.String() {
			typs = append(typs, x)
			return true
		} else if x, ok := x.(*types.Basic); ok {
			if y, ok := y.(*types.Basic); ok {
				// x y all basic
				var max types.BasicKind
				if x.Kind() > y.Kind() {
					max = x.Kind()
				} else {
					max = y.Kind()
				}
				if max < types.Complex128 {
					typs = append(typs, types.Typ[max])
					return true
				}
			}
		}
		return false
	}

	handlerUnTyped := func(untyped, typed Value) {
		typ := typed.GetType()
		untyp := untyped.GetType()
		if len(typ) == 0 {
			if len(org) == 0 {
				fmt.Printf("warn: binop all is unknow type[]\n")
				typs = org
				return
			} else if len(org) == 1 {
				typ = append(typ, org[0])
				untyp = append(untyp, org[0])
				typed.SetType(typ)
			} else {
				// org > 1
			}
		} else if len(typ) == 1 {
			untyp = append(untyp, typ[0])
			typs = append(typs, typ[0])
		} else {
			// handler multiple type
		}
		untyped.SetType(untyp)

	}

	if len(x) == 0 {
		handlerUnTyped(b.X, b.Y)
	} else if len(y) == 0 {
		handlerUnTyped(b.Y, b.X)
	} else if len(x) == 1 && len(y) == 1 {
		parseType(x[0], y[0])
	} else if len(x) == 1 {
		for _, y := range y {
			if parseType(x[0], y) {
				break
			}
		}
	} else if len(y) == 1 {
		for _, x := range x {
			if parseType(x, y[0]) {
				break
			}
		}

	} else {
		// x > 1 && y > 1
	}

	if len(typs) == 0 {
		typs = org
	}
	b.typs = typs

	if org.Compare(typs) {
		for _, user := range b.GetUsers() {
			user.InferenceType()
		}
	}
}

func (i *Interface) InferenceType() {

}

func GetField(I Type, key Value) Type {
	switch I := I.(type) {
	case *SliceType:
		return I.Elem
	case *MapType:
		return I.Value
	case *StructType:
		return I.GetField(key)
	}

	return nil
}

func (f *Field) InferenceType() {
	org := f.GetType()
	interfacetyp := f.I.GetType()
	typs := make(Types, 0)
	if len(f.I.GetType()) == 0 {
		fmt.Printf("warn: interface type is not set\n")
	} else if len(interfacetyp) == 1 {
		typs = append(typs, GetField(interfacetyp[0], f.Key))
	} else {
		// handler interface-type and key

	}
	if len(typs) == 0 {
		typs = org
	}
	f.typs = typs

}

func (u *Update) InferenceType() {
	address := u.address
	value := u.value
	addTyp := address.GetType()
	valueTyp := value.GetType()

	if len(addTyp) == 0 && len(valueTyp) == 0 {
		fmt.Printf("warn: address and value type is not set\n")
	} else if len(addTyp) == 0 {
		if len(valueTyp) == 1 {
			addTyp = append(addTyp, valueTyp[0])
			address.SetType(addTyp)
		} else {
			// handler mutiple type
		}
	} else if len(valueTyp) == 0 {
		if len(addTyp) == 1 {
			valueTyp = append(valueTyp, addTyp[0])
			value.SetType(valueTyp)
		} else {
			// handler mutiple type
		}
	} else {
		// addtyp > 0 && valueTyp > 0
		addTyp = lo.UniqBy(append(addTyp, valueTyp...), func(t Type) string { return t.String() })
		address.SetType(addTyp)
	}
}

func CheckUpdateType(address, value []Type) {
	if len(address) == 0 || len(value) == 0 {
		// panic("unknow type")
	} else if len(address) == 1 && len(value) == 1 {
		address := address[0]
		value := value[0]
		if address == value {
			return
		} else {
			// need transform
			CheckTransForm(value, address)
		}
	}
}

func CheckTransForm(from, to Type) {
}
