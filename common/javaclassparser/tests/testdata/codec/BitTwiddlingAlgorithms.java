package codec;

/**
 * BitTwiddlingAlgorithms - self-hosted Hacker's-Delight style integer/bit routines, each implemented
 * from scratch and (where a JDK equivalent exists) folded into the fingerprint right next to the JDK
 * result so the golden output visibly cross-checks the hand-rolled algorithm against the platform.
 *
 * Purpose: dense, branch-light arithmetic/bitwise coverage (shifts, and/or/xor, neg, mul, compares,
 * int<->long conversions) on top of the round-trip oracle - decompile -> recompile -> run must keep
 * every result byte-for-byte identical.
 *
 * Written with simple counted loops and ascending if/else (no `if(!cond) throw;` guards before large
 * bodies, no descending fall-through switch, no trailing `while(cond){i++}` loops) so it sidesteps the
 * control-flow reconstruction bugs catalogued in CODEC_TODO.md. Single public top-level class, static
 * methods only, fully deterministic.
 */
public class BitTwiddlingAlgorithms {

    // ---- population count (SWAR parallel bit count), 32-bit ----
    public static int popcount32(int x) {
        x = x - ((x >>> 1) & 0x55555555);
        x = (x & 0x33333333) + ((x >>> 2) & 0x33333333);
        x = (x + (x >>> 4)) & 0x0f0f0f0f;
        x = x + (x >>> 8);
        x = x + (x >>> 16);
        return x & 0x3f;
    }

    // ---- population count, 64-bit ----
    public static int popcount64(long x) {
        x = x - ((x >>> 1) & 0x5555555555555555L);
        x = (x & 0x3333333333333333L) + ((x >>> 2) & 0x3333333333333333L);
        x = (x + (x >>> 4)) & 0x0f0f0f0f0f0f0f0fL;
        x = x + (x >>> 8);
        x = x + (x >>> 16);
        x = x + (x >>> 32);
        return (int) (x & 0x7f);
    }

    // ---- bit reversal, 32-bit (parallel swaps) ----
    public static int reverse32(int x) {
        x = ((x & 0x55555555) << 1) | ((x >>> 1) & 0x55555555);
        x = ((x & 0x33333333) << 2) | ((x >>> 2) & 0x33333333);
        x = ((x & 0x0f0f0f0f) << 4) | ((x >>> 4) & 0x0f0f0f0f);
        x = ((x & 0x00ff00ff) << 8) | ((x >>> 8) & 0x00ff00ff);
        x = (x << 16) | (x >>> 16);
        return x;
    }

    // ---- byte reversal, 32-bit ----
    public static int reverseBytes32(int x) {
        return ((x >>> 24) & 0xff) | ((x >>> 8) & 0xff00) | ((x << 8) & 0xff0000) | (x << 24);
    }

    // ---- byte reversal, 64-bit ----
    public static long reverseBytes64(long x) {
        x = ((x & 0x00ff00ff00ff00ffL) << 8) | ((x >>> 8) & 0x00ff00ff00ff00ffL);
        x = ((x & 0x0000ffff0000ffffL) << 16) | ((x >>> 16) & 0x0000ffff0000ffffL);
        x = (x << 32) | (x >>> 32);
        return x;
    }

    // ---- number of trailing zeros, 32-bit (counted loop; 32 for x==0) ----
    public static int ntz32(int x) {
        if (x == 0) {
            return 32;
        }
        int n = 0;
        while ((x & 1) == 0) {
            x = x >>> 1;
            n++;
        }
        return n;
    }

    // ---- number of leading zeros, 32-bit (binary search; 32 for x==0) ----
    public static int nlz32(int x) {
        if (x == 0) {
            return 32;
        }
        int n = 0;
        if ((x & 0xffff0000) == 0) { n += 16; x <<= 16; }
        if ((x & 0xff000000) == 0) { n += 8;  x <<= 8; }
        if ((x & 0xf0000000) == 0) { n += 4;  x <<= 4; }
        if ((x & 0xc0000000) == 0) { n += 2;  x <<= 2; }
        if ((x & 0x80000000) == 0) { n += 1; }
        return n;
    }

    // ---- parity (1 if odd number of set bits) ----
    public static int parity32(int x) {
        x ^= x >>> 16;
        x ^= x >>> 8;
        x ^= x >>> 4;
        x &= 0xf;
        return (0x6996 >>> x) & 1;
    }

    // ---- next power of two >= x (x in [1, 2^30]) ----
    public static int nextPow2(int x) {
        x--;
        x |= x >>> 1;
        x |= x >>> 2;
        x |= x >>> 4;
        x |= x >>> 8;
        x |= x >>> 16;
        x++;
        return x;
    }

    // ---- floor(log2(x)) for x >= 1, via leading-zero count ----
    public static int log2floor(int x) {
        return 31 - nlz32(x);
    }

    // ---- Euclid gcd ----
    public static int gcd(int a, int b) {
        if (a < 0) a = -a;
        if (b < 0) b = -b;
        while (b != 0) {
            int t = a % b;
            a = b;
            b = t;
        }
        return a;
    }

    // ---- integer square root (floor) of a non-negative long, bit-by-bit ----
    public static long isqrt(long n) {
        if (n < 0) {
            return -1;
        }
        long result = 0;
        // highest power of four <= n
        long bit = 1L << 62;
        while (bit > n) {
            bit >>>= 2;
        }
        while (bit != 0) {
            if (n >= result + bit) {
                n -= result + bit;
                result = (result >>> 1) + bit;
            } else {
                result >>>= 1;
            }
            bit >>>= 2;
        }
        return result;
    }

    // ---- rotate left/right, 32-bit ----
    public static int rotl32(int x, int r) {
        r &= 31;
        return (x << r) | (x >>> (32 - r));
    }

    public static int rotr32(int x, int r) {
        r &= 31;
        return (x >>> r) | (x << (32 - r));
    }

    private static String h32(int v) {
        return Integer.toHexString(v);
    }

    private static String h64(long v) {
        return String.format("%016x", v);
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        int[] ints = {0, 1, 2, 7, 255, 256, 0x12345678, -1, -2, 0x80000000, 0x7fffffff, 1023, 1024};
        for (int x : ints) {
            // each hand-rolled result is paired with the JDK builtin so the golden line cross-checks them
            sb.append(popcount32(x)).append('/').append(Integer.bitCount(x)).append(';');
            sb.append(h32(reverse32(x))).append('/').append(h32(Integer.reverse(x))).append(';');
            sb.append(h32(reverseBytes32(x))).append('/').append(h32(Integer.reverseBytes(x))).append(';');
            sb.append(ntz32(x)).append('/').append(Integer.numberOfTrailingZeros(x)).append(';');
            sb.append(nlz32(x)).append('/').append(Integer.numberOfLeadingZeros(x)).append(';');
            sb.append(parity32(x)).append(';');
            sb.append(h32(rotl32(x, 5))).append('/').append(h32(Integer.rotateLeft(x, 5))).append(';');
            sb.append(h32(rotr32(x, 5))).append('/').append(h32(Integer.rotateRight(x, 5))).append('|');
        }
        sb.append(',');

        int[] positives = {1, 2, 3, 5, 17, 31, 32, 33, 1000, 65535, 65536, 1 << 30};
        for (int x : positives) {
            sb.append(nextPow2(x)).append('/').append(Integer.highestOneBit(nextPow2(x))).append(';');
            sb.append(log2floor(x)).append(';');
        }
        sb.append(',');

        long[] longs = {0L, 1L, 0x0123456789abcdefL, -1L, 0x8000000000000000L, 0x7fffffffffffffffL};
        for (long x : longs) {
            sb.append(popcount64(x)).append('/').append(Long.bitCount(x)).append(';');
            sb.append(h64(reverseBytes64(x))).append('/').append(h64(Long.reverseBytes(x))).append('|');
        }
        sb.append(',');

        long[] sqInputs = {0L, 1L, 2L, 3L, 4L, 15L, 16L, 17L, 1000000L, 1000000007L, 999999999999L, Long.MAX_VALUE};
        for (long x : sqInputs) {
            sb.append(isqrt(x)).append(';');
        }
        sb.append(',');

        int[][] gcdPairs = {{12, 18}, {1071, 462}, {17, 5}, {0, 9}, {9, 0}, {-12, 18}, {1000000, 8}};
        for (int[] p : gcdPairs) {
            sb.append(gcd(p[0], p[1])).append(';');
        }

        System.out.println(sb);
    }
}
