package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func TestFunctionCallTypeCheck(t *testing.T) {
	t.Run("normal function", func(t *testing.T) {
		check(t, `
		codec.DecodeBase64(1)
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "codec.DecodeBase64"),
		})
	})

	t.Run("variadic function call", func(t *testing.T) {
		check(t, `
		ssa.Parse(1)
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "ssa.Parse"),
		})
	})

	//TODO: check this parameter type
	t.Run("variadic function call, error type in variadic parament", func(t *testing.T) {
		check(t, `
		ssa.Parse("a", 1)
		`, []string{})
	})

	t.Run("variadic function call, error type both", func(t *testing.T) {
		check(t, `
		ssa.Parse(1, 1) 
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "ssa.Parse"),
		})
	})
}

func TestFunctionCallParameterLength(t *testing.T) {
	t.Run("normal function", func(t *testing.T) {
		check(t, `
		codec.DecodeBase64()
		`, []string{
			ssa4analyze.NotEnoughArgument("codec.DecodeBase64", "", "string"),
		})
	})

	t.Run("variadic function call, not enough min length", func(t *testing.T) {
		check(t, `
		ssa.Parse()
		`, []string{
			ssa4analyze.NotEnoughArgument("ssa.Parse", "", "string, ...ssaapi.Option"),
		})
	})

	t.Run("variadic function call", func(t *testing.T) {
		check(t, `
		ssa.Parse("a")
		`, []string{})
	})

	t.Run("variadic function call, has more parament", func(t *testing.T) {
		check(t, `
		ssa.Parse("a", ssa.withLanguage(ssa.Javascript))
		`, []string{})
	})

	t.Run("variadic function call, has ellipsis", func(t *testing.T) {
		check(t, `
		opt = [ssa.withLanguage(ssa.Javascript)]
		ssa.Parse("a", opt...)
		`, []string{})
	})

	t.Run("no-variadic function call, but has ellipsis", func(t *testing.T) {
		check(t, `
		a = ["a", "b"]
		codec.DecodeBase64(a...)
		`, []string{
			ssa4analyze.NotEnoughArgument("codec.DecodeBase64", "[]string", "string"),
		})
	})
}

func TestMakeByte(t *testing.T) {
	t.Run("make byte", func(t *testing.T) {
		check(t, `
		a = make([]byte, 1)
		`, []string{})
	})
	t.Run("make slice, without size cap", func(t *testing.T) {
		check(t, `
		make([]int)`, []string{})
	})
	t.Run("make slice, without cap ", func(t *testing.T) {
		check(t, `
		make([]int, 1)`, []string{})
	})
	t.Run("make slice", func(t *testing.T) {
		check(t, `
		make([]int, 1, 1)`, []string{})
	})
	t.Run("make slice more argument", func(t *testing.T) {
		check(t, `
		make([]int, 1, 1, 3)`, []string{
			yak2ssa.MakeArgumentTooMuch("slice"),
		})
	})

	t.Run("make chan more argument", func(t *testing.T) {
		check(t, `
		make(chan int, 1, 3)`, []string{
			yak2ssa.MakeArgumentTooMuch("chan"),
		})
	})
}
