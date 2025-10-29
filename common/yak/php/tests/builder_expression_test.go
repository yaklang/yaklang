package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestExpression_PHP_Name(t *testing.T) {
	t.Run("$", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		<?php
		function a() {
		}
		$a = 1;
		println(a());
		println($a);
		`, []string{
			"Function-a()", "1",
		}, t)
	})
	t.Run("defined variable", func(t *testing.T) {
		code := `<?php
$PHP_EOL=1;
println($PHP_EOL);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
}
func TestCallNumber(t *testing.T) {
	code := `<?php
$a = new B();
$a->a();
$a->a();
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(".a() as $call", ssaapi.QueryWithEnableDebug())
		assert.True(t, len(result.GetValues("call")) == 2)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestMemberAddSideEffect(t *testing.T) {
	code := `<?php
	class test{
	public $a = 0;
	public function __construct($a){
		$this->a = $a;
	}
}
	$a = new test(1);
	$a->a++;
	println($a->a);
`
	ssatest.CheckPrintlnValue(code, []string{"add(side-effect(Parameter-$a, $this.a), 1)"}, t)
}

func TestExperssion_PHP_Scope(t *testing.T) {
	t.Run("block scope capture a", func(t *testing.T) {
		code := `<?php
$a = 1;
{
	$a = 2;
}
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"2"}, t)
	})
}

func TestExpression_BitwiseExpression(t *testing.T) {
	t.Run("&&", func(t *testing.T) {
		code := `<?php
$b =($a=1) && ($a=0);
println($b);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($b)[eq(0, true),false]", "phi($a)[0,1]"}, t)
	})
	t.Run("||", func(t *testing.T) {
		code := `<?php
$b =($a=0) || ($a=1);
println($b);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($b)[true,eq(1, true)]", "phi($a)[0,1]"}, t)
	})
}
func TestExpression_NullCoalescingExpression(t *testing.T) {
	t.Run("check has variables declare", func(t *testing.T) {
		code := `<?php
$a = 1;
$a = $a??12312;
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[12312,1]"}, t)
	})

}
func TestPhpPrintln(t *testing.T) {
	c := `<?php $a=1; println($a);`
	ssatest.CheckPrintf(t, ssatest.TestCase{Want: []string{"1"}, Code: c})
}
func TestExpression_AssignmentOperator(t *testing.T) {
	t.Run("operator -=", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
$a=1;
$a-=1;
println($a);
`,
			[]string{"0"}, t,
		)
	})

	t.Run("opertor +=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a=1;
$a+=1;
println($a);`, []string{"2"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a+=1;
println($a);`, []string{"add(Undefined-$a, 1)"}, t)
		})
	})
	t.Run("opertor *=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a=1;
$a*=5;
println($a);
`, []string{"5"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a*=5;
println($a);
`, []string{"mul(Undefined-$a, 5)"}, t)
		})
	})
	t.Run("opertor /=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a=5;
$a/=5;
println($a);
`, []string{"1"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a/=5;
println($a);
`, []string{"div(Undefined-$a, 5)"}, t)
		})
	})
	t.Run("opertor **=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a=2;
$a**=3;
println($a);
`, []string{"8"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a**=3;
println($a);
`, []string{"pow(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("opertor %=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 10;
$a %=3;
println($a);`, []string{"1"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a %=3;
println($a);`, []string{"mod(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("opertor &=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 10;
$a &=3;
println($a);
`, []string{"2"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a &=3;
println($a);
`, []string{"and(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("opertor |=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 10;
$a |=3;
println($a);
`, []string{"11"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a |=3;
println($a);
`, []string{"or(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("opertor ^=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 10;
$a ^=3;
println($a);
`, []string{"9"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a ^=3;
println($a);
`, []string{"xor(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("operator <<=", func(t *testing.T) {
		t.Run("test-1", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 10;
$a <<=3;
println($a);
`, []string{"80"}, t)
		})
		t.Run("test-2", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a <<=3;
println($a);
`, []string{"shl(Undefined-$a, 3)"}, t)
		})
	})
	t.Run("operator ??=", func(t *testing.T) {
		t.Run("no variable declare", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a ??= 1;
println($a);
?>`, []string{"1"}, t)
		})
		t.Run("has variable declare", func(t *testing.T) {
			ssatest.CheckPrintlnValue(`<?php
$a = 2;
$a ??= 1;
println($a);
?>`, []string{"2"}, t)
		})
	})
	t.Run("operator-'.='", func(t *testing.T) {
		code := `<?php
$a="1";
$a.=$a;
println($a);`
		ssatest.CheckPrintlnValue(code, []string{`"11"`}, t)
	})
}

func TestExpression_LogicExpression(t *testing.T) {
	t.Run("and-1", func(t *testing.T) {
		code := `<?php
$b = 2 and  3;
println($b);
`
		ssatest.CheckPrintlnValue(code, []string{"2"}, t)
	})
	t.Run("and-2", func(t *testing.T) {
		code := `<?php
$b =(($a=1) and ($a=3));
println($a);
println($b);
`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[3,1]", "phi($b)[eq(3, true),false]"}, t)
	})
	t.Run("or-1", func(t *testing.T) {
		code := `<?php
$b = 2 or  3;
println($b);`
		ssatest.CheckPrintlnValue(code, []string{"2"}, t)
	})
	t.Run("or-2", func(t *testing.T) {
		code := `<?php
$b = (($a=2) or ($a=3));
println($b);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($b)[true,eq(3, true)]", "phi($a)[2,3]"}, t)
	})
	t.Run("xor-1", func(t *testing.T) {
		code := `<?php
$a = 1 xor 1;
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("xor-2", func(t *testing.T) {
		code := `<?php
$b = (($a=1) xor ($a=3));
println($a);
println($b);`
		ssatest.CheckPrintlnValue(code, []string{"3", "phi($b)[true,false]"}, t)
	})
}
func TestExpression_OrdinaryAssignmentExpression(t *testing.T) {
	t.Run("=", func(t *testing.T) {
		code := `<?php 
$a=0;
$a+=1;
println($a);
$a-=2;
println($a);
$a*=5;
println($a);
`
		ssatest.CheckPrintlnValue(code, []string{"1", "-1", "-5"}, t)
	})
	t.Run("+=", func(t *testing.T) {
		code := `<?php
$a=0;
$a+=1;
println($a);
$a-=1;
`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
}
func TestExpression_DynamicVariable(t *testing.T) {
	t.Run("check $$a", func(t *testing.T) {
		ssatest.CheckPrintlnValue(
			`<?php
$a = "b";
$$a = 2; 
println($$a);
`,
			[]string{"2"}, t)
	})

	t.Run("check $b", func(t *testing.T) {
		ssatest.CheckPrintlnValue(
			`<?php
$a = "b";
$$a = 2; 
println($b);
`,
			[]string{"2"}, t)
	})

	t.Run("check $$$", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
// Variable expression and dynamic variable expression
$identifier = "dynamicVar";
$dynamicVar = "test";
$test="1";
$$$identifier=123;

println($test);`,
			[]string{"123"}, t)
	})
}

func TestAssignVariables(t *testing.T) {
	t.Run("test $_GET variables", func(t *testing.T) {
		code := `<?php
$a = $_GET["1"];
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"Undefined-$a(valid)"}, t)
	})
}

func TestParseSSA_DeclareConst(t *testing.T) {
	t.Run("global const declare", func(t *testing.T) {
		code := `<?php
const NAME = 1,DJAOP=2;
println(NAME);
println(DJAOP);`
		ssatest.CheckPrintlnValue(code, []string{"1", "2"}, t)
	})
	t.Run("global const declare redefined", func(t *testing.T) {
		code := `<?php
const NAME = "1";
const NAME = 2;
println(NAME);`
		ssatest.CheckPrintlnValue(code, []string{"\"1\""}, t)
	})
	t.Run("defined const", func(t *testing.T) {
		code := `<?php define('a',1); println(a);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("define function const redefined", func(t *testing.T) {
		code := `<?php

define('a','2');
define('a',1);
println(a);`
		ssatest.CheckPrintlnValue(code, []string{"\"2\""}, t)
	})
	t.Run("const and definded function", func(t *testing.T) {
		code := `<?php
const a = 3;
define('a','2');
println(a);`
		ssatest.CheckPrintlnValue(code, []string{"3"}, t)
	})
	t.Run("const and function, use const", func(t *testing.T) {
		code := `<?php
const a = 1;
function a(){
    echo "ada";
}
println(a);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("const and function,use function", func(t *testing.T) {
		code := `<?php
const a = 1;
function a(){
    echo "ada";
}
println(a());`
		ssatest.CheckPrintlnValue(code, []string{"Function-a()"}, t)
	})
	t.Run("function and const", func(t *testing.T) {
		code := `<?php
const a = 1;
function a(int $c){
    echo "ada";
}
function b(int $a){
    echo $a;
}

println(a(b(a)));`
		ssatest.CheckPrintlnValue(code, []string{"Function-a(Function-b(1))"}, t)
	})
}

func TestParseSSA_MemberCallKey(t *testing.T) {
	t.Run("memberCallKey", func(t *testing.T) {
		code := `<?php
$a[1|1]=0;
println($a[1|1]);
`
		ssatest.CheckPrintlnValue(code, []string{"0"}, t)
	})
}

func TestParseSSA_NothingBody(t *testing.T) {
	code := `<?php`
	ssatest.MockSSA(t, code)
}

func TestParseSSA_Include(t *testing.T) {
	t.Run("set_include_path_lower", func(t *testing.T) {
		code := `<?php
set_include_path("./syntax");
include('for.php');
`
		ssatest.MockSSA(t, code)
	})
	t.Run("set_include_path_union", func(t *testing.T) {
		code := `<?php
set_INclude_path("./syntax");
include('for.php');
`
		ssatest.MockSSA(t, code)
	})
	t.Run("include", func(t *testing.T) {
		code := `<?php
include('syntax/include/include.php');
`
		ssatest.MockSSA(t, code)
	})

}

func TestVariables(t *testing.T) {
	code := `<?php 
$a = &$c;
$fields_meta{1}->a;
$fields_meta[1]->a;
$fields_meta{1}{1}->a;
$fields_meta{1}{1}->a=1;
$this->{$kind} = [$address, $name];
$this->{$kind}[] = [$address, $name];
$d->getMockBuilder();
a::c()->c();
a::c()->b;
$stub = $this->getMockBuilder(SMTP::class)->getMock();
$a = <<<EOT 
ac
EOT;
`
	ssatest.CheckPrintlnValue(code, []string{}, t)
}
func TestSyntaxClassStatic(t *testing.T) {
	code := `<?php
$a = $c::$c;
$c = $c::${$c->c};
$a = c::$c;
$a = c::${$c->c};
$a = "test"::$c;
$a = "test"::${$c->c};
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func Test_Array(t *testing.T) {
	t.Run("array assign1", func(t *testing.T) {
		code := `<?php
$validate = "1.2";
[$validate,$scene]=explode('.',$validate);
eval($validate);
`
		ssatest.CheckSyntaxFlow(t, code,
			`eval(* #-> *  as $param)`,
			map[string][]string{
				"param": {"Function-explode", `"."`, `"1.2"`},
			},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("array assign2", func(t *testing.T) {
		code := `<?php
	$a = array("1","2");
	[$validate,$scene]=$a;
	eval($validate);
`
		ssatest.CheckSyntaxFlow(t, code,
			`eval(* #-> *  as $param)`,
			map[string][]string{
				"param": {"Function-array", `"2"`, `"1"`},
			},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("instanceof expression", func(t *testing.T) {
		code := `<?php

$c = $a instanceof $b;
println($c);`
		ssatest.CheckPrintlnValue(code, []string{"Function-instanceOf(Undefined-$a,Undefined-$b)"}, t)
	})
}
func TestCondition(t *testing.T) {
	code := `<?php
$a = 1;
$a<>"";
`
	ssatest.MockSSA(t, code)
}
