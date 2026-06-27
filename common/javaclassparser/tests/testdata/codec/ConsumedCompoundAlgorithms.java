package codec;

/**
 * ConsumedCompoundAlgorithms - exercises compound assignments / increments whose RESULT VALUE IS
 * CONSUMED (the dup_x1/dup_x2/dup2/dup2_x1/dup2_x2 idioms that javac emits when the updated value
 * also feeds another expression). This is the historical "Bug J": the consumer re-evaluated the RHS
 * after the target was already written (`(a[i]+v)*k` instead of `r*k`), double-applying the operator,
 * or folded an iinc-updated local back to its stale literal.
 *
 * Root cause(s) fixed:
 *   - var-fold resolved a copy of a dup-materialized shared temp THROUGH the temp down to its
 *     defining expression, so the copy re-evaluated it. resolveFoldValue now stops at dup-shared temps.
 *   - OP_DUP2_X2 kept a single shared addUser callback (overwritten by the index/arrayref pops), so
 *     the duplicated category-2 value's two consumers never registered and the temp folded away. It
 *     now tracks addUser per item, matching OP_DUP2.
 *
 * Every method here round-trips byte-for-byte (decompile -> recompile -> run identical). Single public
 * top-level class, deterministic, mixes static + one instance to reach the `this`-based dup forms.
 */
public class ConsumedCompoundAlgorithms {

    private static int sIntAcc = 7;
    private static long sLongAcc = 11;

    private int iIntAcc = 100;
    private long iLongAcc = 5;

    // ---- consumed compound on int[] element (dup_x2) ----
    public static long arrIntConsumed(int n) {
        int[] a = new int[n];
        for (int i = 0; i < n; i++) {
            a[i] = i * 3 + 1;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            int r = (a[i] += (i + 5));   // dup_x2: r and a[i] share the updated value
            acc = acc * 131 + r;
            acc = acc * 131 + a[i];
            int s = (a[i] *= 2);
            acc ^= ((long) s << 3) ^ a[i];
        }
        return acc;
    }

    // ---- consumed compound on long[] element (dup2_x2) ----
    public static long arrLongConsumed(int n) {
        long[] a = new long[n];
        for (int i = 0; i < n; i++) {
            a[i] = (long) i * 1000 + 1;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            long r = (a[i] += (long) (i + 7));   // dup2_x2
            acc = acc * 1315423911 + r;
            acc ^= a[i];
            long s = (a[i] ^= 0x5a5a5a5aL);
            acc = acc * 31 + s + a[i];
        }
        return acc;
    }

    // ---- consumed compound on double[] element (dup2_x2, fp) ----
    public static long arrDoubleConsumed(int n) {
        double[] d = new double[n];
        for (int i = 0; i < n; i++) {
            d[i] = i + 0.5;
        }
        double acc = 0;
        for (int i = 0; i < n; i++) {
            double r = (d[i] += 1.5);   // dup2_x2 on a double element
            acc += r * 2.0 + d[i];
            double s = (d[i] *= 3.0);
            acc -= s - d[i];
        }
        return Double.doubleToLongBits(acc);
    }

    // ---- consumed compound on a local int: iinc form (`x += c`) and dup form (`x *= c`) ----
    public static long localIntConsumed(int seed) {
        int x = seed;
        long acc = 0;
        for (int i = 0; i < 16; i++) {
            int r = (x += (i + 1));   // iinc-coded; consumer must see the NEW x, not the seed
            acc = acc * 131 + r;
            acc = acc * 131 + x;
            int s = (x *= 3);         // dup form (not iinc-expressible)
            acc ^= ((long) s << 5) ^ x;
            x %= 1000003;
        }
        return acc;
    }

    // ---- consumed compound on a local long (dup2) ----
    public static long localLongConsumed(long seed) {
        long x = seed;
        long acc = 0;
        for (int i = 0; i < 16; i++) {
            long r = (x += (long) (i * 7 + 1));
            acc = acc * 1000003 + r;
            long s = (x *= 5L);
            acc ^= s + x;
            x %= 0x7fffffffL;
        }
        return acc;
    }

    // ---- consumed compound on a static field, int (dup) and long (dup2) ----
    public static long staticFieldConsumed(int rounds) {
        sIntAcc = 7;
        sLongAcc = 11;
        long acc = 0;
        for (int i = 0; i < rounds; i++) {
            int r = (sIntAcc += i);
            acc = acc * 131 + r + sIntAcc;
            int t = (sIntAcc ^= 0x33);
            acc = acc * 131 + t;
            long u = (sLongAcc *= 3L);
            acc ^= u + sLongAcc;
            sIntAcc %= 100000;
            sLongAcc %= 0x7fffffffL;
        }
        return acc;
    }

    // ---- consumed compound on an instance field, int (dup_x1) and long (dup2_x1) ----
    public long instanceFieldConsumed(int rounds) {
        iIntAcc = 100;
        iLongAcc = 5;
        long acc = 0;
        for (int i = 0; i < rounds; i++) {
            int r = (iIntAcc += (i + 1));   // dup_x1
            acc = acc * 131 + r + iIntAcc;
            long u = (iLongAcc += (long) (i * 3 + 1));   // dup2_x1
            acc ^= u + iLongAcc;
            iIntAcc %= 100000;
            iLongAcc %= 0x7fffffffL;
        }
        return acc;
    }

    public static void main(String[] args) {
        ConsumedCompoundAlgorithms self = new ConsumedCompoundAlgorithms();
        StringBuilder sb = new StringBuilder();
        sb.append(Long.toHexString(arrIntConsumed(13))).append(',');
        sb.append(Long.toHexString(arrLongConsumed(11))).append(',');
        sb.append(Long.toHexString(arrDoubleConsumed(9))).append(',');
        sb.append(Long.toHexString(localIntConsumed(123))).append(',');
        sb.append(Long.toHexString(localLongConsumed(99))).append(',');
        sb.append(Long.toHexString(staticFieldConsumed(20))).append(',');
        sb.append(Long.toHexString(self.instanceFieldConsumed(20)));
        System.out.println(sb);
    }
}
