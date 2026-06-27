package codec;

/**
 * Sha256Algorithms - a from-scratch SHA-256 (FIPS 180-4) implementation used as a heavy correctness
 * oracle. No java.security: every rotation, `>>>` fill-shift, int-array message schedule, and the
 * 64-round compression loop is decompiled and round-tripped, so any miscompiled shift/promotion,
 * dropped statement, or wrong loop bound surfaces as a digest mismatch.
 *
 * Exercises: int rotate-right via `(x >>> n) | (x << (32-n))`, logical right shift, big-endian
 * byte<->int packing with sign-masking, message padding with a 64-bit length, nested loops, and a
 * long-lived 8-way running state (a..h) reassigned every round (slot churn).
 *
 * Sanity (verified by batterySanity): SHA-256("") = e3b0c442...b855.
 * Single public top-level class, static only, deterministic.
 */
public class Sha256Algorithms {

    private static final int[] K = {
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
        0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
        0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
        0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
        0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
        0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
        0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
        0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
    };

    private static int rotr(int x, int n) {
        return (x >>> n) | (x << (32 - n));
    }

    public static byte[] sha256(byte[] message) {
        int[] h = {
            0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
            0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19
        };

        int origLen = message.length;
        long bitLen = (long) origLen * 8L;
        int rem = origLen % 64;
        int padLen = (rem < 56) ? (56 - rem) : (120 - rem);
        byte[] padded = new byte[origLen + padLen + 8];
        for (int i = 0; i < origLen; i++) {
            padded[i] = message[i];
        }
        padded[origLen] = (byte) 0x80;
        for (int i = 0; i < 8; i++) {
            padded[padded.length - 1 - i] = (byte) (bitLen >>> (8 * i));
        }

        int[] w = new int[64];
        for (int chunk = 0; chunk < padded.length; chunk += 64) {
            for (int i = 0; i < 16; i++) {
                int j = chunk + i * 4;
                w[i] = ((padded[j] & 0xff) << 24)
                     | ((padded[j + 1] & 0xff) << 16)
                     | ((padded[j + 2] & 0xff) << 8)
                     | (padded[j + 3] & 0xff);
            }
            for (int i = 16; i < 64; i++) {
                int s0 = rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >>> 3);
                int s1 = rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >>> 10);
                w[i] = w[i - 16] + s0 + w[i - 7] + s1;
            }

            int a = h[0], b = h[1], c = h[2], d = h[3];
            int e = h[4], f = h[5], g = h[6], hh = h[7];

            for (int i = 0; i < 64; i++) {
                int bigS1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
                int ch = (e & f) ^ ((~e) & g);
                int temp1 = hh + bigS1 + ch + K[i] + w[i];
                int bigS0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
                int maj = (a & b) ^ (a & c) ^ (b & c);
                int temp2 = bigS0 + maj;
                hh = g;
                g = f;
                f = e;
                e = d + temp1;
                d = c;
                c = b;
                b = a;
                a = temp1 + temp2;
            }

            h[0] += a; h[1] += b; h[2] += c; h[3] += d;
            h[4] += e; h[5] += f; h[6] += g; h[7] += hh;
        }

        byte[] digest = new byte[32];
        for (int i = 0; i < 8; i++) {
            digest[i * 4] = (byte) (h[i] >>> 24);
            digest[i * 4 + 1] = (byte) (h[i] >>> 16);
            digest[i * 4 + 2] = (byte) (h[i] >>> 8);
            digest[i * 4 + 3] = (byte) h[i];
        }
        return digest;
    }

    private static String hex(byte[] b) {
        StringBuilder sb = new StringBuilder(b.length * 2);
        for (int i = 0; i < b.length; i++) {
            int v = b[i] & 0xff;
            sb.append(Character.forDigit(v >>> 4, 16));
            sb.append(Character.forDigit(v & 0xf, 16));
        }
        return sb.toString();
    }

    private static byte[] bytes(String s) {
        byte[] out = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            out[i] = (byte) s.charAt(i);
        }
        return out;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(hex(sha256(bytes("")))).append(',');
        sb.append(hex(sha256(bytes("abc")))).append(',');
        sb.append(hex(sha256(bytes("The quick brown fox jumps over the lazy dog")))).append(',');
        // 1 million-ish workload kept bounded for the 10s budget: a 1000-byte block.
        byte[] big = new byte[1000];
        for (int i = 0; i < big.length; i++) {
            big[i] = (byte) (i * 31 + 7);
        }
        sb.append(hex(sha256(big)));
        System.out.println(sb);
    }
}
