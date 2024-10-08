package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_Eval1(t *testing.T) {
	code := `<?php
$key = "password";
$fun = base64_decode($_GET['func']);
$c=$a.$s.$_GET["func2"];
$c($fun);
`
	_ = code
	ssatest.CheckSyntaxFlow(t, code,
		"_GET.* -{until: `* ?{opcode:call} as $sink`}-> *",
		map[string][]string{"sink": {"Function-base64_decode(Undefined-_GET.func(valid))", "add(add(Undefined-$a, Undefined-$s), Undefined-_GET.func2(valid))(Function-base64_decode(Undefined-_GET.func(valid)))"}},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
func TestCodeFromDb(t *testing.T) {
	code := `<?php $a = $_GET['a'];`
	library, err := sfdb.GetLibrary("php-param")
	require.NoError(t, err)
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		rule, err2 := prog.SyntaxFlowRule(library)
		require.NoError(t, err2)
		rule.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
