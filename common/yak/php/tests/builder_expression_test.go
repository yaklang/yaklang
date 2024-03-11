package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func TestPhpPrintln(t *testing.T) {
	c := `<?php $a=1; println($a);`
	ssatest.CheckPrintf(t, ssatest.TestCase{Want: []string{"1"}, Code: c})
}

func TestExpression_DynamicVariable(t *testing.T) {
	code := `<?php
// Variable expression and dynamic variable expression
$identifier = "dynamicVar";
$$identifier = "Hello, dynamic!";
`
	ssatest.MockSSA(t, code)
}
func TestExpression_DynamicVariable_2(t *testing.T) {
	code := `<?php
// Variable expression and dynamic variable expression
$identifier = "dynamicVar";
$dynamicVar = "test";
$test="1";
$$$identifier=123;

echo $test;`
	ssatest.MockSSA(t, code)
}

func TestExpression_If1(t *testing.T) {
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
		ssatest.MockSSA(t, `<?php
switch($a):;
   case 1:
       echo 1;
   default:
       echo 1;
endswitch;
`)
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
			Want: []string{"2", "3", "4", "phi($a)[2,3,4,1]"},
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
		ssatest.CheckPrintlnValue(code, []string{"2", "3", "4", "5", "phi($a)[2,3,4,5,1]"}, t)
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
		ssatest.CheckPrintlnValue(code, []string{}, t)
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
		ssatest.CheckPrintlnValue(code, []string{}, t)
	})
}

func TestExpressionAllInONE(t *testing.T) {
	code := `<?php
// Clone expression
$originalObject = new stdClass;
$clonedObject = clone $originalObject;

// New expression
$newObject = new stdClass;

// Variable name expression
$varName = "test";

// Variable expression and dynamic variable expression
$identifier = "dynamicVar";
$$identifier = "Hello, dynamic!";

// Array creation expression
$array = [1, 2, 3];
$associativeArray = ["a" => 1, "b" => 2];

// Print expression
print "Hello, world!\n";

// Scalar expressions
$constant = PHP_INT_MAX;
$string = "string";
$label = true;

// BackQuoteString expression
$shellOutput = ` + "`ls -al`" + `;

// Parenthesis expression
$parenthesis = (5 * 3) + 2;

// Yield expression
function generator() {
    yield 1;
    yield 2;
}

// Special word expressions (List, IsSet, Empty, Eval, Exit, Include, Require)
list($a, $b) = [1, 2];
$issetExample = isset($a);
$emptyExample = empty($nonexistent);
eval('$evalResult = "Evaluated";');
// Exit; // Uncommenting this will stop the script
include 'nonexistentfile.php'; // Warning suppressed with @
require 'anothernonexistentfile.php'; // Warning suppressed with @

// Lambda function expression
$lambda = function($x) { return $x * 2; };
echo $lambda(5);

// Match expression (PHP 8+)
$matchResult = match($a) {
    1 => 'one',
    2 => 'two',
    default => 'other',
};

// Cast expression
$casted = (int) "123";

// Unary operator expression
$unary = ~$a; // bitwise NOT
$negation = !$issetExample;

// Arithmetic expression
$sum = 1 + 2;
$product = 2 * $a;
$exponentiation = 2 ** 3;

// InstanceOf expression
$instanceOfExample = $newObject instanceof stdClass;

// Bitwise expressions
$and = $a & $b;
$or = $a | $b;
$xor = $a ^ $b;

// Logical expressions
$logicalAnd = $a && $b;
$logicalOr = $a || $b;
$logicalXor = $a xor $b; // Note: 'xor' is not as commonly used

// Conditional expression
$ternary = $a == 2 ? "equals 2" : "does not equal 2";

// Null coalescing expression
$nullCoalesce = $undefinedVar ?? "default";

// Spaceship expression
$spaceship = 1 <=> 2;

// Throw expression (PHP 7.1+)
// throw new Exception("This is an exception");

// Assignment operator expression
$a += 5;

// LogicalAnd, LogicalOr, LogicalXor
$logicalAndSimple = $a and $b;
$logicalOrSimple = $a or $b;
$logicalXorSimple = $a xor $b;

echo "Script execution completed.\n";
`
	_ = code
	// ssatest.MockSSA(t, code)
}
