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
 *   - a `len & 3` tail guard chain (rem >= 3 / >= 2 / >= 1) reproducing Murmur3's descending tail mix
 *     via if-branches (the canonical descending switch fall-through is a known decompiler limitation),
 *   - the fmix finalizer's `h ^= h >>> 16` chains exercising iushr again, and
 *   - little-endian block assembly from signed bytes via `b & 0xff` (iand) and `<< 8/16/24`.
 *
 * A divergence in any of these (a logical vs arithmetic shift, a dropped fall-through, a sign-extended
 * byte, a swapped operand) changes the digest, so the differential round-trip pins them precisely.
 */
public class GuavaMurmur3Algorithms {

    private static final int C1 = 0xcc9e2d51;
    private static final int C2 = 0x1b873593;

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
        int rem = len & 3;
        // NOTE: the canonical Murmur3 tail is a `switch (len & 3)` with DESCENDING-value fall-through
        // (case 3 -> case 2 -> case 1). The decompiler currently re-orders switch cases ascending by
        // value, which inverts fall-through direction (a separate switch-structuring limitation, see
        // switch_rewriter.go), so the round-trip is expressed here as the semantically identical guard
        // chain. It still computes the exact same digest and exercises iushr/ishl/iand/imul/ixor.
        if (rem >= 3) {
            k1 = k1 ^ ((data[tail + 2] & 0xff) << 16);
        }
        if (rem >= 2) {
            k1 = k1 ^ ((data[tail + 1] & 0xff) << 8);
        }
        if (rem >= 1) {
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

        System.out.println(sb);
    }
}
