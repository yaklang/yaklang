package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
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
	t.Run("check no variables declare", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
$a = $a??12312;
println($a);
`, []string{"12312"}, t)
	})
	t.Run("check has variables declare", func(t *testing.T) {
		code := `<?php
$a = 1;
$a = $a??12312;
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
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
$a=1;
$a.=$a;
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"\"11\""}, t)
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
	code := `<?php $a=1;if($b){$a=2;}`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("php"))
	if err != nil {
		t.Error(err)
		return
	}
	parse.Ref("$a").Show()
	//fmt.Println(parse.GetAllSymbols())
}
