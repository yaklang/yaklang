package php2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"testing"
)

func TestParseSSA_FuncCall(t *testing.T) {
	smokingtest(`<?php
$b = 1+a()+1;
`)
}

func TestParseSSA_DoWhileTag(t *testing.T) {
	smokingtest(`<?php
function funcName() {
	echo "a called";
	return 2;
}
do{ print 2; } while (funcName() == 1);

`)
}

func TestParseSSA_WhileTag(t *testing.T) {
	smokingtest(`
<ul>
<?php while ($i <= 5) : ?>
    <li>Item <?php echo $i; ?></li>
    <?php $i++; ?>
<?php endwhile; ?>
</ul>

`)
}

func TestParseSSA_While(t *testing.T) {
	smokingtest(`<?php
while ($i < 5) {
	echo 1;
}

while ($i < 5) {
	if(true) {break};

	if (false) {continue};
}
`)
}

func TestParseSSA_IF(t *testing.T) {
	smokingtest(`<?php

			if (true) echo "abc";
			if (true) echo "abc"; else if (trye) 1+1;
			if (true) echo "abc"; else if (true) 1+1; else "abc"."ccc";

if ($a > 0) {
echo "abc"
}
`)
}

func TestParseSSA_Function_1(t *testing.T) {
	smokingtest(`<?php
function testFunction2($a, $b='1', $c=array(1,2,3,), string $d) {
	1&&1;
	return 1;
}

function testFunction1($a, $b='1', $c=array(1,2,3,), string $d) {
	1&&1;
	return 1;
}
`)
}

func TestParseSSA_Function(t *testing.T) {
	smokingtest(`<?php

function testFunction() {
	1&&1
}
`)
}

func TestParseSSA_YieldName(t *testing.T) {
	smokingtest(`<?php
function gen() {
	forearch (range(1, 10) as $1) {
		yield $1;
	}
}

foreach (gen() as $val) {
	echo $val;
}

`)
}

func TestParseSSA_Valid(t *testing.T) {
	p := ParseSSA(`<?php 
$b = "a"."b";
$b = 1+1;

`, nil)
	p.Show()
	ins := p.GetFunctionFast().FirstBlockInstruction()
	_ = ins
	if len(ins) != 2 {
		t.Fatal("build ins failed: count")
	}
	if ins[1].(*ssa.ConstInst).Const.String() != "2" {
		t.Fatal("build ins failed: 1+1")
	}
	t.Log("-")
}

func smokingtest(code string) *ssa.Program {
	return ParseSSA(code, nil).Show()
}

func TestParseSSA_SMOKING_1(t *testing.T) {
	smokingtest(`<?php 
++$a;--$a;$b++;$c++;`)
}

func TestParseSSA_unpack(t *testing.T) {
	smokingtest(`<?php
[$a, $v] = array(1,2);
`)
}

func TestParseSSA_Spaceship(t *testing.T) {
	smokingtest(`<?php
1 <=> 1;
0 <=> 1;
1 <=> 0;

1|1;
2^1;
1&1;
1&&1;
2||2;

a?b:c;
1?:3;
1??1;

1<<1;
1>>1;
1>1;
1<1;
1==1;
1>=1;
2<=1;
1===1;
1!==1;
1!=1;

`)
}

func TestParseSSA_SMOKING_if(t *testing.T) {
	smokingtest(`<?php 
true and false;
false or false;
false xor true;
`)
}

func TestParseSSA_SMOKING(t *testing.T) {
	p := ParseSSA(`<?php
abc[1]

(bool)1;
(int8)1;
(int16)1;
(int)1;
(uint)1;
(int64)1;
(double)1;
(real)1;
(float)1;
(string)1;
(binary)1;
(unicode)1;
(array)1;
(object)1;
(unset)1;
(resource)1;
(any)1;
(null)1;

~$a;
@$a();
+(1+1);
-(1-1);
!(1+1)


`, nil)
	p.Show()
}

func TestParseSSA_AssignOp(t *testing.T) {
	p := ParseSSA(`<?php 

$a = 1+1;
$emptyVal = null;
$emptyVal ??= 1+1;
$a += 1;
$b -= 1;
$c *= 1;
$e **= 6;
$d /= 1;
$e = "bbb";
$e .= "c";
$f %= 1;
$g &= 1;
$h |= 1;
$i ^= 1;
$j <<= 1;
$k >>= 1;

$c[1]
$c[]
c[0]


`, nil)
	p.Show()
	ins := p.GetFunctionFast().FirstBlockInstruction()
	_ = ins
	//if len(ins) != 2 {
	//	t.Fatal("build ins failed: count")
	//}
	//if ins[1].(*ssa.ConstInst).Const.String() != "2" {
	//	t.Fatal("build ins failed: 1+1")
	//}
	t.Log("-")
}

func TestParseSSA_Valid1(t *testing.T) {
	p := ParseSSA(`<?php 
// 声明一个数组
$array = array("apple", "banana", "cherry");

// 访问数组中的元素
echo $array[0]; // 输出 "apple"
echo $array[1]; // 输出 "banana"
echo $array[2]; // 输出 "cherry"

`, nil)
	p.Show()
	ins := p.GetFunctionFast().FirstBlockInstruction()
	_ = ins
	//if len(ins) != 2 {
	//	t.Fatal("build ins failed: count")
	//}
	//if ins[1].(*ssa.ConstInst).Const.String() != "2" {
	//	t.Fatal("build ins failed: 1+1")
	//}
	t.Log("-")
}

func TestParseSSA_Smoking(t *testing.T) {
	ParseSSA(`<?php echo 111 ?>`, nil)
}

func TestParseSSA_Smoking2(t *testing.T) {
	ParseSSA(`<?php echo "Hello world"; // comment ?>
`, nil)
}

func TestParse_BASIC_EXPR(t *testing.T) {
	ParseSSA(`<?php

1+1;
"a"."cccc";

$a = 1+1;
$b = 1+1+$a;


`, nil)
}

func TestParseCLS(t *testing.T) {
	ParseSSA(`<?php

class A {
	private $num;

	public function __construct($num) {
		this.$num = $num;
	}

	public function getNum() {
		return this.$num;
	}
}

$a = new A(1);
echo $a;

/*
	build a named struct as object template
*/
`, nil)
}

func TestParseSSA_1(t *testing.T) {
	ParseSSA(`<?php

id: 
	echo "test123";

{
	echo "11";
}

if (true) echo "abc";
if (true) echo "abc"; else if true 1+1;
if (true) echo "abc"; else if true 1+1; else "abc"."ccc";

$a=1;

if ($a > 0) echo "abc";
if ($a > 0) echo "abc"; else echo "ghi";
if ($a > 0) echo "abc"; else if ($a < 0) echo "def"; else echo "ghi";



?>

<?php
$condition = true;
$anotherCondition = false;

if ($condition):
    echo "Condition is true.";
elseif ($anotherCondition):
    echo "Another condition is true.";
else:
    echo "Both conditions are false.";
endif;
?>

<?php if ($condition): ?>
    <p>The condition is true.</p>
<?php elseif ($anotherCondition): ?>
    <p>Another condition is true.</p>
<?php else: ?>
    <p>Both conditions are false.</p>
<?php endif; ?>

`, nil)
}
