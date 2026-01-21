function fib(n) {
    if (n <= 2) {
        return 1;
    }
    return fib(n-1) + fib(n-2);
}

function factorial(n) {
    let result = 1;
    for (let i = 1; i <= n; i++) {
        result = result * i;
    }
    return result;
}

function sumRange(start, end) {
    let sum = 0;
    for (let i = start; i <= end; i++) {
        sum = sum + i;
    }
    return sum;
}

function check() {
    const a = fib(7);
    const b = factorial(5);
    const c = sumRange(1, 10);
    return a + b + c;
}
