package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
		"global*<getMembers><getMembers> -{until: `* ?{opcode:call} as $sink`}-> *",
		map[string][]string{"sink": {`Function-base64_decode(Undefined-.get.func(valid))`, `add(add(Undefined-$a, Undefined-$s), Undefined-.get.func2(valid))(Function-base64_decode(Undefined-.get.func(valid)))`}},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
