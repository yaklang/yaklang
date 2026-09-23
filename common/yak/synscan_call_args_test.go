package yak

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/synscanx"
)

func TestAppendSynScanOptionsKeepsTrailingVariadicOption(t *testing.T) {
	fn := func(targets, ports string, opts ...synscanx.SynxConfigOption) {}
	fnType := reflect.TypeOf(fn)
	seen := 0
	mark := synscanx.SynxConfigOption(func(config *synscanx.SynxConfig) { seen++ })
	args := []reflect.Value{
		reflect.ValueOf("127.0.0.1"),
		reflect.ValueOf("80"),
		reflect.ValueOf(synscanx.WithWaiting(1)),
		reflect.ValueOf(mark),
	}
	fixed := appendSynScanOptions(fnType, args, []synscanx.SynxConfigOption{
		synscanx.WithCtx(context.Background()),
	})
	if len(fixed) != 5 {
		t.Fatalf("arg count = %d, want targets, ports, runtime opt, wait, trailing opt", len(fixed))
	}
	cfg := synscanx.NewDefaultConfig()
	for _, arg := range fixed[2:] {
		arg.Interface().(synscanx.SynxConfigOption)(cfg)
	}
	if seen != 1 {
		t.Fatal("trailing synscan option was dropped")
	}
}

func TestAppendSynScanOptionsAcceptsOptionSlice(t *testing.T) {
	fn := func(targets, ports string, opts ...synscanx.SynxConfigOption) {}
	seen := 0
	mark := synscanx.SynxConfigOption(func(config *synscanx.SynxConfig) { seen++ })
	args := []reflect.Value{
		reflect.ValueOf("127.0.0.1"),
		reflect.ValueOf("80"),
		reflect.ValueOf([]synscanx.SynxConfigOption{mark}),
	}
	fixed := appendSynScanOptions(reflect.TypeOf(fn), args, nil)
	if len(fixed) != 3 {
		t.Fatalf("arg count = %d", len(fixed))
	}
	cfg := synscanx.NewDefaultConfig()
	fixed[2].Interface().(synscanx.SynxConfigOption)(cfg)
	if seen != 1 {
		t.Fatal("slice option was dropped")
	}
}
