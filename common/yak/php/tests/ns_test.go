package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	t.Run("namespace variables assign and read both in unname", func(t *testing.T) {
		code := `<?php
namespace {
	$a = 1;
}

namespace{
	println($a);
}`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("namespace variables", func(t *testing.T) {
		code := `<?php
namespace test{
	$a = 1;
	println($a);
}

namespace{
	println($a);
}`
		ssatest.CheckPrintlnValue(code, []string{"1", "1"}, t)
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
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("decl function", func(t *testing.T) {
		code := `<?php

namespace a\b {
    class test
    {
        public $a = 1;
		public function setA($a){
			$this->a=$a;
		}
    }
}
namespace {
    
    $a = new a\b\test();
	$a->setA(1);
    println($a->a);
}`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			ssaapi.WithLanguage(ssaconfig.PHP))
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
    //println($a::$a);
}`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		},
			ssaapi.WithLanguage(ssaconfig.PHP))
		//ssatest.CheckSyntaxFlow(t, code,
		//	`println(* #-> * as $param)`,
		//	map[string][]string{"param": {"1"}},
		//	ssaapi.WithLanguage(ssaconfig.PHP))
	})
	//todo:
	t.Run("use namespace", func(t *testing.T) {
		code := `<?php

namespace a\b\c {
    class test
    {
        public $a;

        public function getA()
        {
            return 1;
        }
    }
}

namespace {

    use \a\b\c;

    $a = new c\test();
    println($a->getA());
}
`
		//ssatest.MockSSA(t, code)
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})

	t.Run("more namespace", func(t *testing.T) {
		t.SkipNow()
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
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("namespace references each other", func(t *testing.T) {
		code := `<?php

namespace a {

    use function b\aa;

    function t()
    {
        return 1;
    }

    function b()
    {
        return aa();
    }
}

namespace b {

    use function a\t;

    function aa()
    {
        return t();
    }
	function bb(){
	}
}

namespace {
    $a = a\b();
    println($a);
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("namespace references each other1", func(t *testing.T) {
		code := `<?php

namespace a {

    use function b\aa;

    function t()
    {
        return 1;
    }

    function b()
    {
        return aa();
    }
}

namespace b {

    use function a\t;

    function aa()
    {
        return bb();
    }
	function bb(){
		return 2;
	}
}

namespace {
    $a = a\b();
    println($a);
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"2"})
	})
	t.Run("all namespace use static member", func(t *testing.T) {
		code := `<?php
	
	namespace a\b\c {
	 class test
	 {
	     public static $a = 1;
	 }
	}
	
	namespace {
	 println(\a\b\c\test::$a);
	}`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("test no namespace", func(t *testing.T) {
		code := `<?php

namespace{
    function a(){
        return 1;
    }
    println(a());
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("test namespace not use", func(t *testing.T) {
		code := `<?php

namespace aa\b{
    function a(){
        return 2;
    }
}
namespace{
    function a(){
        return 1;
    }
    println(a());
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("test namespace lib", func(t *testing.T) {
		code := `<?php

namespace A\B\C{
    class test{
        
    }
}
namespace B\C\D{
    class B{
        
    }
}
namespace B\C\D\E{
	use A\B\C\test;
    $t = new test();
    println($t->a);
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(*<fullTypeName> as $param)`, map[string][]string{
			"param": {"\"A.B.C.test\""},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
func TestMember(t *testing.T) {
	code := `<?php
class A{
	public function aa(){
		$this->a = 1;
	}
}
$a = new A();
$a->aa();
println($a->a);
`
	ssatest.CheckSyntaxFlow(t, code,
		`println(* #-> as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
}
func TestCheckObjectExist(t *testing.T) {
	code := `<?php

class Generate extends Backend
{
    public function initialize()
    {
        $this->service = new GenerateService();
    }
	public function generate()
    {
		$param = 1111;
		return $this->service->generate($param);
    }
}
class GenerateService{
	public function generate($param){
		println($param);
	}
}
$a = new Generate();
$a->initialize();
$a->generate();
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1111"},
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
