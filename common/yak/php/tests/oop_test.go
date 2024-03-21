package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestOOP_static_member(t *testing.T) {

	t.Run("normal static member", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	<?php
class Foo {
	public static $my_static = 'foo';
}

println(Foo::$my_static . PHP_EOL);

?>    
	`, []string{
			"add(\"foo\", Parameter-PHP_EOL)",
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
			"add(any, Parameter-PHP_EOL)",
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
				"add(\"foo\", Parameter-PHP_EOL)",
			}, t)
	})
	t.Run("defined variable", func(t *testing.T) {
		code := `<?php
$PHP_EOL=1;
println($PHP_EOL);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
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
			"Function-.getA(Function-__constructor_normal(),0)",
			"Function-.getA(Function-__constructor_normal(),1)",
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
			"Function-.getA(Function-__constructor_normal(),0)",
			"Function-.getA(Function-__constructor_normal(),side-effect(Parameter-$par, $this.a))",
		}, t)
	})
}

