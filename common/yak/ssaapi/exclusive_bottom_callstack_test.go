package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"
)

func TestYaklangExplore_BottomUses_BasicCallStack(t *testing.T) {
	prog := Parse(`var a = 1;

b = i => i+1

c = b(a)
e = c+1

sink = i => {
	println(i)
}

sink(e)
`)

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
	prog.Program.Show()
	if !foundDeepSink {
		t.Error("deep callstack sink check failed")
	}
}

func TestYaklangExplore_BottomUses_1(t *testing.T) {
	prog := Parse(`var c = 1
var a = 1
if cond {
	a = c + 2
} else {
	a = c + 3
}

d = a;
myFunctionName(d)`)

	refName := "c"

	var foundMyFunctionName bool = false

	prog.Ref(refName).ForEach(func(value *Value) {
		log.Infof("%v: %s", refName, value.String())
		value.GetBottomUses().ForEach(func(value *Value) {
			log.Infof("%v Bottom Uses: %s", refName, value.String())
			if value.node.GetName() == "myFunctionName" {
				foundMyFunctionName = true
			}
		})
	})
	prog.Program.Show()
	if !foundMyFunctionName {
		t.Error("foundMyFunctionName check failed")
	}
}
