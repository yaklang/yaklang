package test

import (
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func getError2Extern() map[string]any {
	return map[string]any{
		"getError2": func() (int, error) { return 1, errors.New("err") },
	}
}

func TestErrorPropagationViaReturn(t *testing.T) {
	ext2 := getError2Extern()

	t.Run("direct return passes multi-value to caller", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				return getError2()
			}
			`,
			Want:        []string{},
			ExternValue: ext2,
		})
	})

	t.Run("unpack then return both values propagates error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				a, e = getError2()
				return a, e
			}
			`,
			Want:        []string{},
			ExternValue: ext2,
		})
	})

	t.Run("unpack then return error only propagates error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				a, e = getError2()
				return e
			}
			`,
			Want:        []string{},
			ExternValue: ext2,
		})
	})

	t.Run("unpack then return value only drops error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				a, e = getError2()
				return a
			}
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: ext2,
		})
	})

	t.Run("unpack handle error then return", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				a, e = getError2()
				if e != nil {
					return 0, e
				}
				return a, nil
			}
			`,
			Want:        []string{},
			ExternValue: ext2,
		})
	})

	t.Run("unpack without return reports inside function", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				a, e = getError2()
			}
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: ext2,
		})
	})

	t.Run("caller assigns from wrapper without handling err", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrap = func() {
				return getError2()
			}
			_, err = wrap()
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: ext2,
		})
	})

	t.Run("nested call without return still reports", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			other(getError2())
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandledWithType("number, error"),
			},
			ExternValue: map[string]any{
				"getError2": func() (int, error) { return 1, errors.New("err") },
				"other":     func(any) {},
			},
		})
	})
}
