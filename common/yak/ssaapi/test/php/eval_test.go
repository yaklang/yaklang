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
for($i=0;$i<strlen($fun);$i++){
    $fun[$i] = $fun[$i]^$key[$i+1&7];
}
$a = "a";
$s = "s";
$c=$a.$s.$_GET["func2"];
$c($fun);
`
	_ = code
	ssatest.CheckSyntaxFlow(t, code,
		"global*<getMembers><getMembers> -{include: `* ?{opcode:call}`}-> * as $func",
		map[string][]string{"func": {`Function-base64_decode(Undefined-.get.func(valid))`}},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
