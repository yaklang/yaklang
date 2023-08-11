package ssa

import (
	"fmt"
	"go/types"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

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
		if cond.GetType()[0] != basicTypes["bool"] {
			fmt.Printf("warn: if condition must be bool\n")
		}
	} else {
		// handler if multiple possible type
	}

}

func (r *Return) InferenceType() {
	typs := ParseTypesFromValues(r.Results)
	r.typs = typs
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
		b.typs = []Type{basicTypes["bool"]}
		return
	}
	org := b.typs
	typs := make(Types, 0)
	x, y := b.X.GetType(), b.Y.GetType()

	parseType := func(x, y Type) bool {
		if x == y {
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

func (f *Field) InferenceType() {
	org := f.GetType()
	interfacetyp := f.I.GetType()
	typs := make(Types, 0)
	if len(interfacetyp) == 0 {
		fmt.Printf("warn: interface type is not set")
	} else if len(interfacetyp) == 1 {
		typ := interfacetyp[0]
		switch typ := typ.(type) {
		case *types.Slice:
			// f.typs = append(f.typs, typ.Elem())
			typs = append(typs, typ.Elem())
		case *types.Map:
		case *types.Struct:

		}

		// field.typs = append(field.typs, interfacetyp[0])
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
		fmt.Printf("warn: address and value type is not set")
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
	}

}
