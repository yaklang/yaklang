package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNamespace(t *testing.T) {
	// TODO: this php namespace bug will fixup in: https://github.com/yaklang/yaklang/pull/1911
	t.Skip()
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
			ssaapi.WithLanguage(ssaapi.PHP))
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
			ssaapi.WithLanguage(ssaapi.PHP))
		//ssatest.CheckSyntaxFlow(t, code,
		//	`println(* #-> * as $param)`,
		//	map[string][]string{"param": {"1"}},
		//	ssaapi.WithLanguage(ssaapi.PHP))
	})
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
			ssaapi.WithLanguage(ssaapi.PHP))
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
	t.Run("namepsace references each other", func(t *testing.T) {
		code := `<?php

namespace a {

    use function b\aa;

    function t($a)
    {
        return $a;
    }

    function b($b)
    {
        return aa($b);
    }
}

namespace b {

    use function a\t;

    function aa($c)
    {
        return t($c);
    }
}

namespace {
    $a = a\b(1);
    println($a);
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
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
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	//t.Run("namespace function call", func(t *testing.T) {
	//	code := `<?php
	//
	//namespace a\b\c\d {
	//   class t
	//   {
	//       public static $abc = 1;
	//   }
	//
	//   function test($a)
	//   {
	//       return $a;
	//   }
	//}
	//
	//namespace {
	//   $a = \a\b\c\d\test(\a\b\c\d\t::$abc);
	//   println($a);
	//}
	//`
	//	ssatest.CheckPrintlnValue(code, []string{}, t)
	//})
}
