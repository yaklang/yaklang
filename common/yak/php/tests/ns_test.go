package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestNamespace(t *testing.T) {
	t.Run("namespace mock", func(t *testing.T) {
		code := `<?php
namespace test{
	function a(){
		echo 1;
	}	
}

namespace{
}
`
		ssatest.MockSSA(t, code)
	})
	t.Run("namespace variables", func(t *testing.T) {
		code := `<?php
namespace test{
	$a = 1;
}

namespace{
	println($a);
}`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("more namespace variable", func(t *testing.T) {
		code := `<?php
namespace test{
	$a = 1;
}
namespace test1{
	println($a);
}
namespace {}
`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("more namespace edit variable", func(t *testing.T) {
		code := `<?php
namespace test{
	$a = 1;
}
namespace test1{
	$a = 2;
}
namespace {
	println($a);
}
`
		ssatest.CheckPrintlnValue(code, []string{"2"}, t)
	})
	t.Run("usedecl", func(t *testing.T) {
		code := `<?php

namespace a\b{
    class test{
        public $a=1;
    }
}
namespace{
    use a\b\test;
    $a = new test();
    println($a->a);
}`
		ssatest.CheckPrintlnValue(code, []string{"Undefined-$a.a(valid)"}, t)
	})
	t.Run("decl function", func(t *testing.T) {
		code := `<?php

namespace a\b {
    class test
    {
        public $a = 1;
    }
}
namespace {
    
    $a = new a\b\test();
    println($a->a);
}`
		ssatest.CheckPrintlnValue(code, []string{"Undefined-$a.a(valid)"}, t)
	})
	t.Run("new class ", func(t *testing.T) {
		code := `<?php

namespace a\b {
    class test
    {
        public static $a = 1;
    }
}


namespace {
    $a = new a\b\test();
    println($a::$a);
}`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
		//ssatest.MockSSA(t, code)
	})
}
