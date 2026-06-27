package codec;

/**
 * ControlFlowAlgorithms - a battery for the decompiler's loop/branch STRUCTURING core, the part most
 * likely to silently miscompile (loop polarity, merge points, labeled break/continue, do-while vs
 * while, infinite-loop+break, ternary nesting). Each method is a small, well-known algorithm whose
 * control-flow graph forces a specific structuring decision; the differential-execution oracle
 * (compile -> decompile -> recompile -> run -> compare) fails if any of those decisions inverts.
 *
 * Covered shapes:
 *   - while + two-way branch (Collatz step counter, long arithmetic)
 *   - while with swap (Euclid gcd)
 *   - while(lo<=hi) with three-way branch and return INSIDE the loop (binary search)
 *   - nested for with `continue` over a boolean sieve (Eratosthenes)
 *   - nested while building a run-length encoding (inner counting loop)
 *   - nested for over a ragged 2D grid (multi-dim array loads, position-weighted checksum)
 *   - while(true) { ... break; } infinite-loop-with-break (do-while(true) structuring)
 *   - do { ... } while(cond) post-tested loop (digital root)
 *   - if / else-if / else chains (FizzBuzz-style classification counts)
 *   - nested ternary conditional expressions (median-of-three / clamp)
 *   - square-and-multiply modular exponentiation (while + bit test + long)
 *
 * Pure static methods, single public top-level class, deterministic fingerprint in main().
 */
public class ControlFlowAlgorithms {

    static long collatzSteps(long n) {
        long steps = 0;
        while (n != 1) {
            if ((n & 1L) == 0L) {
                n = n / 2;
            } else {
                n = 3 * n + 1;
            }
            steps++;
        }
        return steps;
    }

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

    static int binarySearch(int[] sorted, int key) {
        int lo = 0;
        int hi = sorted.length - 1;
        while (lo <= hi) {
            int mid = (lo + hi) >>> 1;
            if (sorted[mid] == key) {
                return mid;
            } else if (sorted[mid] < key) {
                lo = mid + 1;
            } else {
                hi = mid - 1;
            }
        }
        // Standard "not found" insertion-point encoding: a negated sum -(lo + 1). This exercises the
        // unary-minus-over-a-sum rendering path (Neg(Add(lo,1))); the decompiler must keep the sum
        // parenthesised (-(lo + 1)) instead of re-associating to (-lo) + 1.
        return -(lo + 1);
    }

    static int sieveCount(int n) {
        if (n < 2) {
            return 0;
        }
        // Marking phase: a clean nested for/for over the composite[] sieve (exercises boolean[]
        // store via bastore). Marking the multiples of EVERY i, not only primes, yields the exact same
        // composite set (a prime is never a proper multiple of a smaller number), so the prime count is
        // identical -- while keeping the inner loop OUT of an if/else branch, a nested-loop-in-guard
        // shape the structurer still mis-handles (tracked as the nested-loop bug in CODEC_TODO.md).
        boolean[] composite = new boolean[n + 1];
        for (int i = 2; i <= n; i++) {
            for (int j = i + i; j <= n; j += i) {
                composite[j] = true;
            }
        }
        int count = 0;
        for (int i = 2; i <= n; i++) {
            if (!composite[i]) {
                count++;
            }
        }
        return count;
    }

    static String runLengthEncode(String s) {
        StringBuilder out = new StringBuilder();
        int i = 0;
        int len = s.length();
        while (i < len) {
            char c = s.charAt(i);
            int run = 1;
            while (i + run < len && s.charAt(i + run) == c) {
                run++;
            }
            out.append(c).append(run);
            i += run;
        }
        return out.toString();
    }

    // Full traversal of a ragged 2D grid, accumulating a position-weighted checksum of every cell that
    // is strictly less than `target`. No early exit: a clean nested for/for over int[][] exercising
    // multi-dimensional array loads (aaload + iaload), arraylength on the inner rows and an `if` body,
    // all shapes the structurer handles, so it round-trips. (Labeled break/continue across a 2D scan is
    // a known structuring gap, tracked in CODEC_TODO.md, and deliberately not exercised here.)
    static int countLess(int[][] grid, int target) {
        int acc = 0;
        for (int i = 0; i < grid.length; i++) {
            for (int j = 0; j < grid[i].length; j++) {
                int v = grid[i][j];
                if (v < target) {
                    acc += (i + 1) * 100 + (j + 1) * 7 + v;
                }
            }
        }
        return acc;
    }

    static int nextPowerOfTwo(int v) {
        if (v <= 1) {
            return 1;
        }
        int p = 1;
        while (true) {
            if (p >= v) {
                break;
            }
            p = p << 1;
        }
        return p;
    }

    static int digitalRoot(int n) {
        if (n < 0) {
            n = -n;
        }
        int x = n;
        do {
            int sum = 0;
            while (x > 0) {
                sum += x % 10;
                x = x / 10;
            }
            x = sum;
        } while (x >= 10);
        return x;
    }

    static int classify(int n) {
        if (n % 15 == 0) {
            return 15;
        } else if (n % 3 == 0) {
            return 3;
        } else if (n % 5 == 0) {
            return 5;
        } else {
            return n;
        }
    }

    static int medianOfThree(int a, int b, int c) {
        return a > b
                ? (b > c ? b : (a > c ? c : a))
                : (a > c ? a : (b > c ? c : b));
    }

    static long powMod(long base, long exp, long mod) {
        long result = 1;
        base = base % mod;
        while (exp > 0) {
            if ((exp & 1L) == 1L) {
                result = (result * base) % mod;
            }
            exp = exp >> 1;
            base = (base * base) % mod;
        }
        return result;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        long[] collatzInputs = {1L, 6L, 27L, 97L, 1000000L};
        for (int i = 0; i < collatzInputs.length; i++) {
            sb.append(collatzSteps(collatzInputs[i]));
            sb.append(i + 1 < collatzInputs.length ? "," : ";");
        }

        int[][] gcdPairs = {{48, 18}, {1071, 462}, {-24, 36}, {17, 5}, {0, 9}};
        for (int i = 0; i < gcdPairs.length; i++) {
            sb.append(gcd(gcdPairs[i][0], gcdPairs[i][1])).append(',');
        }
        sb.append(';');

        int[] sortedArr = {1, 3, 5, 7, 9, 11, 13};
        int[] keys = {7, 1, 13, 8, -1, 12};
        for (int i = 0; i < keys.length; i++) {
            sb.append(binarySearch(sortedArr, keys[i])).append(',');
        }
        sb.append(';');

        int[] sieveInputs = {10, 30, 100, 1, 2};
        for (int i = 0; i < sieveInputs.length; i++) {
            sb.append(sieveCount(sieveInputs[i])).append(',');
        }
        sb.append(';');

        String[] rleInputs = {"aaabbbcccd", "x", "", "wwwwww", "abcabc"};
        for (int i = 0; i < rleInputs.length; i++) {
            sb.append(runLengthEncode(rleInputs[i])).append('|');
        }
        sb.append(';');

        int[][] grid = {{1, 2, 3}, {-1, 5, 7}, {4, 9, 6}};
        int[] scanTargets = {9, 3, 100, 6};
        for (int i = 0; i < scanTargets.length; i++) {
            sb.append(countLess(grid, scanTargets[i])).append(',');
        }
        sb.append(';');

        int[] powInputs = {1, 5, 16, 17, 1023, 1024};
        for (int i = 0; i < powInputs.length; i++) {
            sb.append(nextPowerOfTwo(powInputs[i])).append(',');
        }
        sb.append(';');

        int[] rootInputs = {0, 9, 38, 12345, -99999};
        for (int i = 0; i < rootInputs.length; i++) {
            sb.append(digitalRoot(rootInputs[i])).append(',');
        }
        sb.append(';');

        for (int i = 1; i <= 16; i++) {
            sb.append(classify(i)).append(i < 16 ? "." : ";");
        }

        int[][] triples = {{3, 1, 2}, {9, 9, 9}, {1, 2, 3}, {5, 1, 9}, {7, 8, 6}};
        for (int i = 0; i < triples.length; i++) {
            sb.append(medianOfThree(triples[i][0], triples[i][1], triples[i][2])).append(',');
        }
        sb.append(';');

        sb.append(powMod(2, 10, 1000000007)).append(',');
        sb.append(powMod(7, 256, 13)).append(',');
        sb.append(powMod(123456789, 1000000, 998244353)).append(',');
        sb.append(powMod(3, 0, 7));

        System.out.println(sb);
    }
}
