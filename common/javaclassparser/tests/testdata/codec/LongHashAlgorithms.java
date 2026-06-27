package codec;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/**
 * LongHashAlgorithms - a self-contained battery of 64-bit (long-heavy) hash/codec algorithms used as
 * a differential-execution oracle for the Yak decompiler. Its purpose is to exercise the long-typed
 * opcode family that the int-centric CodecAlgorithms battery barely touches: LADD/LSUB/LMUL/LDIV/
 * LREM, LSHL/LSHR/LUSHR, LAND/LOR/LXOR, LCMP, I2L/L2I, LALOAD/LASTORE (long[] tables), plus
 * Long.rotateLeft/rotateRight.
 *
 * Self-checking: sha512() is cross-validated against the JDK's MessageDigest("SHA-512") inside main()
 * so a typo in the round constants fails the oracle source itself (not the decompiler). All other
 * algorithms are deterministic; the round-trip oracle (decompile -> recompile -> run) asserts the
 * decompiler preserved their long arithmetic byte-for-byte.
 *
 * Single public top-level class, only static methods, no inner/extra top-level classes, so the
 * single-class decompile round-trip stays well-defined.
 */
public class LongHashAlgorithms {

    // ---- SHA-512 constants (FIPS 180-4) ----
    private static final long[] SHA512_K = {
        0x428a2f98d728ae22L, 0x7137449123ef65cdL, 0xb5c0fbcfec4d3b2fL, 0xe9b5dba58189dbbcL,
        0x3956c25bf348b538L, 0x59f111f1b605d019L, 0x923f82a4af194f9bL, 0xab1c5ed5da6d8118L,
        0xd807aa98a3030242L, 0x12835b0145706fbeL, 0x243185be4ee4b28cL, 0x550c7dc3d5ffb4e2L,
        0x72be5d74f27b896fL, 0x80deb1fe3b1696b1L, 0x9bdc06a725c71235L, 0xc19bf174cf692694L,
        0xe49b69c19ef14ad2L, 0xefbe4786384f25e3L, 0x0fc19dc68b8cd5b5L, 0x240ca1cc77ac9c65L,
        0x2de92c6f592b0275L, 0x4a7484aa6ea6e483L, 0x5cb0a9dcbd41fbd4L, 0x76f988da831153b5L,
        0x983e5152ee66dfabL, 0xa831c66d2db43210L, 0xb00327c898fb213fL, 0xbf597fc7beef0ee4L,
        0xc6e00bf33da88fc2L, 0xd5a79147930aa725L, 0x06ca6351e003826fL, 0x142929670a0e6e70L,
        0x27b70a8546d22ffcL, 0x2e1b21385c26c926L, 0x4d2c6dfc5ac42aedL, 0x53380d139d95b3dfL,
        0x650a73548baf63deL, 0x766a0abb3c77b2a8L, 0x81c2c92e47edaee6L, 0x92722c851482353bL,
        0xa2bfe8a14cf10364L, 0xa81a664bbc423001L, 0xc24b8b70d0f89791L, 0xc76c51a30654be30L,
        0xd192e819d6ef5218L, 0xd69906245565a910L, 0xf40e35855771202aL, 0x106aa07032bbd1b8L,
        0x19a4c116b8d2d0c8L, 0x1e376c085141ab53L, 0x2748774cdf8eeb99L, 0x34b0bcb5e19b48a8L,
        0x391c0cb3c5c95a63L, 0x4ed8aa4ae3418acbL, 0x5b9cca4f7763e373L, 0x682e6ff3d6b2b8a3L,
        0x748f82ee5defb2fcL, 0x78a5636f43172f60L, 0x84c87814a1f0ab72L, 0x8cc702081a6439ecL,
        0x90befffa23631e28L, 0xa4506cebde82bde9L, 0xbef9a3f7b2c67915L, 0xc67178f2e372532bL,
        0xca273eceea26619cL, 0xd186b8c721c0c207L, 0xeada7dd6cde0eb1eL, 0xf57d4f7fee6ed178L,
        0x06f067aa72176fbaL, 0x0a637dc5a2c898a6L, 0x113f9804bef90daeL, 0x1b710b35131c471bL,
        0x28db77f523047d84L, 0x32caab7b40c72493L, 0x3c9ebe0a15c9bebcL, 0x431d67c49c100d4cL,
        0x4cc5d4becb3e42b6L, 0x597f299cfc657e2aL, 0x5fcb6fab3ad6faecL, 0x6c44198c4a475817L
    };
    private static final long[] SHA512_H0 = {
        0x6a09e667f3bcc908L, 0xbb67ae8584caa73bL, 0x3c6ef372fe94f82bL, 0xa54ff53a5f1d36f1L,
        0x510e527fade682d1L, 0x9b05688c2b3e6c1fL, 0x1f83d9abfb41bd6bL, 0x5be0cd19137e2179L
    };

    private static final long[] CRC64_TABLE = new long[256];
    static {
        for (int n = 0; n < 256; n++) {
            long c = n;
            for (int k = 0; k < 8; k++) c = (c & 1L) != 0 ? (0xC96C5795D7870F42L ^ (c >>> 1)) : (c >>> 1);
            CRC64_TABLE[n] = c;
        }
    }

    // ---- SHA-512 (FIPS 180-4): 128-byte blocks, 80 rounds of long arithmetic ----
    public static byte[] sha512(byte[] input) {
        long[] h = SHA512_H0.clone();
        int origLen = input.length;
        // padded length is a multiple of 128; reserve 16 bytes for the 128-bit length (we only fill 8)
        int paddedLen = (((origLen + 16) / 128) + 1) * 128;
        byte[] msg = Arrays.copyOf(input, paddedLen);
        msg[origLen] = (byte) 0x80;
        long bitLen = (long) origLen * 8;
        for (int i = 0; i < 8; i++) msg[paddedLen - 1 - i] = (byte) (bitLen >>> (i * 8));
        long[] w = new long[80];
        for (int off = 0; off < paddedLen; off += 128) {
            for (int i = 0; i < 16; i++) {
                long word = 0;
                for (int j = 0; j < 8; j++) word = (word << 8) | (msg[off + i * 8 + j] & 0xffL);
                w[i] = word;
            }
            for (int i = 16; i < 80; i++) {
                long s0 = Long.rotateRight(w[i - 15], 1) ^ Long.rotateRight(w[i - 15], 8) ^ (w[i - 15] >>> 7);
                long s1 = Long.rotateRight(w[i - 2], 19) ^ Long.rotateRight(w[i - 2], 61) ^ (w[i - 2] >>> 6);
                w[i] = w[i - 16] + s0 + w[i - 7] + s1;
            }
            long a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
            for (int i = 0; i < 80; i++) {
                long S1 = Long.rotateRight(e, 14) ^ Long.rotateRight(e, 18) ^ Long.rotateRight(e, 41);
                long ch = (e & f) ^ (~e & g);
                long t1 = hh + S1 + ch + SHA512_K[i] + w[i];
                long S0 = Long.rotateRight(a, 28) ^ Long.rotateRight(a, 34) ^ Long.rotateRight(a, 39);
                long maj = (a & b) ^ (a & c) ^ (b & c);
                long t2 = S0 + maj;
                hh = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
            }
            h[0] += a; h[1] += b; h[2] += c; h[3] += d; h[4] += e; h[5] += f; h[6] += g; h[7] += hh;
        }
        byte[] out = new byte[64];
        for (int i = 0; i < 8; i++) {
            for (int j = 0; j < 8; j++) out[i * 8 + j] = (byte) (h[i] >>> (56 - j * 8));
        }
        return out;
    }

    // ---- xxHash64 ----
    private static final long XP1 = 0x9E3779B185EBCA87L, XP2 = 0xC2B2AE3D27D4EB4FL,
            XP3 = 0x165667B19E3779F9L, XP4 = 0x85EBCA77C2B2AE63L, XP5 = 0x27D4EB2F165667C5L;

    private static long xxRound(long acc, long input) {
        acc += input * XP2;
        acc = Long.rotateLeft(acc, 31);
        acc *= XP1;
        return acc;
    }

    private static long xxMergeRound(long acc, long val) {
        val = xxRound(0, val);
        acc ^= val;
        acc = acc * XP1 + XP4;
        return acc;
    }

    private static long getLongLE(byte[] b, int o) {
        long r = 0;
        for (int i = 0; i < 8; i++) r |= (b[o + i] & 0xffL) << (8 * i);
        return r;
    }

    private static long getIntLE(byte[] b, int o) {
        return (b[o] & 0xffL) | ((b[o + 1] & 0xffL) << 8) | ((b[o + 2] & 0xffL) << 16) | ((b[o + 3] & 0xffL) << 24);
    }

    public static long xxHash64(byte[] in, long seed) {
        int len = in.length, idx = 0;
        long h;
        if (len >= 32) {
            long v1 = seed + XP1 + XP2, v2 = seed + XP2, v3 = seed, v4 = seed - XP1;
            int limit = len - 32;
            do {
                v1 = xxRound(v1, getLongLE(in, idx)); idx += 8;
                v2 = xxRound(v2, getLongLE(in, idx)); idx += 8;
                v3 = xxRound(v3, getLongLE(in, idx)); idx += 8;
                v4 = xxRound(v4, getLongLE(in, idx)); idx += 8;
            } while (idx <= limit);
            h = Long.rotateLeft(v1, 1) + Long.rotateLeft(v2, 7) + Long.rotateLeft(v3, 12) + Long.rotateLeft(v4, 18);
            h = xxMergeRound(h, v1); h = xxMergeRound(h, v2); h = xxMergeRound(h, v3); h = xxMergeRound(h, v4);
        } else {
            h = seed + XP5;
        }
        h += len;
        while (idx + 8 <= len) {
            long k1 = xxRound(0, getLongLE(in, idx));
            h ^= k1;
            h = Long.rotateLeft(h, 27) * XP1 + XP4;
            idx += 8;
        }
        if (idx + 4 <= len) {
            h ^= getIntLE(in, idx) * XP1;
            h = Long.rotateLeft(h, 23) * XP2 + XP3;
            idx += 4;
        }
        while (idx < len) {
            h ^= (in[idx] & 0xffL) * XP5;
            h = Long.rotateLeft(h, 11) * XP1;
            idx++;
        }
        h ^= h >>> 33; h *= XP2; h ^= h >>> 29; h *= XP3; h ^= h >>> 32;
        return h;
    }

    // ---- SipHash-2-4 (long[] state exercises LALOAD/LASTORE) ----
    private static void sipRound(long[] v) {
        v[0] += v[1]; v[1] = Long.rotateLeft(v[1], 13); v[1] ^= v[0]; v[0] = Long.rotateLeft(v[0], 32);
        v[2] += v[3]; v[3] = Long.rotateLeft(v[3], 16); v[3] ^= v[2];
        v[0] += v[3]; v[3] = Long.rotateLeft(v[3], 21); v[3] ^= v[0];
        v[2] += v[1]; v[1] = Long.rotateLeft(v[1], 17); v[1] ^= v[2]; v[2] = Long.rotateLeft(v[2], 32);
    }

    public static long sipHash24(byte[] data, long k0, long k1) {
        long[] v = {
            0x736f6d6570736575L ^ k0,
            0x646f72616e646f6dL ^ k1,
            0x6c7967656e657261L ^ k0,
            0x7465646279746573L ^ k1
        };
        int len = data.length;
        int end = len - (len % 8);
        int i = 0;
        for (; i < end; i += 8) {
            long m = getLongLE(data, i);
            v[3] ^= m;
            sipRound(v);
            sipRound(v);
            v[0] ^= m;
        }
        long b = (long) len << 56;
        for (int j = 0; i + j < len; j++) b |= (data[i + j] & 0xffL) << (8 * j);
        v[3] ^= b;
        sipRound(v);
        sipRound(v);
        v[0] ^= b;
        v[2] ^= 0xffL;
        sipRound(v);
        sipRound(v);
        sipRound(v);
        sipRound(v);
        return v[0] ^ v[1] ^ v[2] ^ v[3];
    }

    // ---- CRC64 (XZ / ECMA-182 reflected) ----
    public static long crc64(byte[] data) {
        long crc = 0xFFFFFFFFFFFFFFFFL;
        for (byte b : data) crc = CRC64_TABLE[(int) ((crc ^ b) & 0xff)] ^ (crc >>> 8);
        return ~crc;
    }

    // ---- FNV-1a / FNV-1 (64-bit) ----
    public static long fnv1a64(byte[] data) {
        long h = 0xcbf29ce484222325L;
        for (byte b : data) {
            h ^= (b & 0xffL);
            h *= 0x100000001b3L;
        }
        return h;
    }

    public static long fnv1_64(byte[] data) {
        long h = 0xcbf29ce484222325L;
        for (byte b : data) {
            h *= 0x100000001b3L;
            h ^= (b & 0xffL);
        }
        return h;
    }

    // ---- splitmix64 (Steele/Vigna) ----
    public static long splitmix64(long x) {
        x += 0x9E3779B97F4A7C15L;
        long z = x;
        z = (z ^ (z >>> 30)) * 0xBF58476D1CE4E5B9L;
        z = (z ^ (z >>> 27)) * 0x94D049BB133111EBL;
        return z ^ (z >>> 31);
    }

    // ---- manual unsigned-long radix formatter (forces real LDIV / LREM / LCMP) ----
    public static String toRadixUnsigned(long value, int radix) {
        if (value == 0) return "0";
        char[] digits = "0123456789abcdefghijklmnopqrstuvwxyz".toCharArray();
        StringBuilder sb = new StringBuilder();
        // handle the high bit without unsigned division by peeling one step when negative
        if (value < 0) {
            long q = (value >>> 1) / radix * 2;
            long r = value - q * radix;
            if (r >= radix) {
                r -= radix;
                q += 1;
            }
            value = q;
            sb.append(digits[(int) r]);
            // now value is non-negative; fall through to the loop and reverse at the end
            StringBuilder head = new StringBuilder();
            while (value != 0) {
                head.append(digits[(int) (value % radix)]);
                value /= radix;
            }
            return head.reverse().toString() + sb.toString();
        }
        while (value != 0) {
            sb.append(digits[(int) (value % radix)]);
            value /= radix;
        }
        return sb.reverse().toString();
    }

    private static String hex64(long v) {
        return String.format("%016x", v);
    }

    private static String hex(byte[] h) {
        StringBuilder sb = new StringBuilder();
        for (byte b : h) sb.append(String.format("%02x", b & 0xff));
        return sb.toString();
    }

    private static String jdkSha512Hex(byte[] in) {
        try {
            return hex(java.security.MessageDigest.getInstance("SHA-512").digest(in));
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        String[] texts = {"", "a", "abc", "message digest",
            "The quick brown fox jumps over the lazy dog",
            "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"};
        // SHA-512 self-check: our implementation MUST match the JDK, else the oracle source is wrong.
        for (String t : texts) {
            byte[] in = t.getBytes(StandardCharsets.UTF_8);
            String mine = hex(sha512(in));
            String jdk = jdkSha512Hex(in);
            if (!mine.equals(jdk)) {
                throw new RuntimeException("sha512 self-check failed for \"" + t + "\": mine=" + mine + " jdk=" + jdk);
            }
            sb.append(mine).append(",");
        }
        byte[] data = "The quick brown fox jumps over the lazy dog".getBytes(StandardCharsets.UTF_8);
        sb.append(hex64(xxHash64(data, 0))).append(",");
        sb.append(hex64(xxHash64(data, 0x9E3779B185EBCA87L))).append(",");
        sb.append(hex64(xxHash64("".getBytes(StandardCharsets.UTF_8), 0))).append(",");
        sb.append(hex64(sipHash24(data, 0x0706050403020100L, 0x0f0e0d0c0b0a0908L))).append(",");
        sb.append(hex64(crc64(data))).append(",");
        sb.append(hex64(fnv1a64(data))).append(",");
        sb.append(hex64(fnv1_64(data))).append(",");
        long s = 0x1234567890abcdefL;
        for (int i = 0; i < 5; i++) {
            s = splitmix64(s);
            sb.append(hex64(s)).append(",");
        }
        sb.append(toRadixUnsigned(0xFFFFFFFFFFFFFFFFL, 16)).append(",");
        sb.append(toRadixUnsigned(0xFFFFFFFFFFFFFFFFL, 10)).append(",");
        sb.append(toRadixUnsigned(123456789012345L, 36));
        System.out.println(sb);
    }
}
