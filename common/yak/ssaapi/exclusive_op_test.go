package ssaapi

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"
)

func TestYakChanExplore_ForPhi(t *testing.T) {
	prog := Parse(`
i = 0
b = 3

calc = i => i;

for i < 10 {
	if f() {
		b = calc(i)	
	}
}
c = b
`)
	prog.GetValueByIdMust(0).Show()
	prog.GetValueByIdMust(1).Show()
	prog.Ref("c").ForEach(func(value *Value) {
		log.Infof("%v: %v", value.GetId(), value.String())
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

	check5 := false
	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]
		if value.GetConstValue() == 5 {
			check5 = true
		}
		defs := value.GetTopDefs()
		spew.Dump(defs)
	})
	if !check5 {
		t.Error("check5 failed, side-effect failed")
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
`).Show()

	/*
		[INFO] 2023-12-19 17:23:47 [exclusive_op_test:22] g value: 4
		(ssaapi.Values) (len=1 cap=1) Values: 1
			0: ConstInst: 4

	*/
	prog.Ref("originValue").ForEach(func(value *Value) {
		log.Infof("originValue value[%v]: %v", value.GetId(), value.String())
	})

	check5 := false
	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value[%v]: %v", value.GetId(), value.String()) // phi? why
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]
		if value.GetConstValue() == 5 {
			check5 = true
		}
		defs := value.GetTopDefs()
		spew.Dump(defs)
	})
	if !check5 {
		t.Error("check5 failed, side-effect failed")
	}
}

func TestYakChanExplore_Phi_For_Negative(t *testing.T) {
	prog := Parse(`
originValue = 1
var f = outter()
for i := 3; i < f; i++ {
	d := originValue + i
	d += f
}
g = d 
`)
	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value: %v", value.String()) // phi? why
		if strings.Contains(value.String(), "phi(d)") {
			t.Errorf("g should not be phi, but got %v", value.String())
			t.Failed()
		}
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]

		defs := value.GetTopDefs()
		spew.Dump(defs)
	})
}

func TestYakChanExplore_Phi_For_Negative_2(t *testing.T) {
	prog := Parse(`
originValue = 1
var f = outter()
for i := 3; i < f; i++ {
	d := originValue + i // var in yaklang will create new symbol
	d += f
}
g = d 
`)
	// g not phi
	prog.Ref("g").ForEach(func(value *Value) {
		log.Infof("g value: %v", value.String()) // phi? why
		if strings.Contains(value.String(), "phi(d)") {
			t.Errorf("g should not be phi, but got %v", value.String())
			t.Failed()
		}
		// g value: phi(d)[d,add(add(1, phi(i-2)[3,add(i-2, 1)]), outter())]

		defs := value.GetTopDefs()
		spew.Dump(defs)
	})
}

func TestYakChanExplore_Phi_For(t *testing.T) {
	prog := Parse(`
originValue = 1
var f = outter()
var d = 2
for i := 3; i < f; i++ {
	d = originValue + i
	d += f
}
g = d // g deps-> 1 / 2 / 3
`)
	c1 := false
	c2 := false
	c3 := false
	prog.Ref("g").ForEach(func(value *Value) {
		defs := value.GetTopDefs()
		for _, i := range defs {
			if i.GetConstValue() == 1 {
				c1 = true
			}
			if i.GetConstValue() == 2 {
				c2 = true
			}
			if i.GetConstValue() == 3 {
				c3 = true
			}
		}
	})

	if !c1 {
		t.Error("c1 check failed")
	}

	if !c2 {
		t.Error("c2 check failed")
	}

	if !c3 {
		t.Error("c3 check failed")
	}
}

func TestYakChanExplore_4(t *testing.T) {
	prog := Parse(`
originValue = 1
var f = outter()
a = e => {
	return e
}
if (f) {
	d = 3
} else {
	d = a(originValue)
}
g = d
`)
	lenCheck := false
	valCheck := false
	valCheck_eq3 := false
	prog.Ref("g").ForEach(func(value *Value) {
		defs := value.GetTopDefs()
		if len(defs) == 2 {
			lenCheck = true
		}
		if len(defs) > 0 {
			for _, def := range defs {
				log.Infof("found def: %v", def.String())
				if def.GetConstValue() == 1 {
					valCheck = true
				}
				if def.GetConstValue() == 3 {
					valCheck_eq3 = true
				}
			}
		}
	})
	if !lenCheck {
		t.Error("len check failed")
	}
	if !valCheck {
		t.Error("val check failed")
	}
	if !valCheck_eq3 {
		t.Error("val eq 3 check failed")
	}
}

func TestYakChanExplore_3(t *testing.T) {
	prog := Parse(`
originValue = 1
var f = outter()
a = e => {
	return e
}
if (f) {
	d = 3
} else {
	d = a(originValue)
}
g = d
`)
	valCheck := false
	valCheck_eq3 := false
	var topDefsCount = 0
	prog.Ref("d").ForEach(func(value *Value) {
		defs := value.GetTopDefs()
		spew.Dump(len(defs))
		topDefsCount += len(defs)
		if len(defs) > 0 {
			for _, def := range defs {
				log.Infof("found def: %v", def.String())
				if def.GetConstValue() == 1 {
					valCheck = true
				}
				if def.GetConstValue() == 3 {
					valCheck_eq3 = true
				}
			}
		}
	})
	if topDefsCount != 4 {
		t.Error("len check failed")
	}
	if !valCheck {
		t.Error("val check failed")
	}
	if !valCheck_eq3 {
		t.Error("val eq 3 check failed")
	}
}

func TestYakChanExplore_2(t *testing.T) {
	prog := Parse(`
originValue = 1
a = e => {
	return e
}
d = a(originValue)
`)
	lenCheck := false
	valCheck := false
	prog.Ref("d").ForEach(func(value *Value) {
		defs := value.GetTopDefs()
		if len(defs) == 1 {
			lenCheck = true
		}
		if defs[0].GetConstValue() == 1 {
			valCheck = true
		}
	})
	if !lenCheck {
		t.Error("len check failed")
	}
	if !valCheck {
		t.Error("val check failed")
	}
}

func TestYakChanExplore(t *testing.T) {
	prog := Parse(`
a = () => {
	var c = 1
	return c
}

d = a()
`)
	lenCheck := false
	valCheck := false
	prog.Ref("d").ForEach(func(value *Value) {
		defs := value.GetTopDefs()
		if len(defs) == 1 {
			lenCheck = true
		}
		if defs[0].GetConstValue() == 1 {
			valCheck = true
		}
	})
	if !lenCheck {
		t.Error("len check failed")
	}
	if !valCheck {
		t.Error("val check failed")
	}
}
