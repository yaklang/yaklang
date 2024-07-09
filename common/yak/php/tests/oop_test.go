package tests

import (
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
			"add(Undefined-Foo_my_static, Undefined-PHP_EOL)",
		}, t)
	})

	t.Run("normal static member,  assign again ", func(t *testing.T) {
		code := `
	<?php
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
}

func TestOOP_var_member(t *testing.T) {

	t.Run("normal", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		class A {
			var $a = 0; 
		}
		$a = new A; 
		println($a->a); // 0

		$b = "a";
		println($a->$b); // 0

		$c = "b";
		println($a->$$c); // 0
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
			"Undefined-$a.getA(valid)(make(A)) member[0]",
			"Undefined-$a.getA(valid)(make(A)) member[1]",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
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
		$a = new A; 
		println($a->getA());
		$a->setA(1);
		println($a->getA());
		`, []string{
			"Undefined-$a.getA(valid)(make(A)) member[0]",
			"Undefined-$a.getA(valid)(make(A)) member[side-effect(Parameter-$par, $this.a)]",
		}, t)
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
		println($a->a); // 0

		$b = "a";
		println($a->$b); // 0

		$c = "b";
		println($a->$$c); // 0
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
			"Undefined-$a.getA(valid)(make(A)) member[0]",
			"Undefined-$a.getA(valid)(make(A)) member[1]",
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
			"Undefined-$a.getA(valid)(make(A)) member[0]",
			"Undefined-$a.getA(valid)(make(A)) member[side-effect(Parameter-$par, $this.a)]",
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
			"Undefined-$a.getNum(valid)(make(A)) member[0]",
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
			"Undefined-$a.getNum(valid)(make(A)) member[side-effect(Parameter-$num, $this.num)]",
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

func TestOOP_Class_Instantiation(t *testing.T) {
	t.Run("Instantiate a non-existent object", func(t *testing.T) {
		code := `
<?php
		
		$a = new A();
		println($a);`
		ssatest.CheckPrintlnValue(code, []string{
			"make(any)",
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
			"make(A)",
		}, t)
	})

}

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
	t.Run("impl __construct", func(t *testing.T) {
		code := `<?php


class b{
    public $a;
    public function __construct($a){
        $this->a = $a;
    }
}

class childB extends b{
    public $c;
    public function __construct($a){
     parent::__construct($a);
}
}
$b = new childB(1);
println($b->a);
`
		ssatest.CheckPrintlnValue(code, []string{"Undefined-.a(valid)"}, t)
	})
	t.Run("get parent static member", func(t *testing.T) {
		code := `<?php
class b{
    public static $a=1;
}
class childB extends b{
}
println(b::$a);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
}

func TestA(t *testing.T) {
	code := `<?php

class test{
    public $a=0;
}

$a = new test;
$a->a=1;
`
	ssatest.MockSSA(t, code)
}
