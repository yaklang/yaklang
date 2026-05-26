package test

import (
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestErrorPropagationViaReturn(t *testing.T) {
	t.Run("wrapper returns single error - no report inside wrapper", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			getError = func() {
				return getError1()
			}
			`,
			Want: []string{},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
			},
		})
	})

	t.Run("caller discards wrapper return - no report", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			getError = func() {
				return getError1()
			}
			getError()
			`,
			Want: []string{},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
			},
		})
	})

	t.Run("caller assigns wrapper error without handling", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			getError = func() {
				return getError1()
			}
			err = getError()
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
			},
		})
	})

	t.Run("closure returns multi-value with error - no report inside wrapper", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			wrapHTTP = func() {
				return getError2()
			}
			`,
			Want: []string{},
			ExternValue: map[string]any{
				"getError2": func() (int, error) { return 1, errors.New("err") },
			},
		})
	})

	t.Run("closure like sendPacket0 returns mockHTTP - no report inside", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			sendPacket0 = func(target, payload) {
				return mockHTTP(target, payload)
			}
			`,
			Want: []string{},
			ExternValue: map[string]any{
				"mockHTTP": func(any, any) (any, error) { return nil, errors.New("err") },
			},
		})
	})

	t.Run("caller assigns multi-value from wrapper without handling err", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			sendPacket0 = func(target, payload) {
				return mockHTTP(target, payload)
			}
			rsp, err = sendPacket0("a", "b")
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{
				"mockHTTP": func(any, any) (any, error) { return nil, errors.New("err") },
			},
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
