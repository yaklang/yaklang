package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestExpression_If1(t *testing.T) {
	t.Run("condition", func(t *testing.T) {
		code := `<?php 
$cid = !empty($_REQUEST['cid']) ? b($_REQUEST['cid']) : '';
println($cid);`
		ssatest.CheckPrintlnValue(code, []string{`phi($cid)[Undefined-b(Undefined-.global.cid(valid)),""]`}, t)
	})
	t.Run("customIf", func(t *testing.T) {
		code := `<?php $a = 0;
println($a);
if ($c) {
    $a = 1;
    println($a);
}
println($a);
`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"0", "1", "phi($a)[1,0]"},
			Code: code,
		})
	})
	t.Run("custom-if-else", func(t *testing.T) {
		code := `<?php
$a=0;
if($b){
    $a=1;
    println($a);
}elseif($c){
    $a=2;
    println($a);
}elseif($d){
    $a=3;
    println($a);
}elseif($e){
    $a=4;
    println($a);
}else{
    $a=5;
    println($a);
}
println($a);
`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Code: code,
			Want: []string{"1", "2", "3", "4", "5", "phi($a)[1,2,3,4,5]"},
		})
	})
	t.Run("other-if", func(t *testing.T) {
		code := `<?php
$a = 1;
println($a);
if($b):
$a=2;
println($a);
endif;
println($a);`

		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"1", "2", "phi($a)[2,1]"},
			Code: code,
		})
	})
	t.Run("other-if-else", func(t *testing.T) {
		code := `<?php
$a = 1;
println($a);
if($b):
$a=2;
println($a);
elseif($b):
$a=3;
println($a);
endif;
println($a);`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"1", "2", "3", "phi($a)[2,3,1]"},
			Code: code,
		})
	})
	t.Run("html-if", func(t *testing.T) {
		code := `<?php if ($a == 5) { ?>
<sample></sample>
<?php }; ?>`
		ssatest.CheckError(t, ssatest.TestCase{
			Code: code,
			Want: []string{ssa.ValueUndefined("$a")},
		})
	})
	t.Run("html-if-else", func(t *testing.T) {
		code := `<?php if ($a == 5) { ?>
<sample></sample>
<?php }else{ ?>
    <script>1</script>
<?php }?>`
		ssatest.CheckError(t, ssatest.TestCase{
			Code: code,
			Want: []string{ssa.ValueUndefined("$a")},
		})
	})
}

func TestExpression_If(t *testing.T) {
	//todo: 还有问题
	t.Run("customIf", func(t *testing.T) {
		code := `<?php
$a=0;
if($b){
    $a=1;
    println($a);
}elseif($c){
    $a=2;
    println($a);
}elseif($d){
    $a=3;
    println($a);
}elseif($e){
    $a=4;
    println($a);
}else{
    $a=5;
    println($a);
}
println($a);
`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Code: code,
			Want: []string{"1", "2", "3", "4", "5", "phi($a)[1,2,3,4,5]"},
		})
	})

	t.Run("other-if", func(t *testing.T) {
		code := `<?php
$a = 1;
println($a);
if($b):
$a=2;
println($a);
endif;
println($a);`

		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"1", "2", "phi($a)[2,1]"},
			Code: code,
		})
	})
	t.Run("other-if-else", func(t *testing.T) {
		code := `<?php
$a = 1;
println($a);
if($b):
$a=2;
println($a);
elseif($b):
$a=3;
println($a);
endif;
println($a);`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"1", "2", "3", "phi($a)[2,3,1]"},
			Code: code,
		})
	})
}

func TestExpression_Switch(t *testing.T) {
	t.Run("switch-mock-default", func(t *testing.T) {
		ssatest.MockSSA(t, `<?php
switch($a){;
   case "1":;;;;;
       echo 1;
   default:;;;;
   default:;;;;
       echo 1;
}`)
	})
	t.Run("switch-mock2-type2", func(t *testing.T) {
		code := `<?php
switch($a):;
   case 1:
       echo 1;
   default:
       echo 1;
endswitch;`
		ssatest.CheckError(t, ssatest.TestCase{
			Code: code,
			Want: []string{ssa.ValueUndefined("$a")},
		})
	})
	t.Run("custom-switch-only-case", func(t *testing.T) {
		code := `<?php
$a =1;
switch($b){
   case "1":
       $a=2;
       println($a);
   case "2":
       $a=3;
       println($a);
   case "3":
       $a=4;
       println($a);
}
println($a);
`
		ssatest.CheckPrintf(t, ssatest.TestCase{
			Want: []string{"2", "3", "4", "phi($a)[4,1]"},
			Code: code,
		})
	})
	t.Run("custom-switch-case-defaut", func(t *testing.T) {
		code := `<?php
$a=1;
switch($b){
   case "1":
       $a=2;
       println($a);
   case "2":
       $a=3;
       println($a);
   case "3":
       $a=4;
       println($a);
    default:
       $a=5;
       println($a);
}
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"2", "3", "4", "5", "5"}, t)
	})

	t.Run("switch check case body, no break", func(t *testing.T) {
		ssatest.CheckPrintlnValue(
			`<?php
$a=1;
switch($b){
   case "1":
       println($a); // 1
       $a=2;
       println($a); // 2
   case "2":
       println($a); // phi($a)[2,1]
       $a=3;
       println($a); // 3 
    default:
       println($a); // phi($a)[3,1]
       $a=5;
       println($a); // 5
}
println($a); // 5
`,
			[]string{
				"1", "2",
				"phi($a)[2,1]", "3",
				"phi($a)[3,1]", "5",
				"5",
			},
			t)
	})

	t.Run("switch check case body, has break", func(t *testing.T) {
		ssatest.CheckPrintlnValue(
			`<?php
$a=1;
switch($b){
   case "1":
       println($a); // 1
       $a=2;
       println($a); // 2
	   break;
   case "2":
       println($a); // 1
       $a=3;
       println($a); // 3 
    default:
       println($a); // phi($a)[3,1]
       $a=5;
       println($a); // 5
}
println($a); // phi[2, 5]
`,
			[]string{
				"1", "2",
				"1", "3",
				"phi($a)[3,1]", "5",
				"phi($a)[2,5]",
			},
			t)
	})
	t.Run("html-switch", func(t *testing.T) {
		code := `<?php switch($a): case 1: // without semicolon?>
        1;
    <?php break ?>
    <?php case 2: ?>
        2;
    <?php break;?>
    <?php case 3: ?>
        3;
    <?php break;?>
<?php endswitch; ?>`
		ssatest.CheckError(t, ssatest.TestCase{
			Code: code,
			Want: []string{ssa.ValueUndefined("$a")},
		})
	})
}
func TestExpression_Loop(t *testing.T) {
	t.Run("while", func(t *testing.T) {
		code := `<?php
$a=0;
$i=0;
while ($a<4) {
    $a++;
}
println($a);
`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[0,add($a, 1)]"}, t)
	})
	t.Run("do-while-custom", func(t *testing.T) {
		code := `<?php
$a = 0;
do {
    $a=1;
} while (false);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[0,1]"}, t)
	})
	t.Run("do-while-true-other", func(t *testing.T) {
		code := `<?php
$a = 0;
do {
    if($b){
        $a=1;
    }else{
        $a=2;
    }
} while (true);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[0,phi($a)[1,2]]"}, t)
	})
	t.Run("do-while-condition", func(t *testing.T) {
		code := `<?php
$a = 0;
do {
    if($b){
        $a=1;
    }else{
        $a=0;
    }
$a++;
} while ($a>3);
println($a);`
		ssatest.CheckPrintlnValue(code, []string{"phi($a)[0,add(phi($a)[1,0], 1)]"}, t)
	})
}

func TestExpression_Try(t *testing.T) {
	t.Run("simple, no final", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
		$a = 1;
		try {
			$a = 2;
			println($a); // 2
		} catch (Exception $e) {
			println($a); // phi(2, 1)
			$a = 3;
			println($a); // 3
		}
		println($a); // phi(2, 3)
		`, []string{
			"2", "phi($a)[2,1]", "3", "phi($a)[2,3]",
		}, t)
	})

	t.Run("simple, with final", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
		$a = 1;
		try {
			$a = 2;
			println($a); // 2
		} catch (ArrayIndexOutOfBoundsException $e) {
			println($a); // phi(2, 1)
			$a = 3;
			println($a); // 3
		} finally {
			println($a); // phi(2, 3)
			$a = 4;
		}
		println($a); // 4
		`, []string{
			"2", "phi($a)[2,1]", "3", "phi($a)[2,3]", "4",
		}, t)
	})

	t.Run("simple, has error ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
		$a = 1;
		try {
		} catch (Exception $e) {
			println($e); 
		}
		println($e); 
		`, []string{
			"Parameter-$e", "Undefined-$e",
		}, t)
	})

	t.Run("multiple catch, no final", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
		$a = 1;
		try {
			$a = 2;
			println($a); // 2
		} catch (ArrayIndexOutOfBoundsException $e) {
			println($a); // phi(2, 1)
			$a = 3;
			println($a); // 3
		} catch (Exception $e) {
			println($a); // phi(2, 1)
			$a = 4;
			println($a); // 4
		}
		println($a); // phi(2, 3, 4)
		`, []string{
			"2", "phi($a)[2,1]", "3", "phi($a)[2,1]", "4", "phi($a)[2,3,4]",
		}, t)
	})

	t.Run("multiple catch, with final", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`<?php
		$a = 1;
		try {
			$a = 2;
			println($a); // 2
		} catch (ArrayIndexOutOfBoundsException $e) {
			println($a); // phi(2, 1)
			$a = 3;
			println($a); // 3
		} catch (Exception $e) {
			println($a); // phi(2, 1)
			$a = 4;
			println($a); // 4
		} finally {
			println($a); // phi(2, 3, 4)
			$a = 5;
		}
		println($a); // 5
		`, []string{
			"2", "phi($a)[2,1]", "3", "phi($a)[2,1]", "4", "phi($a)[2,3,4]", "5",
		}, t)
	})
}
