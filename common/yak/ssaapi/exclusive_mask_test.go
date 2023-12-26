package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestYaklangMask(t *testing.T) {
	p := Parse(`
var a = 3
b = () => {
	a ++
}
if c {
	b()
}
e = a
`) // .Show()
	p.Ref("e").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})

}

func TestYakChanExplore_SideEffect_SelfAdd(t *testing.T) {
	prog := Parse(`
originValue = 4
b = ()=>{
	originValue++
}
b()
g = originValue
`)

	/*
		[INFO] 2023-12-19 17:23:47 [exclusive_op_test:22] g value: 4
		(ssaapi.Values) (len=1 cap=1) Values: 1
			0: ConstInst: 4

	*/
	prog.Ref("originValue").ForEach(func(value *Value) {
		log.Infof("originValue value[%v]: %v", value.GetId(), value.String())
	})

	check1 := false
	check4 := false
	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]
		value.GetTopDefs().ForEach(func(value *Value) {
			if value.GetConstValue() == 1 {
				check1 = true
			}
			if value.GetConstValue() == 4 {
				check4 = true
			}
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
	prog := Parse(`
originValue = 4
b = ()=>{
	originValue = 5
}
b()
g = originValue
`) // .Show()

	/*
		[INFO] 2023-12-19 17:23:47 [exclusive_op_test:22] g value: 4
		(ssaapi.Values) (len=1 cap=1) Values: 1
			0: ConstInst: 4

	*/
	check5 := false
	check4 := false

	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		value.GetTopDefs().ForEach(func(value *Value) {
			if value.GetConstValue() == 4 {
				check4 = true
			}
			if value.GetConstValue() == 5 {
				check5 = true
			}
		})
	})
	if !check5 {
		t.Error("check5 failed, side-effect failed")
	}
	if !check4 {
		t.Error("check4 failed, side-effect failed")
	}
}
