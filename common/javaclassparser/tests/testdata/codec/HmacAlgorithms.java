package codec;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * HmacAlgorithms - self-contained HMAC-MD5 / HMAC-SHA256 (RFC 2104) battery for the Yak decompiler
 * differential-execution oracle. The MD5 and SHA-256 cores are the verified implementations from
 * CodecAlgorithms; the new control flow under test lives in hmac(): the key-normalization branch
 * (hash an over-long key, zero-pad a short key), the ipad/opad XOR loops, and the two-pass hashing
 * dispatched through a ternary over invokestatic.
 *
 * Each result is cross-checked at runtime against the JDK javax.crypto.Mac reference; a mismatch
 * throws, so a broken oracle (or a decompiler that corrupts the algorithm) fails loudly when the
 * battery is run, on top of the fingerprint comparison performed by the round-trip harness.
 */
public class HmacAlgorithms {

    private static final int[] MD5_K = new int[64];
    private static final int[] MD5_S = {
        7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
        5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
        4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
        6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21
    };
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

    static {
        for (int i = 0; i < 64; i++) MD5_K[i] = (int)(long)(Math.abs(Math.sin(i + 1)) * (1L << 32));
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

    // algo selector: 0 = MD5, anything else = SHA-256. Ternary over two invokestatic targets.
    private static byte[] hash(int algo, byte[] x) {
        return algo == 0 ? md5(x) : sha256(x);
    }

    // ---- HMAC (RFC 2104), generic over the inner hash ----
    public static byte[] hmac(int algo, byte[] key, byte[] message) {
        int blockSize = 64;
        byte[] k = key;
        if (k.length > blockSize) {
            k = hash(algo, k);
        }
        if (k.length < blockSize) {
            k = Arrays.copyOf(k, blockSize);
        }
        byte[] ipad = new byte[blockSize];
        byte[] opad = new byte[blockSize];
        for (int i = 0; i < blockSize; i++) {
            ipad[i] = (byte) (k[i] ^ 0x36);
            opad[i] = (byte) (k[i] ^ 0x5c);
        }
        byte[] innerInput = new byte[blockSize + message.length];
        System.arraycopy(ipad, 0, innerInput, 0, blockSize);
        System.arraycopy(message, 0, innerInput, blockSize, message.length);
        byte[] inner = hash(algo, innerInput);
        byte[] outerInput = new byte[blockSize + inner.length];
        System.arraycopy(opad, 0, outerInput, 0, blockSize);
        System.arraycopy(inner, 0, outerInput, blockSize, inner.length);
        return hash(algo, outerInput);
    }

    private static String hex(byte[] h) {
        StringBuilder sb = new StringBuilder();
        for (byte b : h) sb.append(String.format("%02x", b & 0xff));
        return sb.toString();
    }

    private static byte[] jdkHmac(String alg, byte[] key, byte[] msg) {
        try {
            Mac mac = Mac.getInstance(alg);
            mac.init(new SecretKeySpec(key, alg));
            return mac.doFinal(msg);
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    // Cross-check the self-hosted HMAC against the JDK Mac reference; throw on divergence so a broken
    // oracle (or a decompiler-corrupted algorithm) aborts the run with a non-zero exit.
    private static String checked(int algo, String jdkAlg, byte[] key, byte[] msg) {
        byte[] mine = hmac(algo, key, msg);
        byte[] ref = jdkHmac(jdkAlg, key, msg);
        if (!Arrays.equals(mine, ref)) {
            throw new IllegalStateException(jdkAlg + " mismatch: mine=" + hex(mine) + " ref=" + hex(ref));
        }
        return hex(mine);
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        // Keys chosen to exercise all three normalization paths: shorter than the 64-byte block,
        // exactly the block size, and longer than the block (forcing the "hash the key" branch).
        byte[] shortKey = "key".getBytes(StandardCharsets.UTF_8);
        byte[] blockKey = new byte[64];
        for (int i = 0; i < blockKey.length; i++) blockKey[i] = (byte) (i + 1);
        byte[] longKey = new byte[100];
        for (int i = 0; i < longKey.length; i++) longKey[i] = (byte) (i * 7 + 3);
        byte[][] keys = { shortKey, blockKey, longKey };
        String[] msgs = {
            "",
            "Hi There",
            "The quick brown fox jumps over the lazy dog",
            "abcdefghijklmnopqrstuvwxyz0123456789"
        };
        for (byte[] key : keys) {
            for (String m : msgs) {
                byte[] msg = m.getBytes(StandardCharsets.UTF_8);
                sb.append(checked(0, "HmacMD5", key, msg)).append(":");
                sb.append(checked(1, "HmacSHA256", key, msg)).append(";");
            }
            sb.append("|");
        }
        System.out.println(sb);
    }
}
