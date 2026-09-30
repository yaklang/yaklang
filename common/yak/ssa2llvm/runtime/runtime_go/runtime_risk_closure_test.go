package main

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func TestRuntimeMapClosureBecomesGoFunc(t *testing.T) {
	fn, hits := runtimeTestClosureProbe()
	om := newRuntimeOrderedMap()
	om.Set("do", runtimeCallableClosure{fn: fn})
	om.Set("n", int64(2))
	raw := uint64(uintptr(newStdlibShadow(om))) | yakTaggedPointerMask
	got, err := runtimeDecodeArg(raw, reflect.TypeOf(map[string]any{}))
	if err != nil {
		t.Fatalf("decode map: %v", err)
	}
	m, ok := got.Interface().(map[string]any)
	if !ok {
		t.Fatalf("decoded map type %T", got.Interface())
	}
	if m["n"] != int64(2) {
		t.Fatalf("n = %#v", m["n"])
	}
	call, ok := m["do"].(func(...any) any)
	if !ok {
		t.Fatalf("do = %T, want func(...any) any", m["do"])
	}
	before := hits()
	call(2)
	if hits() <= before {
		t.Fatalf("closure hits stayed %d", before)
	}
}

func TestRuntimeRiskClosureCallbackIsInvoked(t *testing.T) {
	previous := yaklib.GetYakitClientInstance()
	yaklib.InitYakit(nil)
	t.Cleanup(func() { yaklib.InitYakit(previous) })
	yak_register_module_risk()

	fn, hits := runtimeTestClosureProbe()
	before := hits()
	closure := runtimeCallableClosure{
		fn:               fn,
		paramMemberCount: 0,
		freeValues:       []uint64{1, 1},
	}
	raw := uint64(uintptr(newRuntimeShadow(closure)))
	if raw == 0 {
		t.Fatal("closure shadow is 0")
	}
	pkg, pkgBuf := pinnedCString("risk")
	register, registerBuf := pinnedCString("RegisterBeforeRiskSave")
	defer runtime.KeepAlive(pkgBuf)
	defer runtime.KeepAlive(registerBuf)
	if _, err := runtimeDispatchYaklibCall([]uint64{pkg, register, raw | yakTaggedPointerMask}, false); err != nil {
		t.Fatalf("RegisterBeforeRiskSave: %v", err)
	}

	titleName, titleBuf := pinnedCString("title")
	argNo, argBuf := pinnedCString("no")
	defer runtime.KeepAlive(titleBuf)
	defer runtime.KeepAlive(argBuf)
	ret, err := runtimeDispatchYaklibCall([]uint64{pkg, titleName, argNo | yakTaggedPointerMask}, false)
	if err != nil || ret == 0 {
		t.Fatalf("risk.title ret=%#x err=%v", uint64(ret), err)
	}
	newRisk, newRiskBuf := pinnedCString("NewRisk")
	host, hostBuf := pinnedCString("127.0.0.1:111")
	defer runtime.KeepAlive(newRiskBuf)
	defer runtime.KeepAlive(hostBuf)
	if _, err := runtimeDispatchYaklibCall([]uint64{
		pkg,
		newRisk,
		host | yakTaggedPointerMask,
		uint64(ret) | yakTaggedPointerMask,
	}, false); err != nil {
		t.Fatalf("risk.NewRisk: %v", err)
	}
	if got := hits(); got <= before {
		t.Fatalf("closure callback hits stayed %d", got)
	}
}
