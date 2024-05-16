package yakdoc

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type A struct{}

type B A

type (
	ASlice        []A
	ASlicePointer []*A
	AMap          map[string]A
	AMapPointer   map[string]*A
)

func TestGetTypeNameWithPkgPath(t *testing.T) {
	check := func(i any, expected string) {
		t.Helper()
		_, pkgPathName := GetTypeNameWithPkgPath(reflect.TypeOf(i))
		require.Equal(t, expected, pkgPathName)
	}

	t.Run("base type", func(t *testing.T) {
		check(int(1), "int")
		check("hello", "string")
		check(float64(1.1), "float64")
		check(true, "bool")
	})

	t.Run("complex base type", func(t *testing.T) {
		check([]int{}, "[]int")
		check(map[string]int{}, "map[string]int")
		check(make(chan int), "chan int")
	})

	t.Run("builtin struct and pointer", func(t *testing.T) {
		check(time.Time{}, "time.Time")
		check(&time.Time{}, "time.Time")
	})

	t.Run("alias builtin struct and pointer", func(t *testing.T) {
		check(net.IP{}, "net.IP")
		check(&net.IP{}, "net.IP")
	})

	t.Run("struct and pointer", func(t *testing.T) {
		check(A{}, "github.com/yaklang/yaklang/common/yak/yakdoc.A")
		check(&A{}, "github.com/yaklang/yaklang/common/yak/yakdoc.A")
	})

	t.Run("struct and pointer raw slice", func(t *testing.T) {
		check([]A{}, "github.com/yaklang/yaklang/common/yak/[]yakdoc.A")
		check([]*A{}, "github.com/yaklang/yaklang/common/yak/[]*yakdoc.A")
	})

	t.Run("alias struct and pointer", func(t *testing.T) {
		check(B{}, "github.com/yaklang/yaklang/common/yak/yakdoc.B")
		check(&B{}, "github.com/yaklang/yaklang/common/yak/yakdoc.B")
	})

	t.Run("alias struct and pointer slice", func(t *testing.T) {
		check(ASlice{}, "github.com/yaklang/yaklang/common/yak/yakdoc.ASlice")
		check(ASlicePointer{}, "github.com/yaklang/yaklang/common/yak/yakdoc.ASlicePointer")
	})

	t.Run("alias struct and pointer slice", func(t *testing.T) {
		check(ASlice{}, "github.com/yaklang/yaklang/common/yak/yakdoc.ASlice")
		check(ASlicePointer{}, "github.com/yaklang/yaklang/common/yak/yakdoc.ASlicePointer")
	})

	t.Run("alias struct and pointer map", func(t *testing.T) {
		check(AMap{}, "github.com/yaklang/yaklang/common/yak/yakdoc.AMap")
		check(AMapPointer{}, "github.com/yaklang/yaklang/common/yak/yakdoc.AMapPointer")
	})
}
