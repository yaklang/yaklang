package php

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSyntaxFlowTopDef(t *testing.T) {
	code := `<?php

function test($a){
    return $a;
}

$c = 3;
if($d){
    $c = filter($c);
}else{
    $c = unsafe($c);
}
if($f){
	$c = aaa($c);
}else{
	$c = bbb($c);
}
eval($c);
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`
e"3" as $const;
eval(* #{include: <<<CODE
<self> & $const
CODE}-> as $param)
`, sfvm.WithEnableDebug())
		require.NoError(t, err)
		result.Show()
		values := result.GetValues("param")
		paths := values.GetEffectOnAllPath()
		for _, path := range paths {
			fmt.Println(path.String())
		}
		assert.True(t, len(paths) == 2)
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
