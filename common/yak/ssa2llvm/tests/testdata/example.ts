function fib(n: number): number {
    if (n <= 2) {
        return 1;
    }
    return fib(n-1) + fib(n-2);
}

function factorial(n: number): number {
    let result: number = 1;
    for (let i: number = 1; i <= n; i++) {
        result = result * i;
    }
    return result;
}

function sumRange(start: number, end: number): number {
    let sum: number = 0;
    for (let i: number = start; i <= end; i++) {
        sum = sum + i;
    }
    return sum;
}

function check(): number {
    const a: number = fib(7);
    const b: number = factorial(5);
    const c: number = sumRange(1, 10);
    return a + b + c;
}
