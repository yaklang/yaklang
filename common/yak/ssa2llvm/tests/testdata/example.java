public class Example {
    public static int fib(int n) {
        if (n <= 2) {
            return 1;
        }
        return fib(n-1) + fib(n-2);
    }

    public static int factorial(int n) {
        int result = 1;
        for (int i = 1; i <= n; i++) {
            result = result * i;
        }
        return result;
    }

    public static int sumRange(int start, int end) {
        int sum = 0;
        for (int i = start; i <= end; i++) {
            sum = sum + i;
        }
        return sum;
    }

    public static int check() {
        int a = fib(7);
        int b = factorial(5);
        int c = sumRange(1, 10);
        return a + b + c;
    }
}
