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

func TestCode(t *testing.T) {
	code := `<?php
// +----------------------------------------------------------------------
// | ThinkPHP [ WE CAN DO IT JUST THINK ]
// +----------------------------------------------------------------------
// | Copyright (c) 2006~2018 http://thinkphp.cn All rights reserved.
// +----------------------------------------------------------------------
// | Licensed ( http://www.apache.org/licenses/LICENSE-2.0 )
// +----------------------------------------------------------------------
// | Author: liu21st <liu21st@gmail.com>
// +----------------------------------------------------------------------

namespace think;

class View
{
    // 视图实例
    protected static $instance;
    // 模板引擎实例
    public $engine;
    // 模板变量
    protected $data = [];
    // 用于静态赋值的模板变量
    protected static $var = [];
    // 视图输出替换
    protected $replace = [];

    /**
     * 构造函数
     * @access public
     * @param array $engine  模板引擎参数
     * @param array $replace  字符串替换参数
     */
    public function __construct($engine = [], $replace = [])
    {
        // 初始化模板引擎
        $this->engine($engine);
        // 基础替换字符串
        $request = Request::instance();
        $base    = $request->root();
        $root    = strpos($base, '.') ? ltrim(dirname($base), DS) : $base;
        if ('' != $root) {
            $root = '/' . ltrim($root, '/');
        }
        $baseReplace = [
            '__ROOT__'   => $root,
            '__URL__'    => $base . '/' . $request->module() . '/' . Loader::parseName($request->controller()),
            '__STATIC__' => $root . '/static',
            '__CSS__'    => $root . '/static/css',
            '__JS__'     => $root . '/static/js',
        ];
        $this->replace = array_merge($baseReplace, (array) $replace);
    }

    /**
     * 初始化视图
     * @access public
     * @param array $engine  模板引擎参数
     * @param array $replace  字符串替换参数
     * @return object
     */
    public static function instance($engine = [], $replace = [])
    {
        if (is_null(self::$instance)) {
            self::$instance = new self($engine, $replace);
        }
        return self::$instance;
    }

    /**
     * 模板变量静态赋值
     * @access public
     * @param mixed $name  变量名
     * @param mixed $value 变量值
     * @return void
     */
    public static function share($name, $value = '')
    {
        if (is_array($name)) {
            self::$var = array_merge(self::$var, $name);
        } else {
            self::$var[$name] = $value;
        }
    }

    /**
     * 模板变量赋值
     * @access public
     * @param mixed $name  变量名
     * @param mixed $value 变量值
     * @return $this
     */
    public function assign($name, $value = '')
    {
        if (is_array($name)) {
            $this->data = array_merge($this->data, $name);
        } else {
            $this->data[$name] = $value;
        }
        return $this;
    }

    /**
     * 设置当前模板解析的引擎
     * @access public
     * @param array|string $options 引擎参数
     * @return $this
     */
    public function engine($options = [])
    {
        if (is_string($options)) {
            $type    = $options;
            $options = [];
        } else {
            $type = !empty($options['type']) ? $options['type'] : 'Think';
        }

        $class = false !== strpos($type, '\\') ? $type : '\\think\\view\\driver\\' . ucfirst($type);
        if (isset($options['type'])) {
            unset($options['type']);
        }
        $this->engine = new $class($options);
        return $this;
    }

    /**
     * 配置模板引擎
     * @access private
     * @param string|array  $name 参数名
     * @param mixed         $value 参数值
     * @return $this
     */
    public function config($name, $value = null)
    {
        $this->engine->config($name, $value);
        return $this;
    }

    /**
     * 解析和获取模板内容 用于输出
     * @param string    $template 模板文件名或者内容
     * @param array     $vars     模板输出变量
     * @param array     $replace 替换内容
     * @param array     $config     模板参数
     * @param bool      $renderContent     是否渲染内容
     * @return string
     * @throws Exception
     */
    public function fetch($template = '', $vars = [], $replace = [], $config = [], $renderContent = false)
    {
        // 模板变量
        $vars = array_merge(self::$var, $this->data, $vars);

        // 页面缓存
        ob_start();
        ob_implicit_flush(0);

        // 渲染输出
        try {
            $method = $renderContent ? 'display' : 'fetch';
            // 允许用户自定义模板的字符串替换
            $replace = array_merge($this->replace, $replace, (array) $this->engine->config('tpl_replace_string'));
            $this->engine->config('tpl_replace_string', $replace);
            $this->engine->$method($template, $vars, $config);
        } catch (\Exception $e) {
            ob_end_clean();
            throw $e;
        }

        // 获取并清空缓存
        $content = ob_get_clean();
        // 内容过滤标签
        Hook::listen('view_filter', $content);
        return $content;
    }

    /**
     * 视图内容替换
     * @access public
     * @param string|array  $content 被替换内容（支持批量替换）
     * @param string        $replace    替换内容
     * @return $this
     */
    public function replace($content, $replace = '')
    {
        if (is_array($content)) {
            $this->replace = array_merge($this->replace, $content);
        } else {
            $this->replace[$content] = $replace;
        }
        return $this;
    }

    /**
     * 渲染内容输出
     * @access public
     * @param string $content 内容
     * @param array  $vars    模板输出变量
     * @param array  $replace 替换内容
     * @param array  $config     模板参数
     * @return mixed
     */
    public function display($content, $vars = [], $replace = [], $config = [])
    {
        return $this->fetch($content, $vars, $replace, $config, true);
    }

    /**
     * 模板变量赋值
     * @access public
     * @param string    $name  变量名
     * @param mixed     $value 变量值
     */
    public function __set($name, $value)
    {
        $this->data[$name] = $value;
    }

    /**
     * 取得模板显示变量的值
     * @access protected
     * @param string $name 模板变量
     * @return mixed
     */
    public function __get($name)
    {
        return $this->data[$name];
    }

    /**
     * 检测模板变量是否设置
     * @access public
     * @param string $name 模板变量名
     * @return bool
     */
    public function __isset($name)
    {
        return isset($this->data[$name]);
    }
}
`
	ssatest.MockSSA(t, code)
}
