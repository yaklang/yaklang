<?php
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
$shellOutput = `ls -al`;

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
