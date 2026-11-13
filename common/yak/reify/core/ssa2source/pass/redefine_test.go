package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestRedefineTypeValueNeedsStringLookup(t *testing.T) {
	program := parseSSAForPass(t, `extern strcmp
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x401136
    <any> t2 = call <any> strcmp (<ptr64> t1) binding[] member[]
    ret`)
	if replaced := RedefineTypeValue(program); replaced != 0 {
		t.Fatalf("replaced %d arguments without a string lookup", replaced)
	}
}

func TestRedefineTypeValueReplacesKnownStringArgument(t *testing.T) {
	program := parseSSAForPass(t, `extern strcmp
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x401136
    <any> t2 = call <any> strcmp (<ptr64> t1) binding[] member[]
    ret`)
	call := findCallForPass(t, program, "strcmp")
	replaced := RedefineTypeValue(program, WithStringAt(func(address uint64) (string, bool) {
		if address == 0x401136 {
			return "point", true
		}
		return "", false
	}))
	if replaced != 1 {
		t.Fatalf("replaced %d arguments, want 1", replaced)
	}
	argument, ok := call.GetValueById(call.Args[0])
	if !ok || argument == nil {
		t.Fatal("replaced argument is missing")
	}
	if constant, ok := ssa.ToConstInst(argument); !ok || constant.GetRawValue() != "point" {
		t.Fatalf("argument = %#v, want point", argument)
	}
}

func TestRedefineTypeValueKeepsUnresolvedArgument(t *testing.T) {
	program := parseSSAForPass(t, `extern strcmp
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x401136
    <any> t2 = call <any> strcmp (<ptr64> t1) binding[] member[]
    ret`)
	if replaced := RedefineTypeValue(program, WithStringAt(func(uint64) (string, bool) { return "", false })); replaced != 0 {
		t.Fatalf("replaced %d unresolved arguments", replaced)
	}
}
