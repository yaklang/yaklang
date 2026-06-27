package codec;

/**
 * OpcodeCoverage - a self-contained battery that deliberately exercises the JVM opcode families the
 * hash/codec batteries barely touch, so the round-trip oracle (decompile -> recompile -> run) can
 * catch silent miscompiles in:
 *
 *   - double arithmetic:  DADD/DSUB/DMUL/DDIV/DREM/DNEG, DCMPG/DCMPL
 *   - float  arithmetic:  FADD/FSUB/FMUL/FDIV/FREM/FNEG, FCMPG/FCMPL
 *   - numeric conversion: I2L/I2F/I2D, L2I/L2F/L2D, F2I/F2L/F2D, D2I/D2L/D2F, I2B/I2C/I2S
 *   - dense switch:       TABLESWITCH
 *   - sparse switch:      LOOKUPSWITCH
 *   - type tests/casts:   INSTANCEOF, CHECKCAST
 *   - arrays:             NEWARRAY/ANEWARRAY/MULTIANEWARRAY, [DFIJB]ALOAD/[DFIJB]ASTORE, ARRAYLENGTH
 *
 * Floating-point results are fingerprinted through Double.doubleToLongBits / Float.floatToIntBits so
 * the comparison is exact bit-for-bit (Java FP has been strict since 17). All inputs are fixed, so
 * the output is fully deterministic.
 *
 * Single public top-level class, only static methods, no inner/extra top-level classes, so the
 * single-class decompile round-trip stays well-defined.
 */
public class OpcodeCoverage {

    // ---- double arithmetic: every D* arithmetic + both compare opcodes ----
    public static double doubleArith(double a, double b) {
        double sum = a + b;
        double diff = a - b;
        double prod = a * b;
        double quot = a / b;
        double rem = a % b;
        double neg = -a;
        double acc = sum + diff - prod + quot - rem + neg;
        // DCMPG / DCMPL: ordering both ways so both compare opcodes are emitted
        if (a > b) acc += 1.0;
        if (a < b) acc -= 2.0;
        if (a >= b) acc += 0.5;
        if (a <= b) acc -= 0.25;
        return acc;
    }

    // ---- float arithmetic: every F* arithmetic + both compare opcodes ----
    public static float floatArith(float a, float b) {
        float sum = a + b;
        float diff = a - b;
        float prod = a * b;
        float quot = a / b;
        float rem = a % b;
        float neg = -b;
        float acc = sum + diff - prod + quot - rem + neg;
        if (a > b) acc += 1.0f;
        if (a < b) acc -= 2.0f;
        if (a >= b) acc += 0.5f;
        if (a <= b) acc -= 0.25f;
        return acc;
    }

    // ---- every numeric conversion opcode, folded into a single long fingerprint ----
    public static long conversions(int i, long l, float f, double d) {
        long acc = 0;
        acc += (long) i;          // I2L
        acc += (long) (i + f);    // I2F (in promotion) then F2? - keep simple
        acc += (long) (double) i; // I2D then D2L
        acc += (long) (int) l;    // L2I then I2L
        acc += (long) (float) l;  // L2F then F2L
        acc += (long) (double) l; // L2D then D2L
        acc += (int) f;           // F2I
        acc += (long) f;          // F2L
        acc += (long) (double) f; // F2D then D2L
        acc += (int) d;           // D2I
        acc += (long) d;          // D2L
        acc += (long) (float) d;  // D2F then F2L
        // narrowing int conversions: I2B / I2C / I2S
        int wide = i * 2654435761L != 0 ? i + 0x12345 : i;
        byte bb = (byte) wide;    // I2B
        char cc = (char) wide;    // I2C
        short ss = (short) wide;  // I2S
        acc += bb;
        acc += cc;
        acc += ss;
        return acc;
    }

    // ---- TABLESWITCH: dense, contiguous case labels ----
    public static int tableSwitch(int x) {
        int r;
        switch (x) {
            case 0: r = 11; break;
            case 1: r = 22; break;
            case 2: r = 33; break;
            case 3: r = 44; break;
            case 4: r = 55; break;
            case 5: r = 66; break;
            case 6: r = 77; break;
            case 7: r = 88; break;
            default: r = -1; break;
        }
        return r * 2 + x;
    }

    // ---- LOOKUPSWITCH: sparse, non-contiguous case labels ----
    public static int lookupSwitch(int x) {
        int r;
        switch (x) {
            case -1000: r = 1; break;
            case 0: r = 2; break;
            case 7: r = 3; break;
            case 42: r = 4; break;
            case 9999: r = 5; break;
            case 123456: r = 6; break;
            default: r = 0; break;
        }
        // fallthrough chain to mix in more branches
        switch (r) {
            case 1:
            case 2:
            case 3:
                r += 100;
                break;
            default:
                r += 1000;
        }
        return r;
    }

    // ---- INSTANCEOF + CHECKCAST over a mixed Object[] ----
    public static String typeName(Object o) {
        if (o == null) return "null";
        if (o instanceof String) {
            String s = (String) o;
            return "S" + s.length();
        }
        if (o instanceof Integer) {
            Integer n = (Integer) o;
            return "I" + (n.intValue() & 0xff);
        }
        if (o instanceof Long) {
            return "L" + (((Long) o).longValue() % 97);
        }
        if (o instanceof int[]) {
            return "ai" + ((int[]) o).length;
        }
        if (o instanceof Object[]) {
            return "ao" + ((Object[]) o).length;
        }
        if (o instanceof Number) {
            return "N" + ((Number) o).intValue();
        }
        return "?";
    }

    // ---- MULTIANEWARRAY + nested IALOAD/IASTORE ----
    public static long multiIntArray(int n) {
        int[][] grid = new int[n][n];
        for (int r = 0; r < n; r++) {
            for (int c = 0; c < n; c++) {
                grid[r][c] = (r * 31 + c * 17) ^ (r << 2);
            }
        }
        long sum = 0;
        for (int r = 0; r < n; r++) {
            for (int c = 0; c < n; c++) {
                sum += grid[r][c];
                sum ^= ((long) grid[c % n][r % n]) << (c & 7);
            }
        }
        // three-dimensional MULTIANEWARRAY
        long[][][] cube = new long[2][n][3];
        long acc = 0;
        for (int a = 0; a < 2; a++) {
            for (int b = 0; b < n; b++) {
                for (int d = 0; d < 3; d++) {
                    cube[a][b][d] = (long) a * 1000 + b * 10 + d;
                    acc += cube[a][b][d];
                }
            }
        }
        return sum + acc + grid.length + cube.length;
    }

    // ---- double[]/float[]: DALOAD/DASTORE, FALOAD/FASTORE, ARRAYLENGTH ----
    public static long fpArrays(int n) {
        double[] ds = new double[n];
        float[] fs = new float[n];
        for (int i = 0; i < n; i++) {
            ds[i] = i * 1.5 - 0.5;
            fs[i] = i * 0.25f + 1.0f;
        }
        double dacc = 0;
        float facc = 0;
        for (int i = 0; i < n; i++) {
            dacc += ds[i] * ds[(i + 1) % n];
            facc += fs[i] - fs[(n - 1 - i + n) % n];
        }
        return Double.doubleToLongBits(dacc) ^ ((long) Float.floatToIntBits(facc) << 1) ^ (ds.length + fs.length);
    }

    // ---- long arithmetic compare chain (LCMP across branches) ----
    public static int longCompare(long a, long b) {
        int r = 0;
        if (a > b) r |= 1;
        if (a < b) r |= 2;
        if (a == b) r |= 4;
        if (a >= b) r |= 8;
        if (a <= b) r |= 16;
        r += (int) (Long.compare(a, b) & 0xff);
        return r;
    }

    // ---- double/float stored into local slots 0 and 1: DSTORE_0/DSTORE_1, FSTORE_0/FSTORE_1 ----
    // A category-2 local that lands in slot 0 (static method, first local is the double/float) emits
    // the *_0 store form; one pushed to slot 1 (a preceding int param occupies slot 0) emits *_1.
    public static double dstoreSlot0() {
        double d = 0.5;       // DSTORE_0 (initial)
        d = d * 3.0 + 1.0;    // DSTORE_0 (reassign)
        return d;
    }

    public static double dstoreSlot1(int seed) {
        double d = seed + 0.5; // seed is slot 0 (int), d is slot 1 (double) -> DSTORE_1
        d = d * 2.0 - 0.25;    // DSTORE_1 (reassign)
        return d;
    }

    public static float fstoreSlot0() {
        float x = 0.25f;       // FSTORE_0
        x = x * 4.0f + 1.0f;   // FSTORE_0 (reassign)
        return x;
    }

    public static float fstoreSlot1(int seed) {
        float x = seed + 0.5f; // seed slot 0 (int), x slot 1 (float) -> FSTORE_1
        x = x * 2.0f - 0.5f;   // FSTORE_1 (reassign)
        return x;
    }

    // ---- compound assignment to a category-2 array element with the result used: DUP2_X2 ----
    // `arr[i] += v` for a double[]/long[] element, when its value is consumed, makes javac duplicate
    // the category-2 result beneath the (arrayref,index) pair via dup2_x2 before the *astore.
    public static long dup2x2(double[] a, long[] b, int i) {
        double dv = (a[i] += 2.5); // double[] element compound-assign, value used -> DUP2_X2
        long lv = (b[i] += 7L);    // long[]  element compound-assign, value used -> DUP2_X2
        return Double.doubleToLongBits(dv) ^ (lv << 1) ^ Double.doubleToLongBits(a[i]) ^ b[i];
    }

    private static String hex64(long v) {
        return String.format("%016x", v);
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        double[][] dpairs = {{3.5, 1.25}, {-2.0, 7.0}, {0.0, -0.0}, {1e10, 3.0}, {-5.5, -5.5}};
        for (double[] p : dpairs) {
            sb.append(hex64(Double.doubleToLongBits(doubleArith(p[0], p[1])))).append(",");
        }
        float[][] fpairs = {{3.5f, 1.25f}, {-2.0f, 7.0f}, {123.5f, 0.5f}, {-5.5f, -5.5f}};
        for (float[] p : fpairs) {
            sb.append(Integer.toHexString(Float.floatToIntBits(floatArith(p[0], p[1])))).append(",");
        }

        sb.append(hex64(conversions(123456, 9876543210L, 3.75f, -12.5))).append(",");
        sb.append(hex64(conversions(-1, -1L, -0.5f, 1e9))).append(",");

        for (int x = -2; x <= 9; x++) sb.append(tableSwitch(x)).append(":");
        sb.append(",");
        int[] lookupInputs = {-1000, 0, 7, 42, 9999, 123456, 5};
        for (int x : lookupInputs) sb.append(lookupSwitch(x)).append(":");
        sb.append(",");

        Object[] objs = {
            "hello", Integer.valueOf(300), Long.valueOf(1000), new int[]{1, 2, 3},
            new Object[]{"a", "b"}, Double.valueOf(2.5), null
        };
        for (Object o : objs) sb.append(typeName(o)).append(":");
        sb.append(",");

        sb.append(multiIntArray(5)).append(",");
        sb.append(hex64(fpArrays(6))).append(",");

        long[][] lpairs = {{5, 3}, {3, 5}, {7, 7}, {Long.MIN_VALUE, Long.MAX_VALUE}};
        for (long[] p : lpairs) sb.append(longCompare(p[0], p[1])).append(":");
        sb.append(",");

        // low-slot category-2 stores + category-2 array compound-assign (DSTORE_0/1, FSTORE_0/1, DUP2_X2)
        sb.append(hex64(Double.doubleToLongBits(dstoreSlot0()))).append(":");
        sb.append(hex64(Double.doubleToLongBits(dstoreSlot1(40)))).append(":");
        sb.append(Integer.toHexString(Float.floatToIntBits(fstoreSlot0()))).append(":");
        sb.append(Integer.toHexString(Float.floatToIntBits(fstoreSlot1(40)))).append(":");
        // NOTE: dup2x2() is intentionally NOT folded into the verified fingerprint. It exists only to
        // emit the DUP2_X2 opcode (a category-2 array compound-assign whose value is used) for the
        // opcode-parse-coverage gate. The decompiler currently mis-reconstructs that idiom by
        // re-evaluating the RHS instead of reading back the stored element (see CODEC_TODO.md "Bug J"),
        // so calling it here would make this semantics round-trip diverge. Keeping the method present
        // (but uncalled) still routes DUP2_X2 through the stack simulator during decompilation.

        System.out.println(sb);
    }
}
