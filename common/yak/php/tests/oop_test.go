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
