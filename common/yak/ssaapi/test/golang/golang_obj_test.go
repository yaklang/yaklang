package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasic_BasicObject(t *testing.T) {
	ssatest.Check(t, `package main

	type t struct {
		b int
		c int
	}

	func main(){
		a := t{}; 
		a.b = 1; 
		a.c = 3; 
		d := a.c + a.b
	}
	`,
		ssatest.CheckTopDef_Contain("d", []string{"3", "1", "make("}),
		ssaapi.WithLanguage(ssaapi.GO),
	)
}	

func TestBasic_BasicObject2(t *testing.T) {
	ssatest.Check(t, `package main

	type t struct {
		b int
		c int
	}

	func f() {
		return t{}
	}
	func main(){
		a := f(); 
		a.b = 1; 
		a.c = 3; 
		d := a.c + a.b
	}
	`,
		ssatest.CheckTopDef_Contain("d", []string{"3", "1", "make("}),
		ssaapi.WithLanguage(ssaapi.GO),
	)
}	

func TestBasic_Phi(t *testing.T) {
	prog, err := ssaapi.Parse(`package main


	func main(){
		a := 0
		if (a > 0) {
			a = 1
		} else if (a > 1) {
			a = 2
		} else {
			a = 4
		}
	}
	`,
	ssaapi.WithLanguage(ssaapi.GO),
)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("a").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}