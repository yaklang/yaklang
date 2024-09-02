package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestOOP_static_member(t *testing.T) {
	t.Run("normal static member, use any", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
	<?php
class Foo {
	public static $my_static;
}

println(Foo::$my_static . PHP_EOL);

?>    
`, `println(* #-> * as $target)`,
			map[string][]string{
				"target": {"Undefined-my_static"},
			}, ssaapi.WithLanguage(ssaapi.PHP))
	})

	t.Run("normal static member,  assign again ", func(t *testing.T) {
		code := `<?php
class Foo {
	public static $my_static;
}

Foo::$my_static = "foo";
println(Foo::$my_static . PHP_EOL);

?>    
	`
		CheckPrintTopDef(t,
			code, []string{
				`"foo"`,
			})
	})

	t.Run("test phi static member", func(t *testing.T) {
		code := `
	<?php
class Foo {
	public static $my_static = "start";
}
if ($a) {
	Foo::$my_static = "foo";
}else {
	Foo::$my_static = "bar";
}
println(Foo::$my_static);
`
		CheckPrintTopDef(t, code, []string{"foo", "bar", "start"})
	})
	t.Run("string to call member", func(t *testing.T) {
		code := `<?php

class Foo
{
    public static $my_static = 'foo';
}

println("Foo"::$my_static);
?>
`
		ssatest.CheckPrintlnValue(code, []string{`phi(my_static)["foo"]`}, t)
	})
	t.Run("variable to call static member", func(t *testing.T) {
		code := `<?php

class Foo
{
    public static $my_static = 'foo';
}

$a = 'Foo';
println($a::$my_static);
?>`
		ssatest.CheckPrintlnValue(code, []string{`phi(my_static)["foo"]`}, t)
	})
	t.Run("more dollar test", func(t *testing.T) {
		code := `<?php

class Foo
{
    public static $my_static = 'foo';
}

$a = 'b';
$b = "Foo";
println($$a::$my_static);
?>`
		ssatest.CheckPrintlnValue(code, []string{`phi(my_static)["foo"]`}, t)
	})
	t.Run("Call static members across classes", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	<?php
class Foo {
	public static $my_static = 'foo';
}
?>
<?php
	class B {
		public static function test() {
			println(Foo::$my_static . PHP_EOL); // normal
			
			println("Foo"::$my_static . PHP_EOL); // string
			
			$a = "Foo";
			println($a::$my_static . PHP_EOL); // variable
			
			$b = "a";
			println($$b::$my_static . PHP_EOL); // dynamic variable
    }

	}
?>    
	`, []string{
			`add(phi(my_static)["foo"], Undefined-PHP_EOL)`,
			`add(phi(my_static)["foo"], Undefined-PHP_EOL)`,
			`add(phi(my_static)["foo"], Undefined-PHP_EOL)`,
			`add(phi(my_static)["foo"], Undefined-PHP_EOL)`,
		}, t)

	})

}

func TestOOP_static_method(t *testing.T) {
	t.Run("normal static method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class Foo {
			public static function aStaticMethod() {
				return "foo";
			}
		}
		println(Foo::aStaticMethod());
		println("Foo"::aStaticMethod());
		$a = "Foo";
		println($a::aStaticMethod());
		$b = "a";
		println($$b::aStaticMethod());
		$instance = new Foo();
		println($instance::aStaticMethod())
		?>
		`, []string{
			"Function-aStaticMethod()",
			"Function-aStaticMethod()",
			"Function-aStaticMethod()",
			"Function-aStaticMethod()",
			"Function-aStaticMethod()",
		}, t)
	})

	t.Run("static method should't assign ", func(t *testing.T) {
		code := `
		<?php
		class Foo {
			public static function aStaticMethod() {
				return "foo";
			}
		}
		Foo::aStaticMethod = "bar";
		?>
		`
		_, err := php2ssa.FrondEnd(code, false)
		require.Error(t, err)
	})

	//todo： 类型检查参数有问题
	t.Run("Call static method across classes", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
class A {
    public static function aStaticMethod() {
				return 22;
			}
}
?>
<?php
class B {
    public static function test() {
		println(A::aStaticMethod());
		println("A"::aStaticMethod());
		$a = "A";
		println($a::aStaticMethod());
		$b = "a";
		println($$b::aStaticMethod());
		$instance = new A();
		println($instance::aStaticMethod());
    }
}
?>
		`, []string{
			"Function-.$staticScope$.A.aStaticMethod()",
			"Function-.$staticScope$.A.aStaticMethod()",
			"Function-.$staticScope$.A.aStaticMethod()",
			"Function-.$staticScope$.A.aStaticMethod()",
			"Function-.$staticScope$.A.aStaticMethod()",
		}, t)

	})
}

func TestOOP_var_member(t *testing.T) {

	t.Run("normal", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class A {
			var $a = 0; 
		}
		$a = new A; 
		println($a->a);

		$b = "a";
		println($a->$b); 

		$c = "b";
		println($a->$$c);
		`, []string{
			"0", "0", "0",
		}, t)
	})

	t.Run("side effect", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class A {
			var $a = 0; 
			function setA($par){
				$this->a = $par; 
			}
		}
		$a = new A; 
		println($a->a);
		$a->setA(1);
		println($a->a);
		`, []string{
			"0", "side-effect(Parameter-$par, $this.a)",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class A {
			var $a = 0;
			function getA() {
				return $this->a;
			}
		}
		$a = new A;
		println($a->getA());
		$a->a = 1;
		println($a->getA());
		`, []string{
			"Undefined-$a.getA(valid)(Undefined-$a) member[0]",
			"Undefined-$a.getA(valid)(Undefined-$a) member[1]",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		code := `
		<?php
		class A {
			var $a = 0; 
			function getA() {
				return $this->a;
			}
			function setA($par){
				$this->a = $par; 
			}
		}
		$b = new A; 
		println($b->getA());
		$b->setA(1);
		println($b->getA());
        eval($b->getA());
		`
		ssatest.CheckSyntaxFlow(t, code,
			`eval(* #-> * as $param)`,
			map[string][]string{},
			ssaapi.WithLanguage(ssaapi.PHP))
		//ssatest.CheckPrintlnValue(code, []string{
		//	"Undefined-$b.getA(valid)(make(A)) member[0]",
		//	"Undefined-$b.getA(valid)(make(A)) member[side-effect(Parameter-$par, $this.a)]",
		//}, t)
	})
}

func TestOOP_Extend_Class(t *testing.T) {

	t.Run("normal", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class O {
			var $a = 0; 
		}
		class A extends O{}
		$a = new A; 
		println($a->a);

		$b = "a";
		println($a->$b);

		$c = "b";
		println($a->$$c);
		`, []string{
			"0", "0", "0",
		}, t)
	})

	t.Run("side effect", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class O {
			var $a = 0; 
			function setA($par){
				$this->a = $par; 
			}
		}
		class A extends O{}
		$a = new A; 
		println($a->a);
		$a->setA(1);
		println($a->a);
		`, []string{
			"0", "side-effect(Parameter-$par, $this.a)",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class O {
			var $a = 0; 
			function getA() {
				return $this->a;
			}
		}
		class A extends O{}
		$a = new A; 
		println($a->getA());
		$a->a = 1;
		println($a->getA());
		`, []string{
			"Undefined-$a.getA(valid)(Undefined-$a) member[0]",
			"Undefined-$a.getA(valid)(Undefined-$a) member[1]",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class O {
			var $a = 0; 
			function getA() {
				return $this->a;
			}
			function setA($par){
				$this->a = $par; 
			}
		}
		class A extends O{}
		$a = new A; 
		println($a->getA());
		$a->setA(1);
		println($a->getA());
		`, []string{
			"Undefined-$a.getA(valid)(Undefined-$a) member[0]",
			"Undefined-$a.getA(valid)(Undefined-$a) member[side-effect(Parameter-$par, $this.a)]",
		}, t)
	})
}

func TestParseCLS_Construct(t *testing.T) {
	t.Run("no construct", func(t *testing.T) {
		code := `<?php
		class A {
			var $num = 0;
			public function getNum() {
				return $this->num;
			}
		}
		$a = new A(); 
		println($a->getNum());
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-$a.getNum(valid)(Undefined-$a) member[0]",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		code := `<?php
class A {
	private $num = 0;
	public function __construct($num) {
		$this->num = $num;
	}
	public function getNum() {
		return $this->num;
	}
}
$a = new A(1);
println($a->getNum());`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-$a.getNum(valid)(Undefined-$a) member[side-effect(Parameter-$num, $this.num)]",
		}, t)
	})
}

func TestOOP_Class_Const(t *testing.T) {
	t.Run("test const value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
<?php
class MyClass
{
    const CONSTANT = 1; 
}

println(MyClass::CONSTANT);

$classname = "MyClass";
println($classname::CONSTANT);

$class = new MyClass();
println($class::CONSTANT);

	`, []string{
			"1", "1", "1",
		}, t)
	})
}
func TestOOP_Class_closure(t *testing.T) {
	code := `<?php
$c = new class("2"){
    public $a=1;
    public function __construct($a){
        $this->a=$a;
    }
};
println($c->a);`
	ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-$a, $this.a)"}, t)
}

//func TestOOP_custom_member(t *testing.T) {
//	code := `<?php
//    class test{
//        public $a = 1;
//    }
//	$c = new test();
//	println($c->$a);
//`
//	ssatest.CheckPrintlnValue(code, []string{"1"}, t)
//}

func TestOOP_Class_Instantiation(t *testing.T) {
	t.Run("Instantiate a non-existent object", func(t *testing.T) {
		code := `
<?php
		
		$a = new A();
		println($a);`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-$a",
		}, t)
	})

	t.Run("instantiate an existing object ", func(t *testing.T) {
		code := `
<?php
		class A {
			var $num = 0;
			public function getNum() {
				return $this->num;
			}
		}
		$a = new A(); 
		println($a->getNum());`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-$a.getNum(valid)(Undefined-$a) member[0]",
		}, t)
	})
}

func TestOOP_Syntax(t *testing.T) {
	t.Run("__construct", func(t *testing.T) {
		code := `<?php

class t
{
    public $a = 1;

    public function __construct()
    {
        $this->a = 2;
    }
}

$c = new t();
println($c->a);`
		CheckPrintTopDef(t, code, []string{"2"})
	})
	t.Run("__destruct", func(t *testing.T) {
		code := `<?php
class test{
    public $a;
    function __destruct(){
        $this->a=1;
		print($this->a);
    }
}
$c = new test;
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))

		//todo: 这个测试有问题
		//ssatest.CheckSyntaxFlowEx(t, code,
		//	`print(* #-> * as $param)`,
		//	false,
		//	map[string][]string{"param": {"1"}},
		//	ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("extends __destruct", func(t *testing.T) {
		code := `<?php
class test{
    public $a;
    function __destruct(){
        eval($this->a);
    }
}

class childTest extends test{}
$c = new childTest;
$c->a = 1;
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
		//ssatest.CheckSyntaxFlow(t, code,
		//	`eval(* #-> * as $param)`,
		//	map[string][]string{"param": {`1`}},
		//	ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestOOP_Extend(t *testing.T) {
	t.Run("impl __construct", func(t *testing.T) {
		code := `<?php
class b{
    public $a;
    public function __construct($a){
        $this->a = $a;
    }
}

class childB extends b{
}
$a = new childB(1);
println($a->a);
`
		ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-$a, $this.a)"}, t)
	})

	t.Run("no impl __construct and get parent custom member", func(t *testing.T) {
		code := `<?php
class b{
    public $a=0;
    public function __construct($a){
        $this->a = $a;
    }
}

class childB extends b{
    public $c;
    public function __construct($a){
    }
}
$b = new childB(1);
println($b->a);
`
		ssatest.CheckPrintlnValue(code, []string{"0"}, t)
	})
	t.Run("class custom member", func(t *testing.T) {
		code := `<?php
class A{
    public function get(){
        return 1;
    }
}
$class = "A";
$engine = new $class();
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("class typehint", func(t *testing.T) {
		code := `<?php
class A{
    public ?string $a=1;
}`
		ssatest.MockSSA(t, code)
	})

	//todo: 待过测试

	//	t.Run("static ++", func(t *testing.T) {
	//		code := `<?php
	//$a = self::$readTimes++;
	//`
	//		ssatest.MockSSA(t, code)
	//	})

	t.Run("test-oop-function", func(t *testing.T) {
		code := `<?php

class test
{
    public function aa()
    {
        $this->testb("123");
    }
	public function testb($cmd)
    {
        exec("$cmd" . "cc");
    }
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test-function", func(t *testing.T) {
		code := `<?php

class test
{
    public $filePath = "";

    public function __construct()
    {
        $this->filePath = true ? I("123") : "";
    }

    public function exec($cmd)
    {
        exec("$cmd");
    }

    public function test1()
    {
		$cmd = "$this->filePath"."whoami";
        $this->exec($cmd);
    }
}

new test();`
		ssatest.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("cls use other cls function", func(t *testing.T) {
		code := `<?php

class t
{
    public function a()
    {
        $a = new b();
        println($a->bb());
    }
}

class b
{
    public function bb()
    {
        return "1";
    }
}`
		CheckPrintTopDef(t, code, []string{`"1"`})
	})

	t.Run("cls use other cls static function", func(t *testing.T) {
		code := `<?php
	
	class t
	{
	   public function a()
	   {
	       println(b::bb());
	   }
	}
	
	class b
	{
	   public static function bb()
	   {
	       return "1";
	   }
	}`
		CheckPrintTopDef(t, code, []string{`"1"`})
	})

	t.Run("anymous-class with parent", func(t *testing.T) {
		code := `<?php
	
	class a
	{
	   public $a;
	
	   public function __construct($a)
	   {
	       $this->a = $a;
	   }
	}
	
	$c = new class("2") extends a {
	};
	println($c->a);`
		ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-$a, $this.a)"}, t)
	})
	t.Run("class const", func(t *testing.T) {
		code := `<?php

class b
{
    const c = 12;
}

println(b::c);`
		CheckPrintTopDef(t, code, []string{"12"})
	})
	t.Run("class use parent static", func(t *testing.T) {
		code := `<?php


class testxx
{
    public static $a = 1;
}

class test extends testxx
{
}


println(test::$a);`
		CheckPrintTopDef(t, code, []string{"1"})
	})

	//todo: 这个有问题
	t.Run("class use parent static and modify static member", func(t *testing.T) {
		code := `<?php


class testxx
{
    public static $a = 1;
}

class test extends testxx
{
}
testxx::$a=2;

println(test::$a);`
		CheckPrintTopDef(t, code, []string{"2"})
	})
	t.Run("test static member", func(t *testing.T) {
		code := `<?php

class a
{
    public static $a = 1;
}

println(a::$a);
`
		ssatest.CheckPrintlnValue(code, []string{"phi(a)[1]"}, t)
	})
	t.Run("oop custom member", func(t *testing.T) {
		code := `<?php

class a
{
    public $a = 1;
}

$c = new a();
println($c->a);`
		CheckPrintTopDef(t, code, []string{"1"})
	})
	t.Run("oop test", func(t *testing.T) {
		code := `<?php
switch ($type) {
    case wechat::MSGTYPE_TEXT:
    case wechat::MSGTYPE_VOICE:
}`
		ssatest.NonStrictMockSSA(t, code)
	})
	t.Run("oop member assign", func(t *testing.T) {
		code := `<?php

class a
{
    public $file;

    public function test()
    {
        $this->file = input("file");
        eval($this->file);
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `eval(* #-> as $param)`, map[string][]string{
			"param": {`"file"`, `Undefined-input`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
