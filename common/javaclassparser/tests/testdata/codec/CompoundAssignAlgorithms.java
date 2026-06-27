package codec;

/**
 * CompoundAssignAlgorithms - exercises the operand-stack dup family (DUP/DUP_X1/DUP_X2/DUP2/DUP2_X1)
 * that javac emits for compound assignment and increment/decrement, in the forms that the decompiler
 * reconstructs correctly (verified by the round-trip oracle: decompile -> recompile -> run identical).
 *
 * Covered (all round-trip byte-for-byte):
 *   - array element `a[i] op= v;` as a statement, for int/long/double/float/byte/char arrays
 *     (DUP2 / DUP2_X1, plus I2B/I2C narrowing on the byte/char paths)
 *   - pre/post increment & decrement on array elements `a[i]++ / ++a[i] / a[i]-- / --a[i]`
 *   - 2-D array element compound assignment `m[i][j] op= v;`
 *   - static field compound assignment `F op= v;`
 *
 * Compound assignment whose result *value is consumed* (`t = (x ^= v); ... t ...`, the dup_x1/dup_x2/
 * dup2/dup2_x1/dup2_x2 idioms) was the historical "Bug J" and is now fixed; its dedicated round-trip
 * coverage lives in ConsumedCompoundAlgorithms (kept separate to isolate the dup-fold path).
 *
 * Single public top-level class, static only, deterministic.
 */
public class CompoundAssignAlgorithms {

    private static int STATIC_ACC = 7;

    // ---- int[] element compound assignment, every arithmetic/bitwise/shift operator, statement form ----
    public static long intArrayOps(int n) {
        int[] a = new int[n];
        for (int i = 0; i < n; i++) {
            a[i] = i + 1;
        }
        for (int i = 0; i < n; i++) {
            a[i] += 5;
            a[i] -= 1;
            a[i] *= 3;
            a[i] /= 2;
            a[i] %= 100;
            a[i] |= 0x10;
            a[i] &= 0x7f;
            a[i] ^= 0x21;
            a[i] <<= 1;
            a[i] >>= 1;
            a[i] >>>= 1;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            acc = acc * 131 + a[i];
        }
        return acc;
    }

    // ---- long[] element compound assignment (category-2 element, statement form: DUP2_X1) ----
    public static long longArrayOps(int n) {
        long[] a = new long[n];
        for (int i = 0; i < n; i++) {
            a[i] = (long) i * 1000 + 1;
        }
        for (int i = 0; i < n; i++) {
            a[i] += 7L;
            a[i] *= 3L;
            a[i] ^= 0x5a5a5a5aL;
            a[i] <<= 2;
            a[i] >>>= 1;
            a[i] -= 11L;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            acc ^= a[i] + ((long) i << 8);
        }
        return acc;
    }

    // ---- double[]/float[] element compound assignment, statement form ----
    public static long fpArrayOps(int n) {
        double[] d = new double[n];
        float[] f = new float[n];
        for (int i = 0; i < n; i++) {
            d[i] = i + 0.5;
            f[i] = i + 0.25f;
        }
        for (int i = 0; i < n; i++) {
            d[i] += 1.5;
            d[i] *= 2.0;
            d[i] -= 0.25;
            f[i] += 1.5f;
            f[i] *= 2.0f;
            f[i] -= 0.25f;
        }
        double dacc = 0;
        float facc = 0;
        for (int i = 0; i < n; i++) {
            dacc += d[i];
            facc += f[i];
        }
        return Double.doubleToLongBits(dacc) ^ ((long) Float.floatToIntBits(facc) << 1);
    }

    // ---- byte[]/char[] element compound assignment (narrowing: I2B / I2C) ----
    public static long narrowArrayOps(int n) {
        byte[] b = new byte[n];
        char[] c = new char[n];
        for (int i = 0; i < n; i++) {
            b[i] = (byte) (i * 3);
            c[i] = (char) ('A' + i);
        }
        for (int i = 0; i < n; i++) {
            b[i] += 17;
            b[i] ^= 0x55;
            c[i] += 1;
            c[i] |= 0x20;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            acc = acc * 37 + (b[i] & 0xff);
            acc = acc * 37 + c[i];
        }
        return acc;
    }

    // ---- pre/post increment & decrement on array elements ----
    public static long incDecArray(int n) {
        int[] a = new int[n];
        for (int i = 0; i < n; i++) {
            a[i] = i;
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            acc += a[i]++;     // post-inc: use old, then bump
            acc += ++a[i];     // pre-inc: bump, then use
            acc -= a[i]--;     // post-dec
            acc -= --a[i];     // pre-dec
        }
        for (int i = 0; i < n; i++) {
            acc = acc * 17 + a[i];
        }
        return acc;
    }

    // ---- 2-D array element compound assignment ----
    public static long matrixOps(int n) {
        int[][] m = new int[n][n];
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                m[i][j] = i * n + j;
            }
        }
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                m[i][j] += (i + 1) * (j + 1);
                m[i][j] ^= (i << 2) | j;
            }
        }
        long acc = 0;
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                acc = acc * 131 + m[i][j];
            }
        }
        return acc;
    }

    // ---- static field compound assignment ----
    public static int staticFieldOps(int rounds) {
        STATIC_ACC = 7;
        for (int i = 0; i < rounds; i++) {
            STATIC_ACC += i;
            STATIC_ACC *= 2;
            STATIC_ACC ^= 0x33;
            STATIC_ACC %= 100000;
        }
        return STATIC_ACC;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(Long.toHexString(intArrayOps(13))).append(',');
        sb.append(Long.toHexString(longArrayOps(11))).append(',');
        sb.append(Long.toHexString(fpArrayOps(9))).append(',');
        sb.append(Long.toHexString(narrowArrayOps(15))).append(',');
        sb.append(Long.toHexString(incDecArray(12))).append(',');
        sb.append(Long.toHexString(matrixOps(6))).append(',');
        sb.append(staticFieldOps(20));
        System.out.println(sb);
    }
}
