package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestOOP_static_member(t *testing.T) {

	t.Run("normal static member", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	<?php
class Foo {
	public static $my_static = 'foo';
}

println(Foo::$my_static . PHP_EOL); // normal

println("Foo"::$my_static . PHP_EOL); // string

$a = "Foo";
println($a::$my_static . PHP_EOL); // variable

$b = "a";
println($$b::$my_static . PHP_EOL); // dynamic variable

?>    
	`, []string{
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
		}, t)

	})

	t.Run("normal static member, use any", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	<?php
class Foo {
	public static $my_static;
}

println(Foo::$my_static . PHP_EOL);

?>    
	`, []string{
			"add(Undefined-Foo_my_static, Undefined-PHP_EOL)",
		}, t)
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
		ssatest.CheckPrintlnValue(
			code, []string{
				"add(\"foo\", Undefined-PHP_EOL)",
			}, t)
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
		ssatest.CheckPrintlnValue(code, []string{
			"phi(Foo.my_static)[\"foo\",\"bar\"]",
		}, t)
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
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
			"add(\"foo\", Undefined-PHP_EOL)",
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
			"Function-Foo.aStaticMethod()",
			"Function-Foo.aStaticMethod()",
			"Function-Foo.aStaticMethod()",
			"Function-Foo.aStaticMethod()",
			"Function-Foo.aStaticMethod()",
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
		_, err := php2ssa.Frontend(code)
		require.Error(t, err)
	})

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
			"Function-A.aStaticMethod()",
			"Function-A.aStaticMethod()",
			"Function-A.aStaticMethod()",
			"Function-A.aStaticMethod()",
			"Function-A.aStaticMethod()",
		}, t)

	})

	t.Run("get static method by variable name", func(t *testing.T) {
		code := `
<?php
class MyClass {
    public static function myStaticMethod() {
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `*myStaticMethod as $fun`, map[string][]string{
			"fun": {"Function-MyClass.myStaticMethod"},
		}, ssaapi.WithLanguage(consts.PHP))
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
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
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
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
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
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-$a.getA(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-$par, $this.a)]",
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
			"Undefined-$a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
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
			"Undefined-$a.getNum(valid)(Function-A.A(Undefined-A,1)) member[side-effect(Parameter-$num, $this.num)]",
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
			"Undefined-A(Undefined-A)",
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
			"Undefined-$a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
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
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"2"})
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
		ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-$a, $this.a)"}, t)
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
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{`"1"`})
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
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{`"1"`})
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
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"12"})
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
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{})
	})

	t.Run("add nomarvariable member", func(t *testing.T) {
		code := `<?php

class test
{
    public static $a = 1;
}

$a = "test";
$b = "$a";
println($a::$b);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{})
	})
	t.Run("oop custom member", func(t *testing.T) {
		code := `<?php

class a
{
    public $a = 1;
}

$c = new a();
println($c->a);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
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

	t.Run("test oop construct", func(t *testing.T) {
		code := `<?php

class a{
    public $a = 1;
    public function __construct($a){
        $this->a = $a;
    }
}
$a = new a(2);
println($a->a);`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"2"}}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test oop constructor", func(t *testing.T) {
		code := `<?php
class A{
	public function __construct($a){
		$this->b = $a;
	}
}
$a = new A(2);
println($a->b);
`
		ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-$a, $this.b)"}, t)
	})
	t.Run("test oop return", func(t *testing.T) {
		code := `<?php
class A{
    public $a =1;
}
function test(){
    $a = new A();
    $a->a = 2;
    return $a;
}

$b = test();
println($b->a);
`
		ssatest.CheckPrintlnValue(code, []string{"side-effect(2, $a.a)"}, t)
	})
	//todo: visit if condition
	//
	//	t.Run("test", func(t *testing.T) {
	//		code := `<?php
	//   function _Include($a)
	//   {
	//       $path = WWWROOT . "/public" . $a;
	//       if (!file_exists($path)) {
	//           return;
	//       } else {
	//           include $path;
	//       }
	//   }
	//       $a = $_GET['a'] ?: "aaaa";
	//       _Include(filter($a));`
	//		ssatest.CheckSyntaxFlow(t, code, `<include('php-param')> as $params;
	//<include('php-filter-function')> as $filter;
	//include(* as $param);
	//`+
	//			"$param #{until: `<self> & $params`,include: `<self> & $params`}-> as $root;"+
	//			`$root?{<dataflow(<<<CODE
	//<self>?{opcode: call && !<self & $filter} as $__next__;
	//CODE)>} as $low;`, map[string][]string{}, ssaapi.WithLanguage(ssaapi.PHP))
	//	})
}

func TestOopFunc(t *testing.T) {
	code := `<?php
class A{
	public function A_method(){
	}
}
function main(){
	$a = new A();
	println($a->A_method());
}
`
	ssatest.CheckPrintlnValue(code, []string{"Undefined-$a.A_method(valid)(Undefined-A(Undefined-A))"}, t)
}

func TestOOP_Super_Class(t *testing.T) {
	t.Run("test super class's field", func(t *testing.T) {
		code := `
<?php
class ParentClass {
    protected $name = "Parent";
}

class ChildClass extends ParentClass {
    protected static $name = "Child";

    public function display() {
       println(parent::$name);
    }
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"\"Parent\"",
		}, t)
	})
	t.Run("more extends", func(t *testing.T) {
		code := `<?php

class A{
    public static $a = 1;
}
class B extends A{
}

class C extends B{
    public function AA(){
        println(parent::$a);
    }
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})

	t.Run("test super class's static field", func(t *testing.T) {
		code := `
<?php
class ParentClass {
    protected static $name = "Parent";
}

class ChildClass extends ParentClass {
    protected static $name = "Child";

    public function display() {
       println(parent::$name);
    }
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"\"Parent\"",
		}, t)
	})

	t.Run("test super class's static method", func(t *testing.T) {
		code := `
<?php
class ParentClass {
    public static function staticMethod() {
        echo "This is the parent static method.";
    }
}

class ChildClass extends ParentClass {
    public static function staticMethod() {
        echo "This is the child static method, overriding the parent static method.";
    }

    public static function callParentStaticMethod() {
        println(parent::staticMethod());
    }
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-ParentClass.staticMethod()",
		}, t)
	})

	t.Run("test super class's  method", func(t *testing.T) {
		code := `
<?php
class ParentClass {
    public  function Method() {
        echo "This is the parent static method.";
    }
}

class ChildClass extends ParentClass {
    public static function Method() {
        echo "This is the child static method, overriding the parent static method.";
    }

    public static function callParentStaticMethod() {
        println(parent::Method());
    }
}
`
		ssatest.CheckPrintlnValue(code, []string{
			`Undefined-.Method(valid)("parent")`,
		}, t)
	})
	t.Run("blueprint test", func(t *testing.T) {
		code := `<?php
$c->a::bb(1);
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test blueprint loop", func(t *testing.T) {
		code := `<?php

class A extends C{

}
class B extends A{

}
class C extends B{

}
$a = new C();
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestSelf(t *testing.T) {
	code := `<?php

class StmComment extends StmBaseModel{


	public static function get_comment($params = []){
		echo $params;
	}

	public static function get_commnet_api(){
		return self::get_comment($_GET);
	}
}`
	ssatest.CheckSyntaxFlow(t, code, `echo(* #-> as $sink)`, map[string][]string{
		"sink": {"Undefined-_GET"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
