package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStatic(t *testing.T) {
	code := `
<?php

class A{
    public static $a =1;
}
println(A::$a);
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
func TestConstructorDataFlow(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		code := `<?php
$a = new AA(1);
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"Undefined-AA", "1", "Undefined-AA.AA-destructor", "AA"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("have constructor", func(t *testing.T) {
		code := `<?php
class A{
	public function __construct(){}
}
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Function-A.A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Function-A.A(Undefined-A))", "Undefined-A.A-destructor(Function-A.A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("no constructor", func(t *testing.T) {
		code := `<?php
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Undefined-A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Undefined-A(Undefined-A))", "Undefined-A.A-destructor(Undefined-A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestFulLTypename(t *testing.T) {
	t.Run("test no package,blueprint packageName", func(t *testing.T) {
		code := `<?php
class A{}
$a = new A();
`
		ssatest.CheckSyntaxFlow(t, code, `A() as $start;  $start<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"main.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint packageName", func(t *testing.T) {
		code := `<?php
namespace B\A\C{
class A{}
}
namespace{
	use B\A\C\A;
	$a = new A();
}
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $start;
$start<fullTypeName><show> as $end;
`, map[string][]string{
			"end": {`"B.A.C.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint member", func(t *testing.T) {
		code := `<?php

class B{

}
class A{
    public B $a;
}
$a = new A();
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $start);$start<fullTypeName><show>  as $end`, map[string][]string{
			"end": {`"main.B"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package bluePrint member not import", func(t *testing.T) {
		code := `<?php

namespace A\B\C{
    use B\C\D\B;
    class A{
        public B $a;
    }
}
namespace {
	use \A\B\C\A;
    $a = new A();
    println($a->a);
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"B.C.D.B"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint", func(t *testing.T) {
		code := `<?php
namespace {
	use B\C\D;
	class A{
		public D $a;
	}
}
$a = new A();
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"B.C.D"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})

	//todo: fix fullTypename member
	t.Run("test no import", func(t *testing.T) {
		code := `<?php
namespace A\B\C{
    class A{
        public B $a;
    }    
}
namespace {
    $a = new A();
    println($a->a);
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"main.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("parent class", func(t *testing.T) {
		code := `<?php

namespace B\C\D{
    class A{}
}
namespace A\B\C{
    use B\C\D\A;
    class BB extends A{}
}
namespace{
    use A\B\C\BB;
    $a = new BB;
    println($a);
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end;`,
			map[string][]string{
				"end": {`"A.B.C.BB"`, `"B.C.D.A"`},
			},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("anymous-class with parent2", func(t *testing.T) {
		code := `<?php


class A extends B{
}

$c= 1;
$a = new class($c) extends A{
	public function __construct($c){
        echo $c;
	}
};
println($a->AA());

class B{
    public function AA(){
        return 1;
    }
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": []string{"1"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
func TestBlueprintNativeCall(t *testing.T) {
	t.Run("test getCurrentBlueprint", func(t *testing.T) {
		code := `<?php
class B extends Think{}
class A extends B{
	public function a($c){
		echo $c;
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `a<getCurrentBlueprint><fullTypeName> as $sink`,
			map[string][]string{
				"sink": {`"main.A"`, `"main.B"`, `"main.Think"`},
			},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test getCurrent blueprint with fs", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("/var/www/html/1.php", `<?php
class A{
	public function a(){}
}
`)
		fs.AddFile("/var/www/html/2.php", `<?php
include("1.php");
class C extends A{
	public function b(){}
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `b<getCurrentBlueprint><fullTypeName> as $output`,
			map[string][]string{
				"output": {`"main.A"`, `"main.C"`},
			},
			true,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("getFunc getCurrentBlueprint", func(t *testing.T) {
		code := `<?php
class B extends Think{}
class A extends B{
	public function a($c){
		$c = "aa";
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `e"aa" as $source
$source<getFunc><getCurrentBlueprint> as $output
$output<fullTypeName> as $sink
`, map[string][]string{
			"sink": {`"main.Think"`, `"main.A"`, `"main.B"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test currentBlueprint with fs", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("/var/www/html/1.php", `<?php
namespace app\common\controller;

use app\BaseController;
use think\facade\Cookie;
class Base extends BaseController
{}
`)
		fs.AddFile("/var/www/html/2.php", `<?php
namespace app\common\controller;
class Backend extends \app\common\controller\Base{
	public function aa(){}
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `aa<getCurrentBlueprint><fullTypeName> as $output`, map[string][]string{
			"output": {"app.BaseController"},
		}, true, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
func TestStaticBlueprint(t *testing.T) {
	code := `<?php
$path = $_GET["path"];
$file = $_FILES["file"];
$savename = \think\facade\Filesystem::disk('public')->putFile($path, $file);
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`Filesystem as $obj
.disk as $method
.putFile(* #-> as $param)
`, ssaapi.QueryWithEnableDebug())
		if err != nil {
			return err
		}
		result.Show()
		require.NotEqual(t, result.GetValues("obj").Len(), 0)
		require.NotEqual(t, result.GetValues("method").Len(), 0)
		require.NotEqual(t, result.GetValues("param").Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}

func Test_MethodName_in_Syntaxflow(t *testing.T) {
	t.Run("syntaxflow method name", func(t *testing.T) {
		code := `<?php

class A{
    public function F(){
        return 1;
    }
}`
		ssatest.CheckSyntaxFlow(t, code, `
			F as $a
			A_F as $b
		
		`, map[string][]string{
			"a": {"Function-A.F"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
