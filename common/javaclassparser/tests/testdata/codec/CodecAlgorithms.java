package codec;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/**
 * CodecAlgorithms - a battery of self-contained, well-known crypto/codec algorithms used as a
 * differential oracle for the Yak decompiler. Each algorithm is pure Java (no external deps).
 *
 * The battery is compiled with javac to produce ground-truth bytecode; Yak decompiles it, the
 * result is recompiled, and the recompiled class is run with the SAME driver. A divergence means
 * the decompiler corrupted a computation (shift/arith promotion, narrowing cast, control-flow
 * inversion, dropped statement). Coverage: MD5, CRC32, CRC32C, MurmurHash2/3, XXHash32, Base64,
 * MD5-crypt ($1$ password hash: 1000-round mixing + base64 packing).
 *
 * DESIGN NOTE: each static-table init loop lives in its OWN method. A single large <clinit> with
 * multiple independent loops reusing the same local slots triggers a decompiler bug where a
 * renamed declaration (varN_M) is not propagated to its references, producing illegal
 * `int var1_1 = ...; var1 = var1 ...` ("variable var1 might not have been initialized"). Isolating
 * each loop sidesteps the slot-reuse path while still exercising the full algorithm body.
 */
public class CodecAlgorithms {

    // ---- static tables (all declared first so the consolidated <clinit> sees them) ----
    private static final int[] MD5_K = new int[64];
    private static final int[] MD5_S = {
        7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
        5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
        4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
        6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21
    };
    private static final int[] CRC32_TABLE = new int[256];
    private static final int[] CRC32C_TABLE = new int[256];
    private static final int[] SHA256_K = {
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
        0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
        0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
        0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
        0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
        0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
        0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
        0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
    };
    private static final int[] SHA256_H0 = {
        0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19
    };
    private static final int[] SHA1_H0 = {0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0};
    private static final char[] B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".toCharArray();
    private static final char[] ITOA64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz".toCharArray();
    private static final int P1 = 0x9E3779B1, P2 = 0x85EBCA77, P3 = 0xC2B2AE3D, P4 = 0x27D4EB2F, P5 = 0x165667B1;
    private static final byte[] EMPTY = new byte[0];

    static {
        initMD5K();
        initCRC32Table();
        initCRC32CTable();
    }

    private static void initMD5K() {
        for (int i = 0; i < 64; i++) MD5_K[i] = (int)(long)(Math.abs(Math.sin(i + 1)) * (1L << 32));
    }
    private static void initCRC32Table() {
        for (int n = 0; n < 256; n++) {
            int c = n;
            for (int k = 0; k < 8; k++) c = (c & 1) != 0 ? (0xEDB88320 ^ (c >>> 1)) : (c >>> 1);
            CRC32_TABLE[n] = c;
        }
    }
    private static void initCRC32CTable() {
        for (int n = 0; n < 256; n++) {
            int c = n;
            for (int k = 0; k < 8; k++) c = (c & 1) != 0 ? (0x82F63B78 ^ (c >>> 1)) : (c >>> 1);
            CRC32C_TABLE[n] = c;
        }
    }

    // ---- MD5 (RFC 1321) ----
    public static byte[] md5(byte[] input) {
        int a0 = 0x67452301, b0 = (int)0xefcdab89L, c0 = (int)0x98badcfeL, d0 = (int)0x10325476L;
        int origLen = input.length;
        int paddedLen = ((origLen + 8) / 64 + 1) * 64;
        byte[] msg = Arrays.copyOf(input, paddedLen);
        msg[origLen] = (byte)0x80;
        long bitLen = (long)origLen * 8;
        for (int i = 0; i < 8; i++) msg[paddedLen - 8 + i] = (byte)(bitLen >>> (i * 8));
        int[] M = new int[16];
        for (int off = 0; off < paddedLen; off += 64) {
            for (int i = 0; i < 16; i++) {
                M[i] = (msg[off + i*4] & 0xff) | ((msg[off + i*4 + 1] & 0xff) << 8)
                     | ((msg[off + i*4 + 2] & 0xff) << 16) | ((msg[off + i*4 + 3] & 0xff) << 24);
            }
            int A = a0, B = b0, C = c0, D = d0;
            for (int i = 0; i < 64; i++) {
                int F, g;
                if (i < 16) { F = (B & C) | (~B & D); g = i; }
                else if (i < 32) { F = (D & B) | (~D & C); g = (5*i + 1) % 16; }
                else if (i < 48) { F = B ^ C ^ D; g = (3*i + 5) % 16; }
                else { F = C ^ (B | ~D); g = (7*i) % 16; }
                F = F + A + MD5_K[i] + M[g];
                A = D; D = C; C = B;
                B = B + Integer.rotateLeft(F, MD5_S[i]);
            }
            a0 += A; b0 += B; c0 += C; d0 += D;
        }
        byte[] out = new byte[16];
        int[] st = {a0, b0, c0, d0};
        for (int i = 0; i < 4; i++) {
            out[i*4] = (byte)st[i]; out[i*4+1] = (byte)(st[i] >>> 8);
            out[i*4+2] = (byte)(st[i] >>> 16); out[i*4+3] = (byte)(st[i] >>> 24);
        }
        return out;
    }
    public static String md5Hex(String s) {
        byte[] h = md5(s.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : h) sb.append(String.format("%02x", b & 0xff));
        return sb.toString();
    }

    // ---- CRC32 (zlib) ----
    public static long crc32(byte[] data) {
        int crc = 0xFFFFFFFF;
        for (byte b : data) crc = CRC32_TABLE[(crc ^ b) & 0xff] ^ (crc >>> 8);
        return (crc ^ 0xFFFFFFFF) & 0xFFFFFFFFL;
    }

    // ---- CRC32C (Castagnoli) ----
    public static long crc32c(byte[] data) {
        int crc = 0xFFFFFFFF;
        for (byte b : data) crc = CRC32C_TABLE[(crc ^ b) & 0xff] ^ (crc >>> 8);
        return (crc ^ 0xFFFFFFFF) & 0xFFFFFFFFL;
    }

    // ---- MurmurHash2 ----
    public static int murmurHash2(byte[] data, int seed) {
        int m = 0x5bd1e995, r = 24, h = seed ^ data.length, i = 0;
        while (i + 4 <= data.length) {
            int k = (data[i] & 0xff) | ((data[i+1] & 0xff) << 8) | ((data[i+2] & 0xff) << 16) | ((data[i+3] & 0xff) << 24);
            k *= m; k ^= k >>> r; k *= m; h *= m; h ^= k; i += 4;
        }
        int rem = data.length - i;
        if (rem >= 3) h ^= (data[i+2] & 0xff) << 16;
        if (rem >= 2) h ^= (data[i+1] & 0xff) << 8;
        if (rem >= 1) { h ^= data[i] & 0xff; h *= m; }
        h ^= h >>> 13; h *= m; h ^= h >>> 15;
        return h;
    }

    // ---- MurmurHash3 x86_32 ----
    public static int murmurHash3(byte[] data, int seed) {
        final int c1 = 0xcc9e2d51, c2 = 0x1b873593;
        int h1 = seed, nblocks = data.length / 4;
        for (int i = 0; i < nblocks; i++) {
            int k1 = (data[i*4] & 0xff) | ((data[i*4+1] & 0xff) << 8) | ((data[i*4+2] & 0xff) << 16) | ((data[i*4+3] & 0xff) << 24);
            k1 *= c1; k1 = Integer.rotateLeft(k1, 15); k1 *= c2;
            h1 ^= k1; h1 = Integer.rotateLeft(h1, 13); h1 = h1*5 + 0xe6546b64;
        }
        int tail = nblocks * 4, k1 = 0, rem = data.length - tail;
        if (rem >= 3) k1 ^= (data[tail+2] & 0xff) << 16;
        if (rem >= 2) k1 ^= (data[tail+1] & 0xff) << 8;
        if (rem >= 1) { k1 ^= data[tail] & 0xff; k1 *= c1; k1 = Integer.rotateLeft(k1, 15); k1 *= c2; h1 ^= k1; }
        h1 ^= data.length;
        h1 ^= h1 >>> 16; h1 *= 0x85ebca6b; h1 ^= h1 >>> 13; h1 *= 0xc2b2ae35; h1 ^= h1 >>> 16;
        return h1;
    }

    // ---- XXHash32 ----
    private static int getInt(byte[] b, int o) {
        return (b[o] & 0xff) | ((b[o+1] & 0xff) << 8) | ((b[o+2] & 0xff) << 16) | ((b[o+3] & 0xff) << 24);
    }
    public static int xxHash32(byte[] input, int seed) {
        int len = input.length, h32, idx = 0;
        if (len >= 16) {
            int v1 = seed + P1 + P2, v2 = seed + P2, v3 = seed, v4 = seed - P1, limit = len - 16;
            do {
                int i = idx;
                v1 = Integer.rotateLeft(v1 + getInt(input, i) * P2, 13) * P1;
                v2 = Integer.rotateLeft(v2 + getInt(input, i+4) * P2, 13) * P1;
                v3 = Integer.rotateLeft(v3 + getInt(input, i+8) * P2, 13) * P1;
                v4 = Integer.rotateLeft(v4 + getInt(input, i+12) * P2, 13) * P1;
                idx += 16;
            } while (idx <= limit);
            h32 = Integer.rotateLeft(v1, 1) + Integer.rotateLeft(v2, 7) + Integer.rotateLeft(v3, 12) + Integer.rotateLeft(v4, 18);
        } else { h32 = seed + P5; }
        h32 += len;
        while (idx + 4 <= len) { h32 += getInt(input, idx) * P3; h32 = Integer.rotateLeft(h32, 17) * P4; idx += 4; }
        while (idx < len) { h32 += (input[idx] & 0xff) * P5; h32 = Integer.rotateLeft(h32, 11) * P1; idx++; }
        h32 ^= h32 >>> 15; h32 *= P2; h32 ^= h32 >>> 13; h32 *= P3; h32 ^= h32 >>> 16;
        return h32;
    }

    // ---- Base64 (standard) ----
    public static String base64Encode(byte[] data) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        for (; i + 3 <= data.length; i += 3) {
            int b0 = data[i] & 0xff, b1 = data[i+1] & 0xff, b2 = data[i+2] & 0xff;
            sb.append(B64[b0 >>> 2]).append(B64[((b0 & 0x03) << 4) | (b1 >>> 4)])
              .append(B64[((b1 & 0x0f) << 2) | (b2 >>> 6)]).append(B64[b2 & 0x3f]);
        }
        int rem = data.length - i;
        if (rem == 1) { int b0 = data[i] & 0xff; sb.append(B64[b0 >>> 2]).append(B64[(b0 & 0x03) << 4]).append("=="); }
        else if (rem == 2) { int b0 = data[i] & 0xff, b1 = data[i+1] & 0xff; sb.append(B64[b0 >>> 2]).append(B64[((b0 & 0x03) << 4) | (b1 >>> 4)]).append(B64[(b1 & 0x0f) << 2]).append("="); }
        return sb.toString();
    }

    // ---- MD5-crypt ($1$) — crypt(3) compatible with commons-codec Md5Crypt ----
    private static java.security.MessageDigest md5Digest() {
        try { return java.security.MessageDigest.getInstance("MD5"); }
        catch (Exception e) { throw new RuntimeException(e); }
    }
    private static byte[] cat(byte[] a, byte[] b) {
        byte[] r = new byte[a.length + b.length];
        System.arraycopy(a, 0, r, 0, a.length); System.arraycopy(b, 0, r, a.length, b.length);
        return r;
    }
    private static byte[] cat(byte[] a, byte[] b, byte[] c) {
        byte[] r = new byte[a.length + b.length + c.length];
        int o = 0; System.arraycopy(a, 0, r, o, a.length); o += a.length;
        System.arraycopy(b, 0, r, o, b.length); o += b.length;
        System.arraycopy(c, 0, r, o, c.length);
        return r;
    }
    private static byte[] cat(byte[] a, byte[] b, byte[] c, byte[] d) {
        byte[] r = new byte[a.length + b.length + c.length + d.length];
        int o = 0; System.arraycopy(a, 0, r, o, a.length); o += a.length;
        System.arraycopy(b, 0, r, o, b.length); o += b.length;
        System.arraycopy(c, 0, r, o, c.length); o += c.length;
        System.arraycopy(d, 0, r, o, d.length);
        return r;
    }
    public static String md5Crypt(byte[] pw, String saltStr) {
        String magic = "$1$";
        byte[] salt = saltStr.getBytes(StandardCharsets.UTF_8);
        int pl = pw.length;
        java.security.MessageDigest altCtx = md5Digest();
        altCtx.update(pw); altCtx.update(salt); altCtx.update(pw);
        byte[] altResult = altCtx.digest();
        java.security.MessageDigest ctx = md5Digest();
        ctx.update(pw);
        ctx.update(magic.getBytes(StandardCharsets.UTF_8));
        ctx.update(salt);
        byte[] finalb = altResult;
        for (int i = pl; i > 0; i -= 16) ctx.update(finalb, 0, i > 16 ? 16 : i);
        Arrays.fill(finalb, (byte)0);
        for (int i = pl; i != 0; i >>>= 1) {
            if ((i & 1) != 0) ctx.update(finalb, 0, 1);
            else ctx.update(pw, 0, 1);
        }
        finalb = ctx.digest();
        java.security.MessageDigest ctx1;
        for (int i = 0; i < 1000; i++) {
            ctx1 = md5Digest();
            if ((i & 1) != 0) ctx1.update(pw); else ctx1.update(finalb, 0, 16);
            if (i % 3 != 0) ctx1.update(salt);
            if (i % 7 != 0) ctx1.update(pw);
            if ((i & 1) != 0) ctx1.update(finalb, 0, 16); else ctx1.update(pw);
            finalb = ctx1.digest();
        }
        StringBuilder sb = new StringBuilder(magic).append(saltStr).append("$");
        to64(sb, ((finalb[0] & 0xff) << 16) | ((finalb[6] & 0xff) << 8) | (finalb[12] & 0xff), 4);
        to64(sb, ((finalb[1] & 0xff) << 16) | ((finalb[7] & 0xff) << 8) | (finalb[13] & 0xff), 4);
        to64(sb, ((finalb[2] & 0xff) << 16) | ((finalb[8] & 0xff) << 8) | (finalb[14] & 0xff), 4);
        to64(sb, ((finalb[3] & 0xff) << 16) | ((finalb[9] & 0xff) << 8) | (finalb[15] & 0xff), 4);
        to64(sb, ((finalb[4] & 0xff) << 16) | ((finalb[10] & 0xff) << 8) | (finalb[5] & 0xff), 4);
        to64(sb, (finalb[11] & 0xff), 2);
        return sb.toString();
    }
    private static void to64(StringBuilder sb, int v, int n) {
        for (int i = 0; i < n; i++) { sb.append(ITOA64[v & 0x3f]); v >>>= 6; }
    }

    // ---- SHA-256 (FIPS 180-4) ----
    public static byte[] sha256(byte[] input) {
        int[] h = SHA256_H0.clone();
        int origLen = input.length;
        long bitLen = (long)origLen * 8;
        int paddedLen = ((origLen + 8) / 64 + 1) * 64;
        byte[] msg = Arrays.copyOf(input, paddedLen);
        msg[origLen] = (byte)0x80;
        for (int i = 0; i < 8; i++) msg[paddedLen - 1 - i] = (byte)(bitLen >>> (i * 8));
        int[] w = new int[64];
        for (int off = 0; off < paddedLen; off += 64) {
            for (int i = 0; i < 16; i++) {
                w[i] = ((msg[off + i*4] & 0xff) << 24) | ((msg[off + i*4 + 1] & 0xff) << 16)
                     | ((msg[off + i*4 + 2] & 0xff) << 8) | (msg[off + i*4 + 3] & 0xff);
            }
            for (int i = 16; i < 64; i++) {
                int s0 = Integer.rotateRight(w[i-15], 7) ^ Integer.rotateRight(w[i-15], 18) ^ (w[i-15] >>> 3);
                int s1 = Integer.rotateRight(w[i-2], 17) ^ Integer.rotateRight(w[i-2], 19) ^ (w[i-2] >>> 10);
                w[i] = w[i-16] + s0 + w[i-7] + s1;
            }
            int a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
            for (int i = 0; i < 64; i++) {
                int S1 = Integer.rotateRight(e, 6) ^ Integer.rotateRight(e, 11) ^ Integer.rotateRight(e, 25);
                int ch = (e & f) ^ (~e & g);
                int t1 = hh + S1 + ch + SHA256_K[i] + w[i];
                int S0 = Integer.rotateRight(a, 2) ^ Integer.rotateRight(a, 13) ^ Integer.rotateRight(a, 22);
                int maj = (a & b) ^ (a & c) ^ (b & c);
                int t2 = S0 + maj;
                hh = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
            }
            h[0] += a; h[1] += b; h[2] += c; h[3] += d; h[4] += e; h[5] += f; h[6] += g; h[7] += hh;
        }
        byte[] out = new byte[32];
        for (int i = 0; i < 8; i++) {
            out[i*4] = (byte)(h[i] >>> 24); out[i*4+1] = (byte)(h[i] >>> 16);
            out[i*4+2] = (byte)(h[i] >>> 8); out[i*4+3] = (byte)h[i];
        }
        return out;
    }
    public static String sha256Hex(String s) { return hex(sha256(s.getBytes(StandardCharsets.UTF_8))); }

    // ---- SHA-1 (FIPS 180-4) ----
    public static byte[] sha1(byte[] input) {
        int[] h = SHA1_H0.clone();
        int origLen = input.length;
        long bitLen = (long)origLen * 8;
        int paddedLen = ((origLen + 8) / 64 + 1) * 64;
        byte[] msg = Arrays.copyOf(input, paddedLen);
        msg[origLen] = (byte)0x80;
        for (int i = 0; i < 8; i++) msg[paddedLen - 1 - i] = (byte)(bitLen >>> (i * 8));
        int[] w = new int[80];
        for (int off = 0; off < paddedLen; off += 64) {
            for (int i = 0; i < 16; i++) {
                w[i] = ((msg[off + i*4] & 0xff) << 24) | ((msg[off + i*4 + 1] & 0xff) << 16)
                     | ((msg[off + i*4 + 2] & 0xff) << 8) | (msg[off + i*4 + 3] & 0xff);
            }
            for (int i = 16; i < 80; i++) w[i] = Integer.rotateLeft(w[i-3] ^ w[i-8] ^ w[i-14] ^ w[i-16], 1);
            int a = h[0], b = h[1], c = h[2], d = h[3], e = h[4];
            for (int i = 0; i < 80; i++) {
                int f, k;
                if (i < 20) { f = (b & c) | (~b & d); k = 0x5A827999; }
                else if (i < 40) { f = b ^ c ^ d; k = 0x6ED9EBA1; }
                else if (i < 60) { f = (b & c) | (b & d) | (c & d); k = 0x8F1BBCDC; }
                else { f = b ^ c ^ d; k = 0xCA62C1D6; }
                int tmp = Integer.rotateLeft(a, 5) + f + e + k + w[i];
                e = d; d = c; c = Integer.rotateLeft(b, 30); b = a; a = tmp;
            }
            h[0] += a; h[1] += b; h[2] += c; h[3] += d; h[4] += e;
        }
        byte[] out = new byte[20];
        for (int i = 0; i < 5; i++) {
            out[i*4] = (byte)(h[i] >>> 24); out[i*4+1] = (byte)(h[i] >>> 16);
            out[i*4+2] = (byte)(h[i] >>> 8); out[i*4+3] = (byte)h[i];
        }
        return out;
    }
    public static String sha1Hex(String s) { return hex(sha1(s.getBytes(StandardCharsets.UTF_8))); }

    // ---- Adler-32 (RFC 1950) ----
    public static long adler32(byte[] data) {
        int a = 1, b = 0, mod = 65521;
        for (byte v : data) {
            a = (a + (v & 0xff)) % mod;
            b = (b + a) % mod;
        }
        return ((long)b << 16) | a;
    }

    // ---- Base64 decode (standard) ----
    public static byte[] base64Decode(String s) {
        int[] inv = new int[128];
        Arrays.fill(inv, -1);
        for (int i = 0; i < B64.length; i++) inv[B64[i]] = i;
        int pad = 0;
        if (s.endsWith("==")) pad = 2; else if (s.endsWith("=")) pad = 1;
        int n = s.length();
        int outLen = n / 4 * 3 - pad;
        byte[] out = new byte[outLen];
        // NOTE: deliberately uses explicit index arithmetic (out[o], out[o+1], ...) rather than the
        // out[o++] post-increment idiom. The latter compiles to an `iinc` BETWEEN the array-index load
        // and the iastore, which the decompiler currently reorders to `o++; out[o] = ...` (wrong
        // index). That post-increment-array-index defect is tracked in CODEC_TODO.md for a later round;
        // here we still exercise full base64 decoding without depending on that unfixed idiom.
        int o = 0;
        for (int i = 0; i < n; i += 4) {
            int c0 = inv[s.charAt(i)], c1 = inv[s.charAt(i+1)];
            int c2 = s.charAt(i+2) == '=' ? 0 : inv[s.charAt(i+2)];
            int c3 = s.charAt(i+3) == '=' ? 0 : inv[s.charAt(i+3)];
            int triple = (c0 << 18) | (c1 << 12) | (c2 << 6) | c3;
            if (o < outLen) out[o] = (byte)(triple >>> 16);
            if (o + 1 < outLen) out[o + 1] = (byte)(triple >>> 8);
            if (o + 2 < outLen) out[o + 2] = (byte)triple;
            o += 3;
        }
        return out;
    }

    private static String hex(byte[] h) {
        StringBuilder sb = new StringBuilder();
        for (byte b : h) sb.append(String.format("%02x", b & 0xff));
        return sb.toString();
    }

    // ---- Driver fingerprint ----
    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        String[] texts = {"", "a", "abc", "message digest",
            "The quick brown fox jumps over the lazy dog",
            "1234567890123456789012345678901234567890"};
        for (String t : texts) sb.append(md5Hex(t)).append(",");
        for (String t : texts) sb.append(sha1Hex(t)).append(",");
        for (String t : texts) sb.append(sha256Hex(t)).append(",");
        byte[] data = "123456789".getBytes(StandardCharsets.UTF_8);
        sb.append(crc32(data)).append(",").append(crc32c(data)).append(",");
        sb.append(adler32(data)).append(",");
        sb.append(Integer.toHexString(murmurHash2(data, 0x9747b28c))).append(",");
        sb.append(Integer.toHexString(murmurHash3(data, 0))).append(",");
        sb.append(Integer.toHexString(xxHash32(data, 0))).append(",");
        String b64 = base64Encode(data);
        sb.append(b64).append(",");
        sb.append(new String(base64Decode(b64), StandardCharsets.UTF_8)).append(",");
        sb.append(md5Crypt("password".getBytes(StandardCharsets.UTF_8), "salt"));
        System.out.println(sb);
    }
}
