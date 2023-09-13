package ssa

func IsConst(v Value) bool {
	_, ok1 := v.(*ConstInst)
	_, ok2 := v.(*Const)
	return ok1 || ok2
}

func ToConst(v Value) *Const {
	if cinst, ok := v.(*ConstInst); ok {
		return &cinst.Const
	}

	if c, ok := v.(*Const); ok {
		return c
	}

	return nil
}
