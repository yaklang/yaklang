// Function fixtures: parameters, returns, recursion and default arguments.

int add(int a, int b) {
    return a + b;
}

int twice(int value) {
    return add(value, value);
}

int factorial(int n) {
    if (n <= 1) {
        return 1;
    }
    return n * factorial(n - 1);
}

int addDefault(int a, int b = 10) {
    return a + b;
}

int apply(int (*fn)(int, int), int a, int b) {
    return fn(a, b);
}

int useAll() {
    int total = add(1, 2);
    total += twice(3);
    total += factorial(4);
    total += addDefault(5);
    return total;
}