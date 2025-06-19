package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestYaklangMask(t *testing.T) {
	p, err := ssaapi.Parse(`
var a = 3
b = () => {
	a ++
}
if c {
	b()
}
e = a
`) // .Show()
	if err != nil {
		t.Fatal("prog ssaapi.Parse error", err)
	}

	p.Ref("e").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})

}

func TestYakChanExplore_SideEffect_SelfAdd(t *testing.T) {
	prog, err := ssaapi.Parse(`
originValue = 4
b = ()=>{
	originValue++
}
b()
g = originValue
`)
	if err != nil {
		t.Fatal("prog ssaapi.Parse error", err)
	}

	/*
		[INFO] 2023-12-19 17:23:47 [exclusive_op_test:22] g value: 4
		(ssaapi.Values) (len=1 cap=1) Values: 1
			0: ConstInst: 4

	*/
	prog.Ref("originValue").ForEach(func(value *ssaapi.Value) {
		log.Infof("originValue value[%v]: %v", value.GetId(), value.String())
	})

	check1 := false
	check4 := false
	// g not phi
	prog.Ref("g").ForEach(func(value *ssaapi.Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]
		value.GetTopDefs().ShowWithSource().ForEach(func(value *ssaapi.Value) {
			if value.GetConstValue() == 1 {
				check1 = true
			}
			if value.GetConstValue() == 4 {
				check4 = true
			}

			value.Show()
		})
	})
	if !check1 {
		t.Error("check1 failed, side-effect failed")
	}
	if !check4 {
		t.Error("check4 failed, side-effect failed")
	}
}

func TestYakChanExplore_SideEffect(t *testing.T) {
	prog, err := ssaapi.Parse(`
originValue = 4
b = ()=>{
	originValue = 5
}
b()
g = originValue
`) // .Show()
	if err != nil {
		t.Fatal("prog ssaapi.Parse error", err)
	}

	/*
		[INFO] 2023-12-19 17:23:47 [exclusive_op_test:22] g value: 4
		(ssaapi.Values) (len=1 cap=1) Values: 1
			0: ConstInst: 4

	*/
	check5 := false
	check4 := false

	// g not phi
	prog.Ref("g").ForEach(func(value *ssaapi.Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			if value.GetConstValue() == 5 {
				check5 = true
			}
		})
	})
	if !check5 {
		t.Error("check5 failed, side-effect failed")
	}
	_ = check4
}

func TestMask_Rough(t *testing.T) {
	prog, err := ssaapi.Parse(`
var a=222;
c = () => {a = 333}
if b {c()}
dump(a)
`)
	if err != nil {
		t.Fatal(err)
	}

	check222 := false
	check333 := false
	masked := prog.Ref("a").ForEach(func(value *ssaapi.Value) {
		ins := value.GetSSAInst()
		_ = ins
		ins.GetName()
	}).Get(0).GetTopDefs().Show().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == 222 {
			check222 = true
		}
		if value.GetConstValue() == 333 {
			check333 = true
		}
	})
	_ = masked
	if !check222 {
		t.Error("check222 failed")
	}
	if !check333 {
		t.Error("check333 failed")
	}
}
