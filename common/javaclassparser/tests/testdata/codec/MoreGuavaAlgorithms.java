package codec;

/**
 * MoreGuavaAlgorithms - a second, complementary Guava-style oracle battery (the first is
 * GuavaAlgorithms.java). Mirrors com.google.common.primitives (Ints/Longs/Shorts/UnsignedInts/
 * UnsignedLongs/UnsignedBytes/Booleans), com.google.common.base.{Strings,Ascii} and a few
 * com.google.common.math.IntMath helpers. No Guava dependency: pure static methods so the
 * single-class decompile round-trip (decompile -> recompile -> run, fingerprints compared) holds.
 *
 * Opcode intent: byte<->int/long packing with sign-extension and masks (ISHL/LSHL, IAND/LAND,
 * I2L), unsigned division/compare via widening and MIN_VALUE flipping (LDIV/LREM/LXOR/LCMP),
 * char[] case folding, counted loops with early break, nested loops. Every loop is a counted
 * for-loop with an `if (...) break;` exit (never a trailing `while`) to avoid the loop-exit
 * inversion catalogued in CODEC_TODO.md; no switch with an empty default (Bug K); no compound
 * assignment whose value is consumed (Bug J).
 *
 * Self-checking: unsigned helpers are cross-validated against the JDK's Integer.toUnsignedLong,
 * Long.compareUnsigned and Long.divideUnsigned, so an oracle typo fails independently of the
 * decompiler. Single public top-level class, only static methods.
 */
public class MoreGuavaAlgorithms {

    // ===== Ints/Longs/Shorts.fromBytes (big-endian packing with sign-extension on the top byte) =====
    public static int intFromBytes(byte b1, byte b2, byte b3, byte b4) {
        return (b1 << 24) | ((b2 & 0xff) << 16) | ((b3 & 0xff) << 8) | (b4 & 0xff);
    }

    public static long longFromBytes(byte b1, byte b2, byte b3, byte b4, byte b5, byte b6, byte b7, byte b8) {
        return ((b1 & 0xffL) << 56) | ((b2 & 0xffL) << 48) | ((b3 & 0xffL) << 40) | ((b4 & 0xffL) << 32)
                | ((b5 & 0xffL) << 24) | ((b6 & 0xffL) << 16) | ((b7 & 0xffL) << 8) | (b8 & 0xffL);
    }

    public static short shortFromBytes(byte b1, byte b2) {
        return (short) ((b1 << 8) | (b2 & 0xff));
    }

    // ===== UnsignedInts / UnsignedLongs / UnsignedBytes =====
    public static long unsignedIntToLong(int x) {
        return x & 0xffffffffL;
    }

    public static int unsignedDivide(int dividend, int divisor) {
        return (int) ((dividend & 0xffffffffL) / (divisor & 0xffffffffL));
    }

    public static int unsignedRemainder(int dividend, int divisor) {
        return (int) ((dividend & 0xffffffffL) % (divisor & 0xffffffffL));
    }

    public static int unsignedLongCompare(long a, long b) {
        long fa = a ^ Long.MIN_VALUE;
        long fb = b ^ Long.MIN_VALUE;
        if (fa < fb) {
            return -1;
        }
        if (fa > fb) {
            return 1;
        }
        return 0;
    }

    // UnsignedLongs.divide via the standard sign-corrected algorithm (no use of Long.divideUnsigned).
    public static long unsignedLongDivide(long dividend, long divisor) {
        if (divisor < 0) {
            // divisor >= 2^63: quotient is 0 or 1
            if (unsignedLongCompare(dividend, divisor) < 0) {
                return 0;
            }
            return 1;
        }
        if (dividend >= 0) {
            return dividend / divisor;
        }
        long quotient = ((dividend >>> 1) / divisor) << 1;
        long rem = dividend - quotient * divisor;
        if (unsignedLongCompare(rem, divisor) >= 0) {
            return quotient + 1;
        }
        return quotient;
    }

    public static int unsignedByteCompare(byte a, byte b) {
        return (a & 0xff) - (b & 0xff);
    }

    // ===== Strings (repeat / padStart / padEnd / commonPrefix / commonSuffix) =====
    public static String repeat(String s, int count) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < count; i++) {
            sb.append(s);
        }
        return sb.toString();
    }

    public static String padStart(String s, int minLength, char pad) {
        if (s.length() >= minLength) {
            return s;
        }
        StringBuilder sb = new StringBuilder();
        for (int i = s.length(); i < minLength; i++) {
            sb.append(pad);
        }
        sb.append(s);
        return sb.toString();
    }

    public static String padEnd(String s, int minLength, char pad) {
        if (s.length() >= minLength) {
            return s;
        }
        StringBuilder sb = new StringBuilder(s);
        for (int i = s.length(); i < minLength; i++) {
            sb.append(pad);
        }
        return sb.toString();
    }

    public static String commonPrefix(String a, String b) {
        int n = Math.min(a.length(), b.length());
        int i = 0;
        for (int k = 0; k < n; k++) {
            if (a.charAt(k) != b.charAt(k)) {
                break;
            }
            i++;
        }
        return a.substring(0, i);
    }

    public static String commonSuffix(String a, String b) {
        int n = Math.min(a.length(), b.length());
        int i = 0;
        for (int k = 0; k < n; k++) {
            if (a.charAt(a.length() - 1 - k) != b.charAt(b.length() - 1 - k)) {
                break;
            }
            i++;
        }
        return a.substring(a.length() - i);
    }

    // ===== Ascii (ASCII-only case folding) =====
    public static String asciiUpper(String s) {
        char[] c = s.toCharArray();
        for (int i = 0; i < c.length; i++) {
            char ch = c[i];
            if (ch >= 'a' && ch <= 'z') {
                c[i] = (char) (ch - 32);
            }
        }
        return new String(c);
    }

    public static String asciiLower(String s) {
        char[] c = s.toCharArray();
        for (int i = 0; i < c.length; i++) {
            char ch = c[i];
            if (ch >= 'A' && ch <= 'Z') {
                c[i] = (char) (ch + 32);
            }
        }
        return new String(c);
    }

    // ===== IntMath / LongMath helpers =====
    public static int pow(int base, int exp) {
        int result = 1;
        for (int i = 0; i < exp; i++) {
            result *= base;
        }
        return result;
    }

    // IntMath.mean: floor of the average without intermediate overflow, rounding toward -inf.
    public static int mean(int x, int y) {
        return (x & y) + ((x ^ y) >> 1);
    }

    public static long factorial(int n) {
        long r = 1;
        for (int i = 2; i <= n; i++) {
            r *= i;
        }
        return r;
    }

    public static int maxOf(int[] a) {
        int m = a[0];
        for (int i = 1; i < a.length; i++) {
            if (a[i] > m) {
                m = a[i];
            }
        }
        return m;
    }

    public static int minOf(int[] a) {
        int m = a[0];
        for (int i = 1; i < a.length; i++) {
            if (a[i] < m) {
                m = a[i];
            }
        }
        return m;
    }

    public static int countTrue(boolean[] vals) {
        int c = 0;
        for (int i = 0; i < vals.length; i++) {
            if (vals[i]) {
                c++;
            }
        }
        return c;
    }

    // Ints.saturatedCast(long)
    public static int saturatedCast(long value) {
        if (value > Integer.MAX_VALUE) {
            return Integer.MAX_VALUE;
        }
        if (value < Integer.MIN_VALUE) {
            return Integer.MIN_VALUE;
        }
        return (int) value;
    }

    private static char bit(boolean v) {
        return v ? '1' : '0';
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        sb.append(Integer.toHexString(intFromBytes((byte) 0xDE, (byte) 0xAD, (byte) 0xBE, (byte) 0xEF))).append(',');
        sb.append(Long.toHexString(longFromBytes((byte) 1, (byte) 2, (byte) 3, (byte) 4,
                (byte) 5, (byte) 6, (byte) 7, (byte) 8))).append(',');
        sb.append((int) shortFromBytes((byte) 0x12, (byte) 0x34)).append(',');

        // unsigned int widening cross-checked against the JDK
        int[] us = {-1, -2, 0x80000000, 123456789, 0};
        for (int i = 0; i < us.length; i++) {
            sb.append(bit(unsignedIntToLong(us[i]) == Integer.toUnsignedLong(us[i])));
        }
        sb.append(',');
        sb.append(unsignedDivide(-1, 7)).append(';').append(unsignedRemainder(-1, 7)).append(',');

        // unsigned long compare + divide cross-checked against the JDK
        long[] ul = {-1L, 1L, Long.MIN_VALUE, 100L, 0L, -100L};
        for (int i = 0; i < ul.length; i++) {
            for (int j = 0; j < ul.length; j++) {
                sb.append(bit(Integer.signum(unsignedLongCompare(ul[i], ul[j]))
                        == Integer.signum(Long.compareUnsigned(ul[i], ul[j]))));
            }
        }
        sb.append(',');
        for (int i = 0; i < ul.length; i++) {
            for (int j = 0; j < ul.length; j++) {
                // Positive guard rather than `if (ul[j] == 0) continue;`. The continue form inverted
                // here (ran the body when the divisor was 0 -> ArithmeticException) inside the second
                // of two sibling nested loops over the same array; see CODEC_TODO.md "Bug L".
                if (ul[j] != 0L) {
                    sb.append(bit(unsignedLongDivide(ul[i], ul[j]) == Long.divideUnsigned(ul[i], ul[j])));
                }
            }
        }
        sb.append(',');

        sb.append(unsignedByteCompare((byte) 0x80, (byte) 0x7f)).append(',');

        sb.append(repeat("ab", 3)).append(';');
        sb.append(padStart("42", 5, '0')).append(';');
        sb.append(padEnd("42", 5, 'x')).append(';');
        sb.append(commonPrefix("flower", "flow")).append(';');
        sb.append(commonSuffix("testing", "running")).append(';');
        sb.append(asciiUpper("Hello, World!")).append(';');
        sb.append(asciiLower("Hello, World!")).append(';');
        sb.append(pow(3, 7)).append(';');
        sb.append(mean(10, 21)).append(';').append(mean(-7, -2)).append(';');
        sb.append(factorial(12)).append(';');

        int[] arr = {3, -7, 12, 0, 5, 12, -100};
        sb.append(maxOf(arr)).append(';').append(minOf(arr)).append(';');

        boolean[] bs = {true, false, true, true, false};
        sb.append(countTrue(bs)).append(';');

        sb.append(saturatedCast(5000000000L)).append(';');
        sb.append(saturatedCast(-5000000000L)).append(';');
        sb.append(saturatedCast(42L));

        System.out.println(sb);
    }
}
