package scantest

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const dataflowMemberPhpRule = `consume(* #-> * as $target)`

// TestDataflowMemberPhp pins object/member dataflow behaviour for PHP.
//
// PHP is dynamically typed and its entry points are open-ended (include,
// framework routing, dynamic dispatch), but property values must still travel a
// real call path. These cases cover same-object flow, a real call path, and the
// absence of leaks between independent instances.
func TestDataflowMemberPhp(t *testing.T) {
	opt := []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.PHP)}

	t.Run("property write and read on same object", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `<?php
class CmdBox {
    public $cmd;
}
function consume($any) {}
function run($cmd) {
    $holder = new CmdBox();
    $holder->cmd = $cmd;
    consume($holder->cmd);
}
`, dataflowMemberPhpRule, map[string][]string{
			"target": {"Parameter-$cmd"},
		}, opt...)
	})

	t.Run("property flows through real call path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `<?php
class CmdBox {
    public $cmd;
}
function consume($any) {}
function run($holder) {
    consume($holder->cmd);
}
function assign($cmd) {
    $value = new CmdBox();
    $value->cmd = $cmd;
    run($value);
}
`, dataflowMemberPhpRule, map[string][]string{
			"target": {"Parameter-$cmd"},
		}, opt...)
	})

	// No call path between assign() and run(): the property value must not leak
	// across the two independent instances.
	t.Run("instances without call path do not leak", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `<?php
class CmdBox {
    public $cmd;
}
function consume($any) {}
function run($holder) {
    consume($holder->cmd);
}
function assign($cmd) {
    $value = new CmdBox();
    $value->cmd = $cmd;
}
`, dataflowMemberPhpRule, map[string][]string{
			"target": {"Parameter-$holder"},
		}, opt...)
	})

	// Same property name on two unrelated classes must not cross-resolve.
	t.Run("same property name on unrelated classes do not cross-resolve", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `<?php
class Alpha {
    public $cmd;
}
class Beta {
    public $cmd;
}
function consume($any) {}
function run($holder) {
    consume($holder->cmd);
}
function assign($cmd) {
    $other = new Beta();
    $other->cmd = $cmd;
}
`, dataflowMemberPhpRule, map[string][]string{
			"target": {"Parameter-$holder"},
		}, opt...)
	})
}
