package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestInterfaceMethodSignatureEdgeCases(t *testing.T) {
	checkNoNotEnoughArgument := func(t *testing.T, code string, want []string) {
		test.Check(t, code, func(prog *ssaapi.Program) error {
			for _, err := range prog.GetErrors() {
				if strings.Contains(err.Message, "Not enough arguments in call") {
					return fmt.Errorf("unexpected argument-count error: %s", err.Message)
				}
			}
			return nil
		}, ssaapi.WithLanguage(ssaconfig.GO))
		test.CheckPrintlnValue(code, want, t)
	}

	t.Run("unnamed interface params", func(t *testing.T) {
		checkNoNotEnoughArgument(t, `package main

type Counter struct{ n int }

func (c *Counter) Set(v int)  { c.n = v }
func (c *Counter) Bump(v int) { c.n = c.n + v }

type Setter interface {
	Set(int)
	Bump(int)
}

func apply(s Setter) {
	s.Set(1)
	s.Bump(2)
}

func main() {
	c := &Counter{n: 0}
	apply(c)
	println(c.n)
}
`, []string{"0"})
	})

	t.Run("named interface params", func(t *testing.T) {
		checkNoNotEnoughArgument(t, `package main

type Counter struct{ n int }

func (c *Counter) Set(v int)  { c.n = v }
func (c *Counter) Bump(v int) { c.n = c.n + v }

type Setter interface {
	Set(v int)
	Bump(delta int)
}

func apply(s Setter) {
	s.Set(1)
	s.Bump(2)
}

func main() {
	c := &Counter{n: 0}
	apply(c)
	println(c.n)
}
`, []string{"0"})
	})

	t.Run("zero arg interface method", func(t *testing.T) {
		checkNoNotEnoughArgument(t, `package main

type Counter struct{ n int }

func (c *Counter) Tick()      { c.n = c.n + 1 }
func (c *Counter) Set(v int)  { c.n = v }
func (c *Counter) Bump(v int) { c.n = c.n + v }

type Setter interface {
	Tick()
	Set(int)
	Bump(int)
}

func apply(s Setter) {
	s.Tick()
	s.Set(1)
	s.Bump(2)
}

func main() {
	c := &Counter{n: 0}
	apply(c)
	println(c.n)
}
`, []string{"0"})
	})

	t.Run("variadic interface method", func(t *testing.T) {
		checkNoNotEnoughArgument(t, `package main

type Counter struct{ n int }

func (c *Counter) AddAll(vals ...int) {
	for _, v := range vals {
		c.n = c.n + v
	}
}

type Adder interface {
	AddAll(...int)
}

func apply(a Adder) {
	a.AddAll(1, 2, 3)
}

func main() {
	c := &Counter{n: 0}
	apply(c)
	println(c.n)
}
`, []string{"0"})
	})
}
