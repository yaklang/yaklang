package codec;

/**
 * PostIncrementAlgorithms - a focused battery for the post-increment / post-decrement-in-expression
 * idiom (CODEC_TODO.md "arr[i++]=v"). javac compiles `a[i++] = v`, `int j = i++`, `return i++` and
 * `a[i++] = b[j--]` as `iload X; iinc X; ...` so the OLD value of slot X is still live on the
 * operand stack when the iinc runs. The decompiler used to emit the standalone `i++` BEFORE the
 * consuming statement, turning `a[i++] = v` into `i++; a[i] = v` (wrong index, off by one ->
 * out-of-bounds / wrong result). The fix folds the iinc into a post-op expression that rides inside
 * the consuming expression, so this battery exercises every shape that depended on it.
 *
 * Opcode intent: iinc (post inc/dec in expression and as a loop step), iastore/bastore/castore with
 * a post-incremented index, array load with a post-incremented index, two-pointer swaps, counted
 * for-loops, ternary, and StringBuilder. Pre-increment (`++i`) is included as a contrast: it must
 * stay correct (it compiles to `iinc X; iload X`, with no live load at iinc time).
 *
 * Every routine is a pure static method so the single-class decompile round-trip
 * (decompile -> recompile -> run, fingerprints compared) is well defined.
 */
public class PostIncrementAlgorithms {

    // ---- run-length encode bytes into an output buffer using out[oi++] ----
    public static byte[] runLengthEncode(byte[] in) {
        byte[] out = new byte[in.length * 2];
        int oi = 0;
        int i = 0;
        while (i < in.length) {
            byte cur = in[i];
            int run = 1;
            while (i + run < in.length && in[i + run] == cur && run < 255) {
                run++;
            }
            out[oi++] = (byte) run;
            out[oi++] = cur;
            i += run;
        }
        byte[] packed = new byte[oi];
        for (int k = 0; k < oi; k++) {
            packed[k] = out[k];
        }
        return packed;
    }

    // ---- reverse an int array in place with a two-pointer i++ / j-- swap ----
    public static int[] reverseInPlace(int[] a) {
        int i = 0;
        int j = a.length - 1;
        while (i < j) {
            int t = a[i];
            a[i++] = a[j];
            a[j--] = t;
        }
        return a;
    }

    // ---- lowercase hex encode using out[oi++] for both nibbles ----
    public static String hexEncode(byte[] in) {
        char[] digits = "0123456789abcdef".toCharArray();
        char[] out = new char[in.length * 2];
        int oi = 0;
        for (int i = 0; i < in.length; i++) {
            int v = in[i] & 0xff;
            out[oi++] = digits[v >>> 4];
            out[oi++] = digits[v & 0x0f];
        }
        return new String(out);
    }

    // ---- copy every other element: dst[j++] = src[i] where i steps by 2 ----
    public static int[] copyEveryOther(int[] src) {
        int[] dst = new int[(src.length + 1) / 2];
        int j = 0;
        for (int i = 0; i < src.length; i += 2) {
            dst[j++] = src[i];
        }
        return dst;
    }

    // ---- `int j = i++` style: consume the pre-increment value, then keep using i ----
    public static int splitAndSum(int i) {
        int a = i++;
        int b = i++;
        int c = i;
        return a * 10000 + b * 100 + c;
    }

    // ---- `return i++` directly returns the OLD value ----
    public static int returnOld(int i) {
        return i++;
    }

    // ---- `return ++i` (pre-increment contrast) returns the NEW value ----
    public static int returnNew(int i) {
        return ++i;
    }

    // ---- simple ring buffer: head/tail post-increment with modulo wraparound ----
    public static int ringChurn(int cap, int rounds) {
        int[] buf = new int[cap];
        int head = 0;
        int tail = 0;
        int sum = 0;
        for (int r = 0; r < rounds; r++) {
            buf[tail] = r;
            tail = (tail + 1) % cap;
            if (r >= cap - 1) {
                sum += buf[head];
                head = (head + 1) % cap;
            }
        }
        return sum;
    }

    // ---- decode a hex string back to bytes, reading two chars via in[i++] ----
    public static int hexDecodeSum(String hex) {
        char[] in = hex.toCharArray();
        int i = 0;
        int sum = 0;
        while (i + 1 < in.length) {
            int hi = Character.digit(in[i++], 16);
            int lo = Character.digit(in[i++], 16);
            sum += (hi << 4) | lo;
        }
        return sum;
    }

    private static String bytesToString(byte[] b) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < b.length; i++) {
            if (i > 0) {
                sb.append('-');
            }
            sb.append(b[i] & 0xff);
        }
        return sb.toString();
    }

    private static String intsToString(int[] a) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < a.length; i++) {
            if (i > 0) {
                sb.append(',');
            }
            sb.append(a[i]);
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        byte[] rle = runLengthEncode(new byte[]{1, 1, 1, 2, 2, 3});
        sb.append(bytesToString(rle)).append('|');

        sb.append(intsToString(reverseInPlace(new int[]{1, 2, 3, 4, 5}))).append('|');

        sb.append(hexEncode(new byte[]{0x00, (byte) 0xab, 0x0f, (byte) 0xff})).append('|');

        sb.append(intsToString(copyEveryOther(new int[]{10, 11, 12, 13, 14, 15, 16}))).append('|');

        sb.append(splitAndSum(7)).append('|');
        sb.append(returnOld(5)).append('|');
        sb.append(returnNew(5)).append('|');

        sb.append(ringChurn(3, 8)).append('|');

        sb.append(hexDecodeSum("00ab0fff"));

        System.out.println(sb.toString());
    }
}
