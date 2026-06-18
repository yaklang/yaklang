package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPhpFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `<?php
class Box{
    public $cmd;
    public $safe;
}
function run(Box $holder){
    println($holder->cmd);
}
function assign($cmd){
    $value = new Box();
    $value->cmd = $cmd;
    $value->safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `println(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-$cmd"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `<?php
class Box{
    public $cmd;
    public $safe;
}
function run(Box $holder){
    println($holder->safe);
}
function assign($cmd){
    $value = new Box();
    $value->cmd = $cmd;
    $value->safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `println(* #-> * as $target)`, map[string][]string{
			"target": {`"safe"`},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
