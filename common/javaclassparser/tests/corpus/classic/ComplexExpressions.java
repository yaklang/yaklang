// Complex-expression corpus: array initializers (1-D and 2-D), mixed numeric
// promotion (int/long/float/double), StringBuilder-style concatenation, chained
// ternaries, recursion, enhanced-for over arrays, and multi-operand arithmetic.
// These stress operand typing, implicit widening, array construction and string
// concat reconstruction together rather than in isolation.
public class ComplexExpressions {
    public int arrayInit1D() {
        int[] a = {1, 2, 3, 4, 5};
        int sum = 0;
        for (int v : a) {
            sum += v;
        }
        return sum;
    }

    public int arrayInit2D() {
        int[][] m = {{1, 2, 3}, {4, 5, 6}, {7, 8, 9}};
        int diag = 0;
        for (int i = 0; i < m.length; i++) {
            diag += m[i][i];
        }
        return diag;
    }

    public double mixedPromotion(int i, long l, float f, double d) {
        double r = i + l;
        r = r * f;
        r = r - d;
        r = r / 2;
        return r + i * l - f / d;
    }

    public String concat(int n, String name, boolean flag) {
        StringBuilder sb = new StringBuilder();
        sb.append("n=").append(n).append(",name=").append(name).append(",flag=").append(flag);
        return sb.toString();
    }

    public String plusConcat(int a, double b, char c, Object o) {
        return "a=" + a + " b=" + b + " c=" + c + " o=" + o;
    }

    public int chainedTernary(int x) {
        return x < 0 ? -1 : x == 0 ? 0 : x < 10 ? 1 : x < 100 ? 2 : 3;
    }

    public long factorial(int n) {
        if (n <= 1) {
            return 1L;
        }
        return n * factorial(n - 1);
    }

    public int fib(int n) {
        if (n < 2) {
            return n;
        }
        return fib(n - 1) + fib(n - 2);
    }

    public int polyEval(int[] coeffs, int x) {
        int result = 0;
        for (int i = 0; i < coeffs.length; i++) {
            int term = coeffs[i];
            for (int p = 0; p < i; p++) {
                term *= x;
            }
            result += term;
        }
        return result;
    }

    public boolean inRange(int v, int lo, int hi) {
        return v >= lo && v <= hi && (v - lo) % 2 == 0;
    }

    public int sumArgs(int first, int... rest) {
        int total = first;
        for (int v : rest) {
            total += v;
        }
        return total;
    }
}
