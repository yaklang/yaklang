package ssaapi

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// A package-level struct field is shared state. Its top-definition must not
// depend on where the reading function sits in the file: a read compiled before
// the writing functions used to stop at the struct zero value, while the same
// read compiled last resolved to the newest write.
func TestGlobalMemberTopDefIsOrderIndependent(t *testing.T) {
	opt := []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.GO)}

	const rule = `consume(* #-> * as $target)`
	wantAllWriters := map[string][]string{
		"target": {"Parameter-a", "Parameter-b"},
	}

	code := func(body string) string {
		return "package main\n\ntype Box struct{ cmd string }\n\nvar shared Box\n\nfunc consume(any) {}\n\n" + body
	}

	t.Run("read before both writers", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func run() { consume(shared.cmd) }

func assignA(a string) { shared.cmd = a }

func assignB(b string) { shared.cmd = b }
`), rule, wantAllWriters, opt...)
	})

	t.Run("read between both writers", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func assignA(a string) { shared.cmd = a }

func run() { consume(shared.cmd) }

func assignB(b string) { shared.cmd = b }
`), rule, wantAllWriters, opt...)
	})

	t.Run("read after both writers", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func assignA(a string) { shared.cmd = a }

func assignB(b string) { shared.cmd = b }

func run() { consume(shared.cmd) }
`), rule, wantAllWriters, opt...)
	})

	// Widening is an over-approximation: the read's bound member records no
	// reading function, so a write made later in the same function is reported
	// as well. The answer stays independent of compile order, which is the
	// property this fix has to guarantee.
	t.Run("same-function writes are part of the shared field", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func run() {
	consume(shared.cmd)
	shared.cmd = "local"
}

func assign(c string) { shared.cmd = c }
`), rule, map[string][]string{
			"target": {`"local"`, "Parameter-c"},
		}, opt...)
	})

	t.Run("same-function write before the read", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func run() {
	shared.cmd = "local"
	consume(shared.cmd)
}

func assign(c string) { shared.cmd = c }
`), rule, map[string][]string{
			"target": {`"local"`, "Parameter-c"},
		}, opt...)
	})

	// One writer is the ordinary shape and must stay unchanged.
	t.Run("single writer", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func run() { consume(shared.cmd) }

func assign(c string) { shared.cmd = c }
`), rule, map[string][]string{
			"target": {"Parameter-c"},
		}, opt...)
	})

	// A field only ever declared has no writer at all: the declaration value is
	// the answer and must not be dropped.
	t.Run("declared but never written", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code(`func run() { consume(shared.cmd) }
`), rule, map[string][]string{
			"target": {`""`},
		}, opt...)
	})

	// A function-local object is reached through its own call path. Sibling
	// functions that write a same-named field of another instance must not leak.
	t.Run("function-local instance does not widen", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct{ cmd string }

func consume(any) {}

func run(holder *Box) {
	consume(holder.cmd)
}

func assign(cmd string) {
	other := &Box{}
	other.cmd = cmd
}
`, rule, map[string][]string{
			"target": {"Parameter-holder"},
		}, opt...)
	})

	// Nested package-level fields: the inner field is the shared one.
	t.Run("nested package-level field", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Inner struct{ cmd string }
type Outer struct{ inner Inner }

var shared Outer

func consume(any) {}

func run() { consume(shared.inner.cmd) }

func assign(c string) { shared.inner.cmd = c }
`, rule, map[string][]string{
			"target": {"Parameter-c"},
		}, opt...)
	})
}

// The widening is bounded: a field with more writers than the fan-out limit
// keeps the ordinary resolution instead of walking every writer.
func TestGlobalMemberTopDefFanoutLimitIsBounded(t *testing.T) {
	opt := []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.GO)}

	var body string
	for i := 0; i < 40; i++ {
		body += fmt.Sprintf("func assign%d(v string) { shared.cmd = v }\n\n", i)
	}
	body += "func run() { consume(shared.cmd) }\n"

	// No assertion on the exact set: the point is that the analysis terminates
	// and still yields a bounded, non-empty answer.
	ssatest.Check(t, "package main\n\ntype Box struct{ cmd string }\n\nvar shared Box\n\nfunc consume(any) {}\n\n"+body,
		func(p *ssaapi.Program) error {
			res, err := p.SyntaxFlowWithError(`consume(* #-> * as $target)`)
			if err != nil {
				return err
			}
			if got := len(res.GetValues("target")); got == 0 {
				t.Errorf("target is empty")
			} else if got > 100 {
				t.Errorf("target is not bounded: %d", got)
			}
			return nil
		}, opt...)
}
