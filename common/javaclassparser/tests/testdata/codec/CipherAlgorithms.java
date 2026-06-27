package codec;

/**
 * CipherAlgorithms - from-scratch TEA / XTEA block ciphers and the RC4 stream cipher, each run as an
 * encrypt-then-decrypt self-check (the recovered plaintext must equal the input) plus a ciphertext
 * fingerprint. Complements the hash batteries by stressing wrap-around int arithmetic (the TEA delta
 * schedule), fixed 32/64-round loops, signed/unsigned shift mixing, and byte[]-with-modulo state
 * permutation (RC4 KSA/PRGA swaps). A miscompiled shift, an int/long promotion, or a wrong array
 * index would make decrypt fail to recover the plaintext and break the round-trip oracle.
 *
 * Single public top-level class, static only, deterministic.
 */
public class CipherAlgorithms {

    private static final int DELTA = 0x9e3779b9;

    // ---- TEA ----
    private static void teaEncrypt(int[] v, int[] k) {
        int v0 = v[0], v1 = v[1], sum = 0;
        for (int i = 0; i < 32; i++) {
            sum += DELTA;
            v0 += ((v1 << 4) + k[0]) ^ (v1 + sum) ^ ((v1 >>> 5) + k[1]);
            v1 += ((v0 << 4) + k[2]) ^ (v0 + sum) ^ ((v0 >>> 5) + k[3]);
        }
        v[0] = v0;
        v[1] = v1;
    }

    private static void teaDecrypt(int[] v, int[] k) {
        int v0 = v[0], v1 = v[1], sum = DELTA * 32;
        for (int i = 0; i < 32; i++) {
            v1 -= ((v0 << 4) + k[2]) ^ (v0 + sum) ^ ((v0 >>> 5) + k[3]);
            v0 -= ((v1 << 4) + k[0]) ^ (v1 + sum) ^ ((v1 >>> 5) + k[1]);
            sum -= DELTA;
        }
        v[0] = v0;
        v[1] = v1;
    }

    // ---- XTEA ----
    private static void xteaEncrypt(int rounds, int[] v, int[] k) {
        int v0 = v[0], v1 = v[1], sum = 0;
        for (int i = 0; i < rounds; i++) {
            v0 += (((v1 << 4) ^ (v1 >>> 5)) + v1) ^ (sum + k[sum & 3]);
            sum += DELTA;
            v1 += (((v0 << 4) ^ (v0 >>> 5)) + v0) ^ (sum + k[(sum >>> 11) & 3]);
        }
        v[0] = v0;
        v[1] = v1;
    }

    private static void xteaDecrypt(int rounds, int[] v, int[] k) {
        int v0 = v[0], v1 = v[1], sum = DELTA * rounds;
        for (int i = 0; i < rounds; i++) {
            v1 -= (((v0 << 4) ^ (v0 >>> 5)) + v0) ^ (sum + k[(sum >>> 11) & 3]);
            sum -= DELTA;
            v0 -= (((v1 << 4) ^ (v1 >>> 5)) + v1) ^ (sum + k[sum & 3]);
        }
        v[0] = v0;
        v[1] = v1;
    }

    // ---- RC4 ----
    private static byte[] rc4(byte[] key, byte[] data) {
        int[] s = new int[256];
        for (int i = 0; i < 256; i++) {
            s[i] = i;
        }
        int j = 0;
        for (int i = 0; i < 256; i++) {
            j = (j + s[i] + (key[i % key.length] & 0xff)) & 0xff;
            int tmp = s[i];
            s[i] = s[j];
            s[j] = tmp;
        }
        byte[] out = new byte[data.length];
        int a = 0, b = 0;
        for (int n = 0; n < data.length; n++) {
            a = (a + 1) & 0xff;
            b = (b + s[a]) & 0xff;
            int tmp = s[a];
            s[a] = s[b];
            s[b] = tmp;
            int ks = s[(s[a] + s[b]) & 0xff];
            out[n] = (byte) (data[n] ^ ks);
        }
        return out;
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

    private static long teaSelfCheck() {
        int[] key = {0x12345678, 0x9abcdef0, 0x0fedcba9, 0x87654321};
        long acc = 0;
        for (int t = 0; t < 50; t++) {
            int[] v = {0xdeadbeef ^ (t * 2654435761L != 0 ? t * 7 : 0), 0xcafebabe + t * 13};
            int p0 = v[0], p1 = v[1];
            teaEncrypt(v, key);
            acc = acc * 1000003 + (v[0] & 0xffffffffL);
            acc = acc * 1000003 + (v[1] & 0xffffffffL);
            teaDecrypt(v, key);
            if (v[0] != p0 || v[1] != p1) {
                return -1L; // decrypt must recover plaintext
            }
        }
        return acc;
    }

    private static long xteaSelfCheck() {
        int[] key = {0x0badf00d, 0x13371337, 0x55aa55aa, 0xa5a5a5a5};
        long acc = 0;
        for (int t = 0; t < 50; t++) {
            int[] v = {0x01234567 + t * 17, 0x89abcdef ^ (t * 31)};
            int p0 = v[0], p1 = v[1];
            xteaEncrypt(64, v, key);
            acc ^= ((long) v[0] << 32) ^ (v[1] & 0xffffffffL);
            xteaDecrypt(64, v, key);
            if (v[0] != p0 || v[1] != p1) {
                return -1L;
            }
        }
        return acc;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(Long.toHexString(teaSelfCheck())).append(',');
        sb.append(Long.toHexString(xteaSelfCheck())).append(',');

        byte[] key = {(byte) 'S', (byte) 'e', (byte) 'c', (byte) 'r', (byte) 'e', (byte) 't'};
        byte[] plain = new byte[64];
        for (int i = 0; i < plain.length; i++) {
            plain[i] = (byte) (i * 5 + 1);
        }
        byte[] cipher = rc4(key, plain);
        byte[] back = rc4(key, cipher);
        boolean ok = true;
        for (int i = 0; i < plain.length; i++) {
            if (plain[i] != back[i]) {
                ok = false;
                break;
            }
        }
        sb.append(hex(cipher)).append(',').append(ok ? "ok" : "bad");
        System.out.println(sb);
    }
}
