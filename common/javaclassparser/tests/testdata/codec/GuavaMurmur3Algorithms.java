package codec;

/**
 * GuavaMurmur3Algorithms - a self-hosted port of Guava's Hashing.murmur3_32 / murmur3_128 finalizer
 * math (com.google.common.hash.Murmur3_32HashFunction). No Guava dependency: pure static methods over
 * byte[]/int/long so the single-class decompile round-trip (decompile -> recompile -> run, fingerprints
 * compared) holds. The algorithm is a dense mix of the exact opcodes a decompiler most often corrupts:
 *
 *   - int multiply/xor with NEGATIVE 0x-prefixed constants (c1=0xcc9e2d51, c2=0x1b873593) exercising
 *     iconst/ldc + imul + ixor and two's-complement wraparound,
 *   - rotate-left built from `(x << r) | (x >>> (32 - r))` exercising ishl + iushr (UNSIGNED shift) + ior,
 *   - the canonical `switch (len & 3)` tail with DESCENDING-value fall-through (case 3 -> 2 -> 1, no
 *     breaks) and a post-switch finalizer, pinning correct fall-through direction + merge folding,
 *   - the fmix finalizer's `h ^= h >>> 16` chains exercising iushr again, and
 *   - little-endian block assembly from signed bytes via `b & 0xff` (iand) and `<< 8/16/24`.
 *
 * A divergence in any of these (a logical vs arithmetic shift, a dropped fall-through, a sign-extended
 * byte, a swapped operand) changes the digest, so the differential round-trip pins them precisely.
 */
public class GuavaMurmur3Algorithms {

    private static final int C1 = 0xcc9e2d51;
    private static final int C2 = 0x1b873593;

    // 128-bit (x64) mixing constants; long-typed so the k1/k2 tail accumulators stay long.
    private static final long C1_128 = 0x87c37b91114253d5L;
    private static final long C2_128 = 0x4cf5ad432745937fL;

    private static int rotl32(int x, int r) {
        return (x << r) | (x >>> (32 - r));
    }

    private static int mixK1(int k1) {
        k1 = k1 * C1;
        k1 = rotl32(k1, 15);
        k1 = k1 * C2;
        return k1;
    }

    private static int mixH1(int h1, int k1) {
        h1 = h1 ^ k1;
        h1 = rotl32(h1, 13);
        h1 = h1 * 5 + 0xe6546b64;
        return h1;
    }

    private static int fmix32(int h, int length) {
        h = h ^ length;
        h = h ^ (h >>> 16);
        h = h * 0x85ebca6b;
        h = h ^ (h >>> 13);
        h = h * 0xc2b2ae35;
        h = h ^ (h >>> 16);
        return h;
    }

    static int murmur3_32(byte[] data, int seed) {
        int h1 = seed;
        int len = data.length;
        int nblocks = len / 4;

        for (int i = 0; i < nblocks; i++) {
            int base = i * 4;
            int k1 = (data[base] & 0xff)
                    | ((data[base + 1] & 0xff) << 8)
                    | ((data[base + 2] & 0xff) << 16)
                    | ((data[base + 3] & 0xff) << 24);
            h1 = mixH1(h1, mixK1(k1));
        }

        int tail = nblocks * 4;
        int k1 = 0;
        // The canonical Murmur3 tail: a `switch (len & 3)` with DESCENDING-value fall-through
        // (case 3 -> case 2 -> case 1, no breaks), with the post-switch finalizer running after the
        // case-1 fall-out as well as for len&3==0. This is the exact shape that previously forced the
        // guard-chain workaround; the decompiler now preserves descending fall-through and folds the
        // post-switch code correctly, so the natural switch is restored as live regression coverage.
        switch (len & 3) {
            case 3:
                k1 = k1 ^ ((data[tail + 2] & 0xff) << 16);
            case 2:
                k1 = k1 ^ ((data[tail + 1] & 0xff) << 8);
            case 1:
                k1 = k1 ^ (data[tail] & 0xff);
                k1 = mixK1(k1);
                h1 = h1 ^ k1;
        }

        return fmix32(h1, len);
    }

    static int murmur3_32(String s, int seed) {
        byte[] data = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            data[i] = (byte) s.charAt(i);
        }
        return murmur3_32(data, seed);
    }

    // A 64-bit avalanche finalizer (Murmur3_128 fmix64) to also cover long shifts/multiplies
    // (lshl/lushr/lmul/lxor) with negative long constants.
    static long fmix64(long k) {
        k = k ^ (k >>> 33);
        k = k * 0xff51afd7ed558ccdL;
        k = k ^ (k >>> 33);
        k = k * 0xc4ceb9fe1a85ec53L;
        k = k ^ (k >>> 33);
        return k;
    }

    private static long rotl64(long x, int r) {
        return (x << r) | (x >>> (64 - r));
    }

    private static long getLongLE(byte[] data, int index) {
        long r = 0;
        for (int i = 0; i < 8; i++) {
            r = r | (((long) (data[index + i] & 0xff)) << (8 * i));
        }
        return r;
    }

    // Murmur3 128-bit (x64). The body loop reads two LONG lanes (k1, k2) per 16-byte block via an INT
    // index, and the tail is a 16-arm `switch (len & 15)` with descending fall-through that mixes the
    // remaining bytes (indexed by an INT offset `tail`) into the two LONG accumulators. This is the
    // exact long-accumulator / int-subscript co-residency that previously merged into one cross-typed
    // variable (a long used as an array subscript -> "possible lossy conversion from long to int").
    // The differential round-trip over every tail length (len & 15 in 0..15) pins that the int offset
    // and the long lanes stay distinct, correctly-typed variables.
    static long murmur3_128(byte[] data, int seed) {
        int len = data.length;
        int nblocks = len / 16;
        long h1 = seed & 0xffffffffL;
        long h2 = seed & 0xffffffffL;

        for (int i = 0; i < nblocks; i++) {
            long k1 = getLongLE(data, i * 16);
            long k2 = getLongLE(data, i * 16 + 8);

            k1 = k1 * C1_128;
            k1 = rotl64(k1, 31);
            k1 = k1 * C2_128;
            h1 = h1 ^ k1;
            h1 = rotl64(h1, 27);
            h1 = h1 + h2;
            h1 = h1 * 5 + 0x52dce729;

            k2 = k2 * C2_128;
            k2 = rotl64(k2, 33);
            k2 = k2 * C1_128;
            h2 = h2 ^ k2;
            h2 = rotl64(h2, 31);
            h2 = h2 + h1;
            h2 = h2 * 5 + 0x38495ab5;
        }

        long k1 = 0;
        long k2 = 0;
        int tail = nblocks * 16;
        switch (len & 15) {
            case 15:
                k2 = k2 ^ (((long) (data[tail + 14] & 0xff)) << 48);
            case 14:
                k2 = k2 ^ (((long) (data[tail + 13] & 0xff)) << 40);
            case 13:
                k2 = k2 ^ (((long) (data[tail + 12] & 0xff)) << 32);
            case 12:
                k2 = k2 ^ (((long) (data[tail + 11] & 0xff)) << 24);
            case 11:
                k2 = k2 ^ (((long) (data[tail + 10] & 0xff)) << 16);
            case 10:
                k2 = k2 ^ (((long) (data[tail + 9] & 0xff)) << 8);
            case 9:
                k2 = k2 ^ ((long) (data[tail + 8] & 0xff));
                k2 = k2 * C2_128;
                k2 = rotl64(k2, 33);
                k2 = k2 * C1_128;
                h2 = h2 ^ k2;
            case 8:
                k1 = k1 ^ (((long) (data[tail + 7] & 0xff)) << 56);
            case 7:
                k1 = k1 ^ (((long) (data[tail + 6] & 0xff)) << 48);
            case 6:
                k1 = k1 ^ (((long) (data[tail + 5] & 0xff)) << 40);
            case 5:
                k1 = k1 ^ (((long) (data[tail + 4] & 0xff)) << 32);
            case 4:
                k1 = k1 ^ (((long) (data[tail + 3] & 0xff)) << 24);
            case 3:
                k1 = k1 ^ (((long) (data[tail + 2] & 0xff)) << 16);
            case 2:
                k1 = k1 ^ (((long) (data[tail + 1] & 0xff)) << 8);
            case 1:
                k1 = k1 ^ ((long) (data[tail] & 0xff));
                k1 = k1 * C1_128;
                k1 = rotl64(k1, 31);
                k1 = k1 * C2_128;
                h1 = h1 ^ k1;
        }

        h1 = h1 ^ len;
        h2 = h2 ^ len;
        h1 = h1 + h2;
        h2 = h2 + h1;
        h1 = fmix64(h1);
        h2 = fmix64(h2);
        h1 = h1 + h2;
        h2 = h2 + h1;
        return h1 ^ h2;
    }

    static long murmur3_128(String s, int seed) {
        byte[] data = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            data[i] = (byte) s.charAt(i);
        }
        return murmur3_128(data, seed);
    }

    private static String hex8(int v) {
        String h = Integer.toHexString(v);
        StringBuilder sb = new StringBuilder();
        for (int i = h.length(); i < 8; i++) {
            sb.append('0');
        }
        sb.append(h);
        return sb.toString();
    }

    private static String hex16(long v) {
        StringBuilder sb = new StringBuilder();
        for (int shift = 60; shift >= 0; shift = shift - 4) {
            int nibble = (int) ((v >>> shift) & 0xf);
            sb.append("0123456789abcdef".charAt(nibble));
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        // Inputs of every tail length (len & 3 in {0,1,2,3}) to hit all switch fall-through arms.
        String[] inputs = {"", "a", "ab", "abc", "abcd", "abcde", "hello, world", "murmur3-coverage"};
        for (int i = 0; i < inputs.length; i++) {
            sb.append(hex8(murmur3_32(inputs[i], 0)));
            sb.append('/');
            sb.append(hex8(murmur3_32(inputs[i], 0x9747b28c)));
            sb.append(';');
        }
        sb.append(',');

        long[] seeds = {0L, 1L, -1L, 0x0123456789abcdefL, 0xdeadbeefcafebabeL};
        for (int i = 0; i < seeds.length; i++) {
            sb.append(hex16(fmix64(seeds[i]))).append(';');
        }
        sb.append(',');

        // Cover every tail length (len & 15 in 0..15) so all 16 switch fall-through arms of the
        // long-accumulator tail run, plus inputs longer than one 16-byte block for the body loop.
        String[] m128Inputs = {
                "", "a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg",
                "abcdefgh", "abcdefghi", "0123456789", "0123456789a", "0123456789ab",
                "0123456789abc", "0123456789abcd", "0123456789abcde", "0123456789abcdef",
                "the quick brown fox jumps over the lazy dog"
        };
        for (int i = 0; i < m128Inputs.length; i++) {
            sb.append(hex16(murmur3_128(m128Inputs[i], 0)));
            sb.append('/');
            sb.append(hex16(murmur3_128(m128Inputs[i], 0x9747b28c)));
            sb.append(';');
        }

        System.out.println(sb);
    }
}
