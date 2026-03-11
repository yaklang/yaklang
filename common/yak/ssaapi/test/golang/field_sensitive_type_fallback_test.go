package ssaapi

import (
	"testing"

	api "github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFieldSensitive_TypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

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
`, `consume(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, api.WithLanguage(ssaconfig.GO))
	})

	t.Run("negative safe field", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

type Box struct {
	cmd  string
	safe string
}

func consume(any) {}

func run(holder *Box) {
	consume(holder.safe)
}

func assign(cmd string) {
	value := &Box{}
	value.cmd = cmd
	value.safe = "safe"
}
`, `consume(* #-> * as $target)`, map[string][]string{
			"target": {`"safe"`},
		}, api.WithLanguage(ssaconfig.GO))
	})
}
