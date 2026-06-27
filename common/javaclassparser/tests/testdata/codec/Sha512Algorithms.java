package codec;

/**
 * Sha512Algorithms - a from-scratch SHA-512 (FIPS 180-4) implementation. Where Sha256Algorithms
 * stresses the 32-bit int opcode family, this battery is dominated by the 64-bit LONG family:
 * long rotate-right `(x >>> n) | (x << (64-n))`, LUSHR/LSHL/LADD/LAND/LXOR, long[] message schedule,
 * 80-round compression, and big-endian long<->byte packing. Any miscompiled long shift width,
 * long/int promotion, or dropped long store surfaces as a digest mismatch via the round-trip oracle.
 *
 * Sanity (verified by batterySanity): SHA-512("") = cf83e135...da3e.
 * Single public top-level class, static only, deterministic.
 */
public class Sha512Algorithms {

    private static final long[] K = {
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

    private static long rotr(long x, int n) {
        return (x >>> n) | (x << (64 - n));
    }

    public static byte[] sha512(byte[] message) {
        long[] h = {
            0x6a09e667f3bcc908L, 0xbb67ae8584caa73bL, 0x3c6ef372fe94f82bL, 0xa54ff53a5f1d36f1L,
            0x510e527fade682d1L, 0x9b05688c2b3e6c1fL, 0x1f83d9abfb41bd6bL, 0x5be0cd19137e2179L
        };

        int origLen = message.length;
        long bitLen = (long) origLen * 8L;
        int rem = origLen % 128;
        int padLen = (rem < 112) ? (112 - rem) : (240 - rem);
        byte[] padded = new byte[origLen + padLen + 16];
        for (int i = 0; i < origLen; i++) {
            padded[i] = message[i];
        }
        padded[origLen] = (byte) 0x80;
        // 128-bit length: high 64 bits are zero for our inputs; write low 64 bits big-endian.
        for (int i = 0; i < 8; i++) {
            padded[padded.length - 1 - i] = (byte) (bitLen >>> (8 * i));
        }

        long[] w = new long[80];
        for (int chunk = 0; chunk < padded.length; chunk += 128) {
            for (int i = 0; i < 16; i++) {
                int j = chunk + i * 8;
                long word = 0L;
                for (int k = 0; k < 8; k++) {
                    word = (word << 8) | (padded[j + k] & 0xffL);
                }
                w[i] = word;
            }
            for (int i = 16; i < 80; i++) {
                long s0 = rotr(w[i - 15], 1) ^ rotr(w[i - 15], 8) ^ (w[i - 15] >>> 7);
                long s1 = rotr(w[i - 2], 19) ^ rotr(w[i - 2], 61) ^ (w[i - 2] >>> 6);
                w[i] = w[i - 16] + s0 + w[i - 7] + s1;
            }

            long a = h[0], b = h[1], c = h[2], d = h[3];
            long e = h[4], f = h[5], g = h[6], hh = h[7];

            for (int i = 0; i < 80; i++) {
                long bigS1 = rotr(e, 14) ^ rotr(e, 18) ^ rotr(e, 41);
                long ch = (e & f) ^ ((~e) & g);
                long temp1 = hh + bigS1 + ch + K[i] + w[i];
                long bigS0 = rotr(a, 28) ^ rotr(a, 34) ^ rotr(a, 39);
                long maj = (a & b) ^ (a & c) ^ (b & c);
                long temp2 = bigS0 + maj;
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

        byte[] digest = new byte[64];
        for (int i = 0; i < 8; i++) {
            for (int k = 0; k < 8; k++) {
                digest[i * 8 + k] = (byte) (h[i] >>> (56 - 8 * k));
            }
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
        sb.append(hex(sha512(bytes("")))).append(',');
        sb.append(hex(sha512(bytes("abc")))).append(',');
        sb.append(hex(sha512(bytes("The quick brown fox jumps over the lazy dog")))).append(',');
        byte[] big = new byte[1000];
        for (int i = 0; i < big.length; i++) {
            big[i] = (byte) (i * 31 + 7);
        }
        sb.append(hex(sha512(big)));
        System.out.println(sb);
    }
}
