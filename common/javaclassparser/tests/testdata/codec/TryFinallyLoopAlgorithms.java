package codec;

/**
 * TryFinallyLoopAlgorithms - a self-hosted battery for PLAIN try/catch/finally (no try-with-resources)
 * whose protected body is a LOOP and whose NORMAL completion must leave the try, run the finally, and
 * continue/return AFTER the loop. javac desugars a `finally` by INLINING a copy of the finally code on
 * every exit edge (normal fall-out, each catch, and a synthetic catch-all `any` handler that rethrows),
 * so the same statements appear several times guarded by a multi-region exception table.
 *
 * This is the non-try-with-resources twin of the shape behind "Bug U": the loop structurer used to treat
 * the synthetic exception-handler edge as a normal loop exit - either rewriting it into a `break` (which
 * dropped the catch and leaked the caught-exception placeholder) or fabricating a spurious second loop
 * exit that collapsed the computed loop-end onto the method's shared `return`, so the loop lost its real
 * `break` and spun forever on already-consumed state. Each method here forces the loop's normal exit to
 * flow past the try into a finally and then a post-try continuation, so any regression of that fix
 * changes the differential fingerprint.
 *
 * Opcode intent: athrow + multi-region exception tables (try / catch / synthetic any), astore of caught
 * Throwable, goto-based finally inlining, int/long arithmetic, array + String index loops, labeled
 * break/continue across nested loops inside a try.
 */
public class TryFinallyLoopAlgorithms {

    // try/finally, for loop that may throw; finally folds an accumulator, return is AFTER the loop+finally
    // 关键词: try-finally, 循环正常退出后跑 finally, 异常处理器边非循环退出边
    public static int sumWithFinally(int[] a) {
        int sum = 0;
        int touched = 0;
        try {
            for (int i = 0; i < a.length; i++) {
                if (a[i] < 0) {
                    throw new RuntimeException("neg");
                }
                sum += a[i];
            }
        } finally {
            touched = sum * 7 + a.length;
        }
        return touched;
    }

    // try/catch/finally, while loop scanning a String; normal exit reaches finally then post-try return
    // 关键词: try-catch-finally, while 循环, finally 内联多副本
    public static int scan(String s) {
        int acc = 0;
        int n = 0;
        try {
            int i = 0;
            while (i < s.length()) {
                char c = s.charAt(i);
                acc = acc * 31 + c;
                i++;
            }
        } catch (RuntimeException e) {
            acc = -1;
        } finally {
            n = acc + s.length();
        }
        return n;
    }

    // nested loops inside a try with a labeled break to the outer loop; finally runs on normal exit
    // 关键词: try-finally, 嵌套循环带标签 break, 正常退出流向 finally
    public static int findPair(int[] a, int target) {
        int found = -1;
        int probes = 0;
        try {
            outer:
            for (int i = 0; i < a.length; i++) {
                for (int j = i + 1; j < a.length; j++) {
                    probes++;
                    if (a[i] + a[j] == target) {
                        found = i * 100 + j;
                        break outer;
                    }
                }
            }
        } finally {
            found = found * 1000 + probes;
        }
        return found;
    }

    // long accumulator with a try/finally loop; exercises 64-bit ladd/lmul on the finally path
    // 关键词: try-finally, long 累加, lmul/ladd 在 finally 路径
    public static long hashLong(byte[] data) {
        long h = 1125899906842597L;
        long mixed = 0L;
        try {
            for (int i = 0; i < data.length; i++) {
                h = h * 31L + (data[i] & 0xff);
            }
        } finally {
            mixed = h ^ (h >>> 32) ^ (long) data.length;
        }
        return mixed;
    }

    // finally body itself contains a loop; both the try-loop and the finally-loop must structure
    // 关键词: try-finally, finally 体内含循环
    public static int finallyHasLoop(int n) {
        int product = 1;
        int sumDigits = 0;
        try {
            for (int i = 1; i <= n; i++) {
                product = (product * i) & 0x7fffffff;
            }
        } finally {
            int v = product;
            while (v > 0) {
                sumDigits += v % 10;
                v /= 10;
            }
        }
        return product * 31 + sumDigits;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(sumWithFinally(new int[]{1, 2, 3, 4, 5})).append(',');
        sb.append(scan("hello, try-finally")).append(',');
        sb.append(findPair(new int[]{2, 7, 11, 15, 4, 9}, 13)).append(',');
        sb.append(hashLong("try-finally-loop".getBytes())).append(',');
        sb.append(finallyHasLoop(12));
        System.out.println(sb);
    }
}
