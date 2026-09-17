package yakvm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"
)

type CoreCacheReceiver struct{ N int }

func (r CoreCacheReceiver) Read() int { return r.N }

func TestCoreMethodCacheColdAndBudget(t *testing.T) {
	if os.Getenv("YAKVM_METHOD_CACHE_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoreMethodCacheColdAndBudget$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YAKVM_METHOD_CACHE_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated cache test: %v\n%s", err, out)
		}
		return
	}
	// Use a subprocess rather than resetting shared package cache state.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got := nativeMethodByName(reflect.ValueOf(CoreCacheReceiver{i}), "Read").Call(nil)[0].Int()
			if got != int64(i) {
				t.Errorf("cold receiver mismatch: %d", got)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if nativeMethodTypeCount.Load() != 1 {
		t.Fatalf("duplicate cold reservations retained: %d", nativeMethodTypeCount.Load())
	}
	for i := 0; i < nativeMethodTypeLimit+4; i++ {
		typ := reflect.StructOf([]reflect.StructField{
			{Name: "CoreCacheReceiver", Type: reflect.TypeOf(CoreCacheReceiver{}), Anonymous: true},
			{Name: "Pad", Type: reflect.ArrayOf(i, reflect.TypeOf(byte(0)))},
		})
		r := reflect.New(typ).Elem()
		r.Field(0).Set(reflect.ValueOf(CoreCacheReceiver{i}))
		got, want := nativeMethodByName(r, "Read"), r.MethodByName("Read")
		if got.Call(nil)[0].Int() != want.Call(nil)[0].Int() {
			t.Fatalf("fallback changed receiver %d", i)
		}
	}
	if nativeMethodTypeCount.Load() != nativeMethodTypeLimit {
		t.Fatal("type budget exceeded")
	}
	r := reflect.ValueOf(CoreCacheReceiver{7})
	for i := 0; i < 1000; i++ {
		if nativeMethodByName(r, fmt.Sprintf("missing%d", i)).IsValid() {
			t.Fatal("unknown method found")
		}
	}
	entries, _ := nativeMethodTypes.Load(r.Type())
	if len(entries.(map[string]int)) != 1 {
		t.Fatal("arbitrary miss names grew cache")
	}
}
