<?php
function fib($n) {
    if ($n <= 2) {
        return 1;
    }
    return fib($n-1) + fib($n-2);
}

function factorial($n) {
    $result = 1;
    for ($i = 1; $i <= $n; $i++) {
        $result = $result * $i;
    }
    return $result;
}

function sumRange($start, $end) {
    $sum = 0;
    for ($i = $start; $i <= $end; $i++) {
        $sum = $sum + $i;
    }
    return $sum;
}

function check() {
    $a = fib(7);
    $b = factorial(5);
    $c = sumRange(1, 10);
    return $a + $b + $c;
}
?>
