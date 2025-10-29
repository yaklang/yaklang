package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_SyntaxPhp(t *testing.T) {
	t.Run("test-1", func(t *testing.T) {
		code := `<?php { ?> echo 1; <?php } ?>`
		test.MockSSA(t, code)
	})
	t.Run("test-2", func(t *testing.T) {
		code := `<script>
    <?php  ?>
</script>`
		test.MockSSA(t, code)
	})
	t.Run("test-3", func(t *testing.T) {
		code := `<html><?php ?></html>`
		test.MockSSA(t, code)
	})
	t.Run("test", func(t *testing.T) {
		code := `
		<?php 
		function f() {
		}
		function d($a) {
			$a();
		}
		function b() {
			d(f);
		}
		`

		test.NonStrictMockSSA(t, code)
	})
	t.Run("test-4", func(t *testing.T) {
		code := `<?php for ($i=0; $i < 5; $i++) { ?>
<script>echo 1;</script>
<?php echo $i;}?>`
		test.CheckError(t, test.TestCase{
			Code: code,
		})
		test.MockSSA(t, code)
	})
	t.Run("test-5", func(t *testing.T) {
		code := `<?php for ($i=0; $i < 5; $i++){ ?><?php }?>`
		test.MockSSA(t, code)
	})
}
func TestParseSSA_Basic(t *testing.T) {
	code := `<?php
$ancasdfasdfasdf;
1+a()+1;
"1"."2";
$c=[1,2,3,];
($b[1] = "1"."2");
($b[1] = "1"."abc");
array(1, "2", "key" => "value");
`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_Basic2(t *testing.T) {
	validateSource(t, "TestParseSSA_Basic2", `<?php
// PHPALLINONE.php: A simplified PHP file containing various syntax elements for compiler testing.

// Basic variable declaration and printing
$name = "PHP Compiler";
echo "Hello, $name!\n";

// Arrays and foreach loop
$numbers = [1, 2, 3, 4, 5];
foreach ($numbers as $number) {
    echo $number . " ";
}
echo "\n";

// Associative array and for loop
$fruits = ['apple' => 'red', 'banana' => 'yellow', 'grape' => 'purple'];
for ($i = 0; $i < count(array_keys($fruits)); $i++) {
    echo array_keys($fruits)[$i] . " is " . array_values($fruits)[$i] . "\n";
}

// Functions
function greet($name) {
    return "Hello, $name!\n";
}
echo greet("World");

// Classes and Objects
class Greeter {
    private $greeting = "Hello";

    public function greet($name) {
        return $this->greeting . ", $name!\n";
    }
}
$greeter = new Greeter();
echo $greeter->greet("OOP World");

// Inheritance
class PoliteGreeter extends Greeter {
    public function greet($name) {
        return parent::greet($name) . "How are you?\n";
    }
}
$politeGreeter = new PoliteGreeter();
echo $politeGreeter->greet("Polite World");

// Interfaces
interface Logger {
    public function log($message);
}
class EchoLogger implements Logger {
    public function log($message) {
        echo $message . "\n";
    }
}
$logger = new EchoLogger();
$logger->log("This is a log message.");

// Traits
trait SayGoodbye {
    public function goodbye($name) {
        return "Goodbye, $name!\n";
    }
}
class FarewellGreeter {
    use SayGoodbye;
}
$farewell = new FarewellGreeter();
echo $farewell->goodbye("Trait World");

// Anonymous functions and closures
$goodbyeFunction = function ($name) {
    return "Goodbye, $name!\n";
};
echo $goodbyeFunction("Anonymous World");

// Try and Catch
try {
    throw new Exception("Just testing exceptions.");
} catch (Exception $e) {
    echo "Caught exception: " . $e->getMessage() . "\n";
}

// Final message
echo "This concludes the basic syntax test.\n";`)
}

func TestParseSSA_RightValue(t *testing.T) {
	code := `<?php
a($b[0]); `
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.PHP))

}

func TestParseSSA_DoWhileTag(t *testing.T) {
	test.Check(t, `<?php
	function funcName() {
		echo "a called";
		return 2;
	}
	do{ print 2; } while (funcName() == 1);
	`, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_WhileTag(t *testing.T) {
	//	test.MockSSA(t, `
	//<ul>
	//<?php while ($i <= 5) : ?>
	//    <li>ComparisonItem <?php echo $i; ?></li>
	//    <?php $i++; ?>
	//<?php endwhile; ?>
	//</ul>
	//`)
}

func TestParseSSA_While(t *testing.T) {
	code := `<?php
while ($i < 5) {
	if(true) {break;};
	if (false) {continue;};
}`
	test.NonStrictMockSSA(t, code)
}

func TestParseSSA_Break(t *testing.T) {
	test.MockSSA(t, `<?php
for(;;){echo 1;break;}
`)
}
func TestParseSSA_IF(t *testing.T) {
	code := `<?php
			if (true) echo "abc";
			if (true) echo "abc"; else if (true) 1+1;
			if (true) echo "abc"; else if (true) 1+1; else "abc"."ccc";

if ($a > 0) {
echo "abc";
}`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_TryCatch(t *testing.T) {
	test.MockSSA(t, `<?php
try {
    echo 1;
}finally{
    echo 2;
}`)
}

func TestParseSSA_YieldName(t *testing.T) {
	//	test.MockSSA(t, `<?php
	//
	//function gen() {
	//   foreach (range(1, 10) as $value) {
	//       yield $value;
	//   }
	//}
	//
	//foreach (gen() as $val) {
	//   echo $val;
	//}
	//
	//`)
}

func TestParseSSA_Valid(t *testing.T) {
	// 	p := test.MockSSA(t, `<?php
	// $b = "a"."b";
	// $b = 1+1;

	// `)
	//
	//	p.Show()
	//	ins := p.GetFunctionFast().FirstBlockInstruction()
	//	_ = ins
	//	if len(ins) != 2 {
	//		t.Fatal("build ins failed: count")
	//	}
	//	if ins[1].(*ssa.ConstInst).Const.String() != "2" {
	//		t.Fatal("build ins failed: 1+1")
	//	}
	//	t.Log("-")
}

// func test.MockSSA(t, code string) *ssa.Program {
// 	return test.MockSSA(t, code, nil).Show()
// }

func TestParseSSA_SMOKING_1(t *testing.T) {
	code := `
	<?php 
++$a;--$a;$b++;$c++;
`
	test.CheckError(t, test.TestCase{
		Code: code,
		Want: []string{ssa.ValueUndefined("$a"), ssa.ValueUndefined("$b"), ssa.ValueUndefined("$c")},
	})
}

func TestParseSSA_unpack(t *testing.T) {
	//	test.MockSSA(t, `<?php
	//[$a, $v] = array(1,2);
	//`)
}

func TestParseSSA_Spaceship(t *testing.T) {
	code := `<?php
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
1!=1;`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_SMOKING_if(t *testing.T) {
	code := `<?php 
true and false;
false or false;
false xor true;
`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_SMOKING(t *testing.T) {
	code := `<?php
(bool)1;
(int)1;1;
(double)1;
(real)1;
(float)1;
(string)1;
(binary)1;1;
(array)1;
(object)1;
(unset)1;;

~$a;
@$a();
+(1+1);
-(1-1);
!(1+1);`
	test.CheckError(t, test.TestCase{
		Code: code,
		Want: []string{ssa.ValueUndefined("$a"), ssa.ValueUndefined("$a")},
	})
}

func TestParseSSA_AssignOp(t *testing.T) {
	code := `<?php 
$a = 1+1;
$emptyVal = null;
$emptyVal = 1+1;
$a += 1;
$b -= 1;
$c *= 2;
$e **= 6;
$d /= 1;
$e = "bbb";
$e .= "c";
$f %= 1;
$g &= 1;
$h |= 1;
$i ^= 1;
$j <<= 1;
$k >>= 1;`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestParseSSA_Valid1(t *testing.T) {
	test.MockSSA(t, `<?php 
// 声明一个数组
$array = array("apple", "banana", "cherry");

// 访问数组中的元素
echo $array[0]; // 输出 "apple"
echo $array[1]; // 输出 "banana"
echo $array[2]; // 输出 "cherry"

`)

}

func TestParseSSA_Smoking(t *testing.T) {
	test.MockSSA(t, `<?php echo 111 ?>`)
}

func TestParseSSA_Smoking2(t *testing.T) {
	test.MockSSA(t, `<?php echo "Hello world"; // comment ?>
`)
}

func TestParse_BASIC_EXPR(t *testing.T) {
	test.MockSSA(t, `<?php

1+1;
"a"."cccc";

$a = 1+1;
$b = 1+1+$a;


`)
}

func TestParseSSA_1(t *testing.T) {
	code := `<?php

id: 
	echo "test123";

{
	echo "11";
}

if (true) echo "abc";
if (true) echo "abc"; else if (true) 1+1;
if (true) echo "abc"; else if (true) 1+1; else "abc"."ccc";

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
<?php endif; ?>`
	test.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	},
		ssaapi.WithLanguage(ssaconfig.PHP))
}
