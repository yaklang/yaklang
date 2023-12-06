package php2ssa

import "testing"

func TestParseSSA_Smoking(t *testing.T) {
	ParseSSA(`<?php echo 111 ?>`, nil)
}

func TestParseSSA_Smoking2(t *testing.T) {
	ParseSSA(`<?php echo "Hello world"; // comment ?>
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
