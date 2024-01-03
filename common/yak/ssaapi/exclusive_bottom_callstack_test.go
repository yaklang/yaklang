package ssaapi

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestYaklangExplore_BottomUses_BasicCallStack(t *testing.T) {
	prog, err := Parse(`var a = 1;

b = i => i+1

c = b(a)
e = c+1

sink = i => {
	println(i)
}

sink(e)
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	refName := "a"

	var foundDeepSink bool = false

	prog.Ref(refName).ForEach(func(value *Value) {
		log.Infof("%v: %s", refName, value.String())
		value.GetBottomUses().ForEach(func(value *Value) {
			log.Infof("%v Bottom Uses: %s", refName, value.String())
			if strings.Contains(value.String(), "println(") {
				foundDeepSink = true
			}
		})
	})
	_ = foundDeepSink
	// prog.Program.Show()
	// if !foundDeepSink {
	// 	t.Error("deep callstack sink check failed")
	// }
}

func TestYaklangExplore_BottomUses_Bad_ConstCollapsed(t *testing.T) {
	prog, err := Parse(`var c = 1
var a = 55 + c
myFunctionName(a)`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	refName := "c"

	var foundMyFunctionName bool = false

	prog.Ref(refName).ForEach(func(value *Value) {
		log.Infof("%v: %s", refName, value.String())
		value.GetBottomUses().ForEach(func(value *Value) {
			log.Infof("%v Bottom Uses: %s", refName, value.String())
			if strings.Contains(value.String(), "myFunctionName(") {
				foundMyFunctionName = true
			}
		})
	})
	prog.Program.Show()
	if !foundMyFunctionName {
		t.Error("foundMyFunctionName check failed")
	}
}

func TestYaklangExplore_BottomUses_Assign(t *testing.T) {
	prog, err := Parse(`var c = bbb
var a = 55 + c
myFunctionName(a)`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	refName := "c"

	var foundMyFunctionName bool = false

	prog.Ref(refName).ForEach(func(value *Value) {
		log.Infof("%v: %s", refName, value.String())
		value.GetBottomUses().ForEach(func(value *Value) {
			log.Infof("%v Bottom Uses: %s", refName, value.String())
			if strings.Contains(value.String(), "myFunctionName(") {
				foundMyFunctionName = true
			}
		})
	})
	prog.Program.Show()
	if !foundMyFunctionName {
		t.Error("foundMyFunctionName check failed")
	}
}

func TestYaklangExplore_BottomUses_1(t *testing.T) {
	prog, err := Parse(`var c
var a = 1
if cond {
	a = c + 2
} else {
	a = c + 3
}

d = a;
myFunctionName(d)`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	refName := "c"

	var foundMyFunctionName bool = false

	prog.Ref(refName).ForEach(func(value *Value) {
		log.Infof("%v: %s", refName, value.String())
		value.GetBottomUses().ForEach(func(value *Value) {
			log.Infof("%v Bottom Uses: %s", refName, value.String())
			if strings.Contains(value.String(), "myFunctionName(") {
				foundMyFunctionName = true
			}
		})
	})
	prog.Program.Show()
	if !foundMyFunctionName {
		t.Error("foundMyFunctionName check failed")
	}
}
