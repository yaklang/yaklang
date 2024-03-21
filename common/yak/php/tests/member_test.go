package tests

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestParseSSA_BasicMember(t *testing.T) {
	code := `<?php
$c=[1,2,3];
var_dump($c[2]);
println($c[2]);
`
	test.CheckPrintlnValue(code, []string{"3"}, t)
}

func TestParseSSA_BasicMember2(t *testing.T) {
	code := `<?php
$b[1]=1;
println($b[1]);`
	test.CheckPrintlnValue(code, []string{"1"}, t)
}
