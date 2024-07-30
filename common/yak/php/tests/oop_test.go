package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

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
			"add(Undefined-Foo.my_static(valid), Undefined-PHP_EOL)",
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
			"phi(Foo_my_static)[\"foo\",\"bar\"]",
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
			"Function-Foo_aStaticMethod()",
			"Function-Foo_aStaticMethod()",
			"Function-Foo_aStaticMethod()",
			"Function-Foo_aStaticMethod()",
			"Function-Foo_aStaticMethod()",
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
			"Function-A_aStaticMethod()",
			"Function-A_aStaticMethod()",
			"Function-A_aStaticMethod()",
			"Function-A_aStaticMethod()",
			"Function-A_aStaticMethod()",
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
			"Undefined-$a.a(valid)", "Undefined-$a.a(valid)", "Undefined-$a.a(valid)",
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
			"Undefined-$a.a(valid)", "side-effect(Parameter-$par, $this.a)",
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
			"Undefined-$a.getA(valid)(Undefined-$a) member[Undefined-$a.a(valid)]",
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
			"Undefined-$a.a(valid)", "Undefined-$a.a(valid)", "Undefined-$a.a(valid)",
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
			"Undefined-$a.a(valid)", "side-effect(Parameter-$par, $this.a)",
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
			"Undefined-$a.getA(valid)(Undefined-$a) member[Undefined-$a.a(valid)]",
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
			"Undefined-$a.getA(valid)(Undefined-$a) member[Undefined-$a.a(valid)]",
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
			"Undefined-$a.getNum(valid)(Undefined-$a) member[Undefined-$a.num(valid)]",
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
		println($a);`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-$a",
		}, t)
	})

}

// func TestOOP_Syntax(t *testing.T) {
// 	t.Run("__construct", func(t *testing.T) {
// 		code := `<?php
// class test{
//     public $a;
//     public function __construct($a){
//     	$this->a = $a;
//         println($this->a);
// 	}
// }
// $a = new test("1");
// `
// 		//执行会有问题，
// 		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
// 			prog.Show()
// 			return nil
// 		}, ssaapi.WithLanguage(ssaapi.PHP))
// 		//ssatest.CheckSyntaxFlow(t, code,
// 		//	`println(* #-> * as $param)`,
// 		//	map[string][]string{"param": {`"1"`}},
// 		//	ssaapi.WithLanguage(ssaapi.PHP))
// 	})
// 	t.Run("__destruct", func(t *testing.T) {
// 		code := `<?php
// class test{
//     public $a;
//     function __destruct(){
//         $this->a=1;
//         print($this->a);
//     }
// }
// $c = new test;
// `
// 		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
// 			prog.Show()
// 			return nil
// 		}, ssaapi.WithLanguage(ssaapi.PHP))
// 		ssatest.CheckSyntaxFlow(t, code,
// 			`print(* #-> * as $param)`,
// 			map[string][]string{"param": {`Undefined-$c.a(valid)`}},
// 			ssaapi.WithLanguage(ssaapi.PHP))
// 	})
// 	t.Run("extends __destruct", func(t *testing.T) {
// 		code := `<?php
// class test{
//     public $a;
//     function __destruct(){
//         eval($this->a);
//     }
// }

// class childTest extends test{}
// $c = new childTest;
// $c->a = 1;
// `
// 		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
// 			prog.Show()
// 			return nil
// 		}, ssaapi.WithLanguage(ssaapi.PHP))
// 		//ssatest.CheckSyntaxFlow(t, code,
// 		//	`eval(* #-> * as $param)`,
// 		//	map[string][]string{"param": {`1`}},
// 		//	ssaapi.WithLanguage(ssaapi.PHP))
// 	})
// 	t.Run("code", func(t *testing.T) {
// 		code := `<?php
// function __destruct(){}
// __destruct();
// `
// 		ssatest.MockSSA(t, code)
// 	})
// }

func TestOOP_Extend(t *testing.T) {
	t.Run("no impl __construct", func(t *testing.T) {
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

	t.Run("impl __construct and get parent custom member", func(t *testing.T) {
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
		ssatest.CheckPrintlnValue(code, []string{"Undefined-$b.a(valid)"}, t)
	})
}
