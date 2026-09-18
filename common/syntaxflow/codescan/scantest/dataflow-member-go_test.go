package scantest

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const dataflowMemberGoRule = `consume(* #-> * as $target)`

// TestDataflowMemberGo pins the object/member dataflow behaviour for Go.
//
// Go has a closed entry point (@main) and no implicit instance sharing, so the
// only legitimate way a field value reaches a read site is through a real
// call/assignment path. These cases assert both the positive flow and the
// absence of cross-instance leaks.
func TestDataflowMemberGo(t *testing.T) {
	opt := []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.GO)}

	t.Run("field write and read in same function", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd string
}

func consume(any) {}

func run(cmd string) {
	holder := &Box{}
	holder.cmd = cmd
	consume(holder.cmd)
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-cmd"},
		}, opt...)
	})

	t.Run("field flows through real call path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd string
}

func consume(any) {}

func run(holder *Box) {
	consume(holder.cmd)
}

func assign(cmd string) {
	value := &Box{}
	value.cmd = cmd
	run(value)
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-cmd"},
		}, opt...)
	})

	// Two independent *Box instances with no call path between them. The field
	// value written in assign() must NOT leak into run()'s read site; the only
	// legitimate resolution is the parameter itself.
	t.Run("same-type instances without call path do not leak", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd  string
	safe string
}

func consume(any) {}

func run(holder *Box) {
	consume(holder.cmd)
}

func assign(cmd string) {
	value := &Box{}
	value.cmd = cmd
	value.safe = "safe"
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-holder"},
		}, opt...)
	})

	// Same field name on two unrelated structs must not cross-resolve: Go has no
	// inheritance, so a shared field name carries no data-flow meaning.
	t.Run("same field name on unrelated types do not cross-resolve", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Alpha struct {
	cmd string
}

type Beta struct {
	cmd string
}

func consume(any) {}

func run(holder *Alpha) {
	consume(holder.cmd)
}

func assign(cmd string) {
	other := &Beta{}
	other.cmd = cmd
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-holder"},
		}, opt...)
	})

	// A field value reaches the read site when the object is handed over as an
	// argument, even though the write happens in a different function.
	t.Run("field flows via pointer argument", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd string
}

func consume(any) {}

func run(holder *Box) {
	consume(holder.cmd)
}

func assign(value *Box, cmd string) {
	value.cmd = cmd
	run(value)
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-cmd"},
		}, opt...)
	})

	// A package-level variable is genuine shared state, so a write in one
	// function and a read in another must resolve.
	t.Run("field flows through package-level variable", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd string
}

var shared Box

func consume(any) {}

func run() {
	consume(shared.cmd)
}

func assign(cmd string) {
	shared.cmd = cmd
}
`, dataflowMemberGoRule, map[string][]string{
			"target": {"Parameter-cmd"},
		}, opt...)
	})
}
