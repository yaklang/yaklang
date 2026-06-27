package codec;

/**
 * SignedArithmeticAlgorithms - a Guava IntMath/LongMath + Hacker's-Delight style battery that stresses
 * SIGNED integer/long arithmetic and, in particular, the rendering of a unary minus over a compound
 * sub-expression (the `-(a + b)` family). The latter is a permanent regression lock for the
 * unary-minus parenthesisation fix (Bug P): the JVM emits `<expr> ; ineg`, which must render as
 * `-(<expr>)`, never `-<expr>` (which re-associates, e.g. `-(a+b)` -> `(-a)+b`, silently changing the
 * value). Every result is folded into a deterministic fingerprint and the differential-execution
 * oracle (compile -> decompile -> recompile -> run -> compare) fails on any divergence.
 *
 * Deliberately avoids the still-open structuring bugs (CODEC_TODO.md): no while-loop-then-ternary on a
 * loop variable (Bug N), no labeled break/continue (Bug O), no nested loop inside an if/else guard
 * (Bug Q). Loops here are flat single while/for; abs/sign use plain if, not a trailing ternary.
 */
public class SignedArithmeticAlgorithms {

    // -(a + b): negate a sum. Must render as -((a) + (b)).
    static int negSum(int a, int b) {
        return -(a + b);
    }

    // -(a * b + c): negate a sum-of-product. Nested compound under one unary minus.
    static int negMulAdd(int a, int b, int c) {
        return -(a * b + c);
    }

    // -(a - b): negate a difference (== b - a).
    static long negDiff(long a, long b) {
        return -(a - b);
    }

    // -(-a + b): negate an expression whose left operand is itself negated.
    static int negNegSum(int a, int b) {
        return -(-a + b);
    }

    // Branchless absolute value via sign-smear, then a checked negate path. No trailing ternary.
    static int abs(int x) {
        if (x < 0) {
            return -x;
        }
        return x;
    }

    // Sign function (-1 / 0 / 1) using plain branches.
    static int signum(int x) {
        if (x > 0) {
            return 1;
        }
        if (x < 0) {
            return -1;
        }
        return 0;
    }

    // Two's-complement low-bit isolation and clear, plus negation identities.
    static int lowestSetBit(int x) {
        return x & (-x);
    }

    static int clearLowestSetBit(int x) {
        return x & (x - 1);
    }

    // Floor division matching Math.floorDiv for negative operands (correction term uses a negated sum).
    static int floorDiv(int a, int b) {
        int q = a / b;
        if ((a ^ b) < 0 && q * b != a) {
            q = q - 1;
        }
        return q;
    }

    static int floorMod(int a, int b) {
        int r = a % b;
        if (r != 0 && (r ^ b) < 0) {
            r = r + b;
        }
        return r;
    }

    // Overflow-safe midpoint of two ints (Hacker's Delight): (a & b) + ((a ^ b) >> 1).
    static int midpoint(int a, int b) {
        return (a & b) + ((a ^ b) >> 1);
    }

    // Saturated addition: clamp to int range instead of wrapping.
    static int saturatedAdd(int a, int b) {
        long sum = (long) a + (long) b;
        if (sum > 2147483647L) {
            return 2147483647;
        }
        if (sum < -2147483648L) {
            return -2147483648;
        }
        return (int) sum;
    }

    // Euclid gcd via a flat while loop; abs applied with a plain if (NOT a trailing ternary -> Bug N).
    static int gcd(int a, int b) {
        while (b != 0) {
            int t = b;
            b = a % b;
            a = t;
        }
        if (a < 0) {
            a = -a;
        }
        return a;
    }

    // Least common multiple built on gcd; guards a zero input to avoid divide-by-zero.
    static long lcm(int a, int b) {
        if (a == 0 || b == 0) {
            return 0L;
        }
        int g = gcd(a, b);
        long prod = (long) abs(a) * (long) abs(b);
        return prod / g;
    }

    // Integer power by squaring (flat while, no nested guard loop).
    static long ipow(long base, int exp) {
        long result = 1L;
        long b = base;
        int e = exp;
        while (e > 0) {
            if ((e & 1) == 1) {
                result = result * b;
            }
            b = b * b;
            e = e >> 1;
        }
        return result;
    }

    // Negated polynomial evaluation: -(((a*x + b)*x) + c) -- compound nested under a single minus.
    static long negPoly(long x, long a, long b, long c) {
        return -((a * x + b) * x + c);
    }

    static String hex(long v) {
        return Long.toHexString(v);
    }

    // Each section lives in its own method: keeping main() small avoids javac reusing one local slot
    // across many sequential loops, which the decompiler currently mis-scopes (slot-reuse identity bug,
    // CODEC_TODO Bug B/C). The negation/arithmetic methods above are the actual coverage targets.
    static int[][] pairs() {
        return new int[][]{{7, 5}, {-7, 5}, {7, -5}, {-7, -5}, {0, 9}, {2147483647, 1}};
    }

    static void emitNegations(StringBuilder sb) {
        int[][] pairs = pairs();
        for (int i = 0; i < pairs.length; i++) {
            int a = pairs[i][0];
            int b = pairs[i][1];
            sb.append(negSum(a, b)).append(',');
            sb.append(negDiff(a, b)).append(',');
            sb.append(negNegSum(a, b)).append(',');
            sb.append(negMulAdd(a, b, 3)).append(';');
        }
        sb.append('|');
    }

    static void emitBitAndSign(StringBuilder sb) {
        int[] xs = {0, 1, -1, 16, -16, 2147483647, -2147483648, 12345, -98765};
        for (int i = 0; i < xs.length; i++) {
            int x = xs[i];
            sb.append(abs(x)).append(':');
            sb.append(signum(x)).append(':');
            sb.append(lowestSetBit(x)).append(':');
            sb.append(clearLowestSetBit(x)).append(',');
        }
        sb.append('|');
    }

    static void emitFloorMath(StringBuilder sb) {
        int[][] pairs = pairs();
        for (int i = 0; i < pairs.length; i++) {
            int a = pairs[i][0];
            int b = pairs[i][1];
            if (b != 0) {
                sb.append(floorDiv(a, b)).append('/');
                sb.append(floorMod(a, b)).append('/');
                sb.append(midpoint(a, b)).append(';');
            }
        }
        sb.append('|');
    }

    static void emitSaturated(StringBuilder sb) {
        sb.append(saturatedAdd(2000000000, 2000000000)).append(',');
        sb.append(saturatedAdd(-2000000000, -2000000000)).append(',');
        sb.append(saturatedAdd(100, 200)).append('|');
    }

    static void emitGcdLcm(StringBuilder sb) {
        int[][] gl = {{12, 18}, {-24, 36}, {17, 5}, {0, 7}, {100, 75}};
        for (int i = 0; i < gl.length; i++) {
            sb.append(gcd(gl[i][0], gl[i][1])).append('/');
            sb.append(lcm(gl[i][0], gl[i][1])).append(',');
        }
        sb.append('|');
    }

    static void emitPowAndPoly(StringBuilder sb) {
        sb.append(hex(ipow(2L, 10))).append(',');
        sb.append(hex(ipow(3L, 7))).append(',');
        sb.append(hex(ipow(7L, 0))).append(',');
        sb.append(hex(ipow(-2L, 5))).append('|');
        sb.append(negPoly(2L, 3L, 4L, 5L)).append(',');
        sb.append(negPoly(-1L, 1L, 1L, 1L)).append(',');
        sb.append(negPoly(0L, 9L, 8L, 7L));
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        emitNegations(sb);
        emitBitAndSign(sb);
        emitFloorMath(sb);
        emitSaturated(sb);
        emitGcdLcm(sb);
        emitPowAndPoly(sb);
        System.out.println(sb.toString());
    }
}
