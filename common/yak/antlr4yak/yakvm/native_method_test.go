package yakvm

import (
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type methodCacheReceiver struct{ N int }

func (r *methodCacheReceiver) Read() int {
	if r == nil {
		return -1
	}
	return r.N
}
func (r *methodCacheReceiver) Write(n int) { r.N = n }
func (r *methodCacheReceiver) hidden()     {}

type methodCacheEmbedded struct{ *methodCacheReceiver }

func TestNativeMethodCache(t *testing.T) {
	for _, value := range []any{&methodCacheReceiver{1}, &methodCacheReceiver{2}, (*methodCacheReceiver)(nil), methodCacheEmbedded{&methodCacheReceiver{3}}, map[string]int{}, 1} {
		receiver := reflect.ValueOf(value)
		for _, name := range []string{"Read", "Write", "Missing", "hidden", ""} {
			want := receiver.MethodByName(name)
			got := nativeMethodByName(receiver, name)
			require.Equal(t, want.IsValid(), got.IsValid())
			if got.IsValid() {
				require.Equal(t, want.Type(), got.Type())
				if name == "Read" {
					require.Equal(t, want.Call(nil)[0].Int(), got.Call(nil)[0].Int())
				}
			}
		}
	}
	// A saved method must bind its actual receiver, and observe later mutation.
	a, b := &methodCacheReceiver{1}, &methodCacheReceiver{2}
	fa := nativeMethodByName(reflect.ValueOf(a), "Read").Interface().(func() int)
	fb := nativeMethodByName(reflect.ValueOf(b), "Read").Interface().(func() int)
	a.N = 9
	require.Equal(t, 9, fa())
	require.Equal(t, 2, fb())
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := reflect.ValueOf(&methodCacheReceiver{i})
			for n := 0; n < 100; n++ {
				if got := nativeMethodByName(r, "Read").Call(nil)[0].Int(); got != int64(i) {
					t.Errorf("receiver leaked: %d != %d", got, i)
				}
			}
		}(i)
	}
	wg.Wait()
}

var nativeMethodBenchmarkResult any

func BenchmarkNativeMethodLookup(b *testing.B) {
	r := reflect.ValueOf(&methodCacheReceiver{7})
	for _, cached := range []bool{false, true} {
		name := "reflect"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var method reflect.Value
				if cached {
					method = nativeMethodByName(r, "Read")
				} else {
					method = r.MethodByName("Read")
				}
				nativeMethodBenchmarkResult = method.Interface()
			}
		})
	}
}
