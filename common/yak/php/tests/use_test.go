package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestUse(t *testing.T) {
	code := `<?php

namespace a\b\c{
    const a = 1;
    function A(){
		return 1;
	}
}
namespace a\b\c\d{
    const a= 2;
}
namespace a{
    const a = 3;
}
namespace{
    use a\b\c;
    println(A());
}`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
