package ssa

import "github.com/yaklang/yaklang/common/utils"

func (b *FunctionBuilder) CreateObjectWithSlice(vs []Value) *Make {
	return b.BuildObjectAddFieldBuild(len(vs),
		func(i int) Value { return b.EmitConstInst(i) },
		func(i int) Value { return vs[i] },
	)
}

func (b *FunctionBuilder) CreateObjectWithMap(keys []Value, vs []Value) *Make {
	return b.BuildObjectAddFieldBuild(len(vs),
		func(i int) Value { return keys[i] },
		func(i int) Value { return vs[i] },
	)
}

func (b *FunctionBuilder) BuildObjectAddFieldBuild(size int, keys func(int) Value, value func(int) Value) *Make {
	// lValueLen := NewConst(size)
	var lValueLen Value = nil
	itf := b.EmitMakeWithoutType(lValueLen, lValueLen)
	if utils.IsNil(itf) {
		return nil
	}
	if b.MarkedVariable != nil {
		itf.SetName(b.MarkedVariable.GetName())
		b.MarkedThisObject = itf
		defer func() {
			b.MarkedThisObject = nil
		}()
	}
	ityp := NewObjectType()
	itf.SetType(ityp)
	for i := 0; i < size; i++ {
		key := keys(i)
		value := value(i)
		v := b.CreateMemberCallVariable(itf, key)
		b.AssignVariable(v, value)
	}
	ityp.Finish()
	// ityp.Len = len(vs)
	return itf
}
