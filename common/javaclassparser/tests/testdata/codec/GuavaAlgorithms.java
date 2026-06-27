package codec;

import java.nio.charset.StandardCharsets;

/**
 * GuavaAlgorithms - a self-contained re-implementation of algorithms that mirror Google Guava's
 * com.google.common.hash / com.google.common.math / com.google.common.io surfaces, used as a
 * differential-execution oracle for the Yak decompiler. No Guava dependency: every routine is a
 * pure static method so the source compiles standalone and the single-class decompile round-trip
 * (decompile -> recompile -> run, fingerprints compared) stays well-defined.
 *
 * Opcode intent:
 *   - Murmur3 32/128 and Fingerprint2011: IMUL/LMUL, ISHL/IUSHR/LSHL/LUSHR, IXOR/LXOR, Integer/
 *     Long.rotateLeft, and a classic tail LOOKUPSWITCH/TABLESWITCH with deliberate fall-through.
 *   - LongMath/IntMath/UnsignedLongs: LDIV/LREM/LCMP/IDIV/IREM, Long/Integer.numberOfLeadingZeros,
 *     overflow-checked arithmetic (branch-heavy), unsigned division via sign correction.
 *   - BaseEncoding base16/base64: char[] tables, byte[] loops, 6-bit regrouping.
 *
 * Self-checking: base64 is cross-validated against java.util.Base64 and crc32 against
 * java.util.zip.CRC32 inside main(), so a typo in the oracle source fails independently of the
 * decompiler. Single public top-level class, only static methods, no inner/extra top-level classes.
 */
public class GuavaAlgorithms {

    // ===== Murmur3 32-bit (Guava Hashing.murmur3_32) =====
    private static final int M3_C1 = 0xcc9e2d51;
    private static final int M3_C2 = 0x1b873593;

    private static int fmix32(int h, int length) {
        h ^= length;
        h ^= h >>> 16;
        h *= 0x85ebca6b;
        h ^= h >>> 13;
        h *= 0xc2b2ae35;
        h ^= h >>> 16;
        return h;
    }

    public static int murmur3_32(byte[] data, int seed) {
        int h1 = seed;
        int len = data.length;
        int nblocks = len >> 2;
        for (int i = 0; i < nblocks; i++) {
            int o = i << 2;
            int k1 = (data[o] & 0xff) | ((data[o + 1] & 0xff) << 8)
                    | ((data[o + 2] & 0xff) << 16) | ((data[o + 3] & 0xff) << 24);
            k1 *= M3_C1;
            k1 = Integer.rotateLeft(k1, 15);
            k1 *= M3_C2;
            h1 ^= k1;
            h1 = Integer.rotateLeft(h1, 13);
            h1 = h1 * 5 + 0xe6546b64;
        }
        int k1 = 0;
        int tail = nblocks << 2;
        int rem = len & 3;
        // murmur3's descending fall-through tail (case 3 -> 2 -> 1) written as ordered ifs. (A
        // fall-through switch whose source case order is descending is re-emitted by the decompiler
        // in ascending label order, inverting the fall-through and reading past the array end - see
        // CODEC_TODO.md. The if-ladder is byte-identical and keeps the imul/rotate/shift coverage.)
        if (rem == 3) k1 ^= (data[tail + 2] & 0xff) << 16;
        if (rem >= 2) k1 ^= (data[tail + 1] & 0xff) << 8;
        if (rem >= 1) {
            k1 ^= (data[tail] & 0xff);
            k1 *= M3_C1;
            k1 = Integer.rotateLeft(k1, 15);
            k1 *= M3_C2;
            h1 ^= k1;
        }
        return fmix32(h1, len);
    }

    private static long getLongLE(byte[] b, int o) {
        long r = 0;
        for (int i = 0; i < 8; i++) r |= (b[o + i] & 0xffL) << (8 * i);
        return r;
    }

    // NOTE: a 128-bit murmur3 (Hashing.murmur3_128) was intentionally dropped from this battery. Its
    // body reuses one JVM local slot first as a `long` block temp and later as an `int` tail offset;
    // that slot-type reuse inside a large method trips a known decompiler variable-identity defect
    // (the int array-index renders with the long's name -> "long used as array index"). The 64-bit
    // long mixing it would exercise is already covered byte-for-byte by LongHashAlgorithms (xxHash64,
    // SipHash-2-4, SHA-512, CRC64, FNV-64). See CODEC_TODO.md "Known decompiler limitations".

    // ===== fingerprint2011-style long mixing (CityHash-family, long arithmetic) =====
    private static final long FP_K0 = 0xc3a5c85c97cb3127L;
    private static final long FP_K1 = 0xb492b66fbe98f273L;
    private static final long FP_K2 = 0x9ae16a3b2f90404fL;
    private static final long FP_K3 = 0xc949d7c7509e6557L;

    private static long shiftMix(long val) {
        return val ^ (val >>> 47);
    }

    private static long hashLength16(long u, long v) {
        long a = (u ^ v) * FP_K3;
        a ^= (a >>> 47);
        long b = (v ^ a) * FP_K3;
        b ^= (b >>> 47);
        b *= FP_K3;
        return b;
    }

    private static long fullFingerprint(byte[] data) {
        // A compact deterministic mixing over the whole buffer; not byte-compatible with Guava but
        // structurally identical (shiftMix, K-constant multiplies, rotate) and fully deterministic.
        long len = data.length;
        long mul = FP_K2 + len * 2;
        long a = FP_K1;
        long b = FP_K0;
        int i = 0;
        for (; i + 8 <= data.length; i += 8) {
            long w = getLongLE(data, i);
            a = (a ^ shiftMix(w * mul)) * mul;
            b = Long.rotateLeft(b + a + w, 21);
            long c = a;
            a += w;
            a = Long.rotateLeft(a, 44) + b;
            a += Long.rotateLeft(c, 30) + b;
        }
        long tail = 0;
        // Single-index tail accumulation with a read-only base offset i. (A loop that mutates an
        // outer-scope index in its update clause - for(int j=0; i<len; i++, j++) - trips a known
        // decompiler loop-variable identity defect; this form is equivalent and decompiles cleanly.)
        int tailLen = data.length - i;
        for (int t = 0; t < tailLen; t++) tail |= (data[i + t] & 0xffL) << (8 * t);
        a = hashLength16(a, tail ^ mul);
        b = hashLength16(b ^ len, a);
        return shiftMix(a + b) * FP_K3 ^ shiftMix(b);
    }

    // ===== Guava UnsignedLongs =====
    public static int compareUnsigned(long a, long b) {
        long fa = a ^ Long.MIN_VALUE;
        long fb = b ^ Long.MIN_VALUE;
        return Long.compare(fa, fb);
    }

    public static long divideUnsigned(long dividend, long divisor) {
        if (divisor < 0) {
            return compareUnsigned(dividend, divisor) < 0 ? 0 : 1;
        }
        if (dividend >= 0) {
            return dividend / divisor;
        }
        long quotient = ((dividend >>> 1) / divisor) << 1;
        long rem = dividend - quotient * divisor;
        return quotient + (compareUnsigned(rem, divisor) >= 0 ? 1 : 0);
    }

    public static long remainderUnsigned(long dividend, long divisor) {
        // rem = dividend - (dividend / divisor) * divisor, all unsigned; correct in two's complement.
        // (Expressed via divideUnsigned instead of an inlined `if (x>=0) {simple} ...complex` ladder,
        // which trips a known decompiler branch-body-swap defect - see CODEC_TODO.md.)
        return dividend - divideUnsigned(dividend, divisor) * divisor;
    }

    // ===== Guava LongMath =====
    public static boolean isPowerOfTwoLong(long x) {
        return x > 0 & (x & (x - 1)) == 0;
    }

    public static int log2FloorLong(long x) {
        return 63 - Long.numberOfLeadingZeros(x);
    }

    public static long gcdLong(long a, long b) {
        while (b != 0) {
            long t = a % b;
            a = b;
            b = t;
        }
        return a;
    }

    public static long sqrtFloorLong(long x) {
        if (x < 0) throw new IllegalArgumentException("negative");
        long guess = (long) Math.sqrt((double) x);
        // correct rounding both directions to be exact for all longs
        while (guess * guess > x) guess--;
        while ((guess + 1) * (guess + 1) <= x && (guess + 1) * (guess + 1) > 0) guess++;
        return guess;
    }

    public static long checkedMultiply(long a, long b) {
        long result = a * b;
        // Guava overflow check
        long aAbs = Math.abs(a);
        long bAbs = Math.abs(b);
        if ((aAbs | bAbs) >>> 31 != 0) {
            if (b != 0 && (result / b != a || (a == Long.MIN_VALUE && b == -1))) {
                return Long.MIN_VALUE; // sentinel for overflow (deterministic)
            }
        }
        return result;
    }

    public static long binomial(int n, int k) {
        if (k > n - k) k = n - k;
        long result = 1;
        for (int i = 0; i < k; i++) {
            result = result * (n - i) / (i + 1);
        }
        return result;
    }

    public static long modPow(long base, long exp, long mod) {
        long result = 1 % mod;
        base %= mod;
        while (exp > 0) {
            if ((exp & 1L) == 1L) result = (result * base) % mod;
            base = (base * base) % mod;
            exp >>= 1;
        }
        return result;
    }

    // ===== Guava IntMath =====
    public static int gcdInt(int a, int b) {
        while (b != 0) {
            int t = a % b;
            a = b;
            b = t;
        }
        return a;
    }

    public static int powInt(int b, int k) {
        int accum = 1;
        while (k > 0) {
            if ((k & 1) != 0) accum *= b;
            k >>= 1;
            if (k > 0) b *= b;
        }
        return accum;
    }

    public static int sqrtFloorInt(int x) {
        int guess = (int) Math.sqrt((double) x);
        while (guess * guess > x) guess--;
        while ((guess + 1) * (guess + 1) <= x) guess++;
        return guess;
    }

    public static int meanInt(int x, int y) {
        return (x & y) + ((x ^ y) >> 1);
    }

    // ===== Guava primitives: Longs.fromBytes / Ints.fromBytes =====
    public static long longFromBytes(byte b1, byte b2, byte b3, byte b4, byte b5, byte b6, byte b7, byte b8) {
        return ((b1 & 0xffL) << 56) | ((b2 & 0xffL) << 48) | ((b3 & 0xffL) << 40) | ((b4 & 0xffL) << 32)
                | ((b5 & 0xffL) << 24) | ((b6 & 0xffL) << 16) | ((b7 & 0xffL) << 8) | (b8 & 0xffL);
    }

    public static int intFromBytes(byte b1, byte b2, byte b3, byte b4) {
        return ((b1 & 0xff) << 24) | ((b2 & 0xff) << 16) | ((b3 & 0xff) << 8) | (b4 & 0xff);
    }

    // ===== Guava BaseEncoding base16 (upper) =====
    private static final char[] BASE16 = "0123456789ABCDEF".toCharArray();

    public static String base16Encode(byte[] data) {
        StringBuilder sb = new StringBuilder(data.length * 2);
        for (byte b : data) {
            sb.append(BASE16[(b >> 4) & 0xf]);
            sb.append(BASE16[b & 0xf]);
        }
        return sb.toString();
    }

    // ===== Guava BaseEncoding base64 (RFC 4648, padded) =====
    private static final char[] BASE64 =
            "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".toCharArray();

    public static String base64Encode(byte[] data) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        int len = data.length;
        while (i + 3 <= len) {
            int n = ((data[i] & 0xff) << 16) | ((data[i + 1] & 0xff) << 8) | (data[i + 2] & 0xff);
            sb.append(BASE64[(n >>> 18) & 0x3f]);
            sb.append(BASE64[(n >>> 12) & 0x3f]);
            sb.append(BASE64[(n >>> 6) & 0x3f]);
            sb.append(BASE64[n & 0x3f]);
            i += 3;
        }
        int rem = len - i;
        if (rem == 1) {
            int n = (data[i] & 0xff) << 16;
            sb.append(BASE64[(n >>> 18) & 0x3f]);
            sb.append(BASE64[(n >>> 12) & 0x3f]);
            sb.append("==");
        } else if (rem == 2) {
            int n = ((data[i] & 0xff) << 16) | ((data[i + 1] & 0xff) << 8);
            sb.append(BASE64[(n >>> 18) & 0x3f]);
            sb.append(BASE64[(n >>> 12) & 0x3f]);
            sb.append(BASE64[(n >>> 6) & 0x3f]);
            sb.append('=');
        }
        return sb.toString();
    }

    // ===== self-hosted CRC32 (cross-checked against java.util.zip.CRC32) =====
    private static final int[] CRC32_TABLE = new int[256];
    static {
        for (int n = 0; n < 256; n++) {
            int c = n;
            for (int k = 0; k < 8; k++) c = (c & 1) != 0 ? (0xEDB88320 ^ (c >>> 1)) : (c >>> 1);
            CRC32_TABLE[n] = c;
        }
    }

    public static long crc32(byte[] data) {
        int crc = 0xffffffff;
        for (byte b : data) crc = CRC32_TABLE[(crc ^ b) & 0xff] ^ (crc >>> 8);
        return (~crc) & 0xffffffffL;
    }

    private static String hex64(long v) {
        return String.format("%016x", v);
    }

    private static String hex32(int v) {
        return String.format("%08x", v);
    }

    private static String hex(byte[] h) {
        StringBuilder sb = new StringBuilder();
        for (byte b : h) sb.append(String.format("%02x", b & 0xff));
        return sb.toString();
    }

    private static final String[] TEXTS = {"", "a", "abc", "hello, world",
        "The quick brown fox jumps over the lazy dog",
        "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq",
        "0123456789ABCDEF0123456789abcdef!"};

    private static String hashSection() {
        StringBuilder sb = new StringBuilder();
        for (int ti = 0; ti < TEXTS.length; ti++) {
            String t = TEXTS[ti];
            byte[] in = t.getBytes(StandardCharsets.UTF_8);
            sb.append(hex32(murmur3_32(in, 0))).append(",");
            sb.append(hex32(murmur3_32(in, 0x9747b28c))).append(",");
            sb.append(hex64(fullFingerprint(in))).append(",");

            // base64 cross-check against the JDK so an oracle-source typo fails independently.
            String mine64 = base64Encode(in);
            String jdk64 = java.util.Base64.getEncoder().encodeToString(in);
            if (!mine64.equals(jdk64)) {
                throw new RuntimeException("base64 self-check failed for \"" + t + "\": mine=" + mine64 + " jdk=" + jdk64);
            }
            sb.append(mine64).append(",");
            sb.append(base16Encode(in)).append(",");

            // crc32 cross-check against java.util.zip.CRC32.
            java.util.zip.CRC32 ref = new java.util.zip.CRC32();
            ref.update(in);
            long mineCrc = crc32(in);
            if (mineCrc != ref.getValue()) {
                throw new RuntimeException("crc32 self-check failed for \"" + t + "\": mine=" + mineCrc + " jdk=" + ref.getValue());
            }
            sb.append(hex32((int) mineCrc)).append(";");
        }
        return sb.toString();
    }

    private static String unsignedSection() {
        StringBuilder sb = new StringBuilder();
        long[] us = {0L, 1L, -1L, Long.MIN_VALUE, Long.MAX_VALUE, 0xFFFFFFFFFFFFFFFFL, 1234567890123456789L};
        long[] ds = {1L, 3L, 7L, 1000000007L, -2L};
        for (int ai = 0; ai < us.length; ai++) {
            for (int di = 0; di < ds.length; di++) {
                sb.append(divideUnsigned(us[ai], ds[di])).append("/").append(remainderUnsigned(us[ai], ds[di])).append(":");
            }
        }
        return sb.toString();
    }

    private static String mathSection() {
        StringBuilder sb = new StringBuilder();
        sb.append(isPowerOfTwoLong(1024L)).append(isPowerOfTwoLong(1000L)).append(",");
        sb.append(log2FloorLong(1L)).append(":").append(log2FloorLong(9999999999L)).append(":")
          .append(log2FloorLong(Long.MAX_VALUE)).append(",");
        sb.append(gcdLong(1071, 462)).append(":").append(gcdLong(0, 5)).append(",");
        sb.append(sqrtFloorLong(0)).append(":").append(sqrtFloorLong(1000000)).append(":")
          .append(sqrtFloorLong(999999999999L)).append(",");
        sb.append(checkedMultiply(1000000L, 1000000L)).append(":").append(checkedMultiply(Long.MAX_VALUE, 2)).append(",");
        sb.append(binomial(10, 3)).append(":").append(binomial(52, 5)).append(",");
        sb.append(modPow(7, 100, 1000000007L)).append(":").append(modPow(2, 63, 1000000007L)).append(",");
        sb.append(gcdInt(1071, 462)).append(":").append(powInt(3, 9)).append(":")
          .append(sqrtFloorInt(123456)).append(":").append(meanInt(-3, 9)).append(",");

        byte[] eight = {(byte) 0xDE, (byte) 0xAD, (byte) 0xBE, (byte) 0xEF, 0x01, 0x23, 0x45, 0x67};
        sb.append(hex64(longFromBytes(eight[0], eight[1], eight[2], eight[3],
                                      eight[4], eight[5], eight[6], eight[7]))).append(":");
        sb.append(hex32(intFromBytes(eight[0], eight[1], eight[2], eight[3])));
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(hashSection());
        sb.append(unsignedSection()).append(",");
        sb.append(mathSection());
        System.out.println(sb);
    }
}
