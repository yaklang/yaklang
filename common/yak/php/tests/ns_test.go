package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
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
	t.Run("more namespace", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("src/main/1.php", `<?php

namespace a\b\c {
	function testt(){
		return 1;
	}
    class test
    {
        public static $a;
    }
}

namespace c\b {
    class b
    {
        public $a;
    }
}

`)
		fs.AddFile("src/main/2.php", `<?php
namespace a\b\c {
    class c
    {
        public static $a;
    }
}
namespace {
    use function \a\b\c\testt;
    $a = testt();
	println($a);
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
