package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
	"testing"
)

func TestYaklangBasic_Used(t *testing.T) {
	token := utils.RandStringBytes(10)
	prog := Parse(`var a, b
` + token + `(a)
`)
	traceFinished := false
	prog.Ref("a").ForEach(func(value *Value) {
		value.GetUsers().ForEach(func(value *Value) {
			log.Infof("a's uses include: %v", value.String())
			if strings.Contains(value.String(), token+"(") {
				traceFinished = true
			}
		})
	})
	if !traceFinished {
		t.Error("trace failed: var cannot trace to call actual arguments")
	}
}

func TestYaklangBasic_if_phi(t *testing.T) {
	prog := Parse(`var a, b

dump(a)

if cond {
	a = a + b
} else {
	c := 1 + b 
}
println(a)
`)
	var traceToCall_via_if bool
	prog.Ref("a").ForEach(func(value *Value) {
		if _, ok := value.node.(*ssa.Phi); ok {
			value.GetUsers().ForEach(func(value *Value) {
				if _, ok := value.node.(*ssa.Call); ok {
					traceToCall_via_if = true
					log.Infof("a's deep uses include: %v", value.String())
				}
			})
		}
	})
	if !traceToCall_via_if {
		t.Error("trace failed: var cannot trace to call actual arguments")
	}
}
