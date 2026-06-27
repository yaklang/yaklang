package codec;

/**
 * SwitchFallthroughAlgorithms - a self-hosted battery that exercises the switch-structuring paths the
 * decompiler historically got wrong, under the differential-execution oracle (compile -> run -> decompile
 * -> recompile -> run, fingerprints compared). Unlike GuavaMurmur3Algorithms (which expresses the
 * Murmur3 tail as a guard chain to dodge a former limitation), this battery uses REAL `switch`
 * statements so the round-trip pins the fix for:
 *
 *   - DESCENDING-value fall-through (case 3 -> case 2 -> case 1): javac lays the bodies out in source
 *     order at increasing offsets while the values descend, so emitting cases sorted by VALUE inverts
 *     the fall-through direction. murmurTail() is the canonical reproducer.
 *   - ASCENDING fall-through with and without break.
 *   - a tableswitch (dense, consecutive cases) and a lookupswitch (sparse cases).
 *   - a `default` in the MIDDLE of the case list.
 *   - an EMPTY `default: break;` whose target coincides with the switch's post-switch merge point
 *     (Bug K): every case breaks to it, so the merge must be emitted after the switch and the breaks
 *     preserved, rather than absorbing the post-switch code into `default:` and dropping the breaks.
 *   - NESTED switches that assign a shared local across sibling arms (the jasperreports slot-hoist /
 *     variable-naming determinism reproducer): the shared local must be declared once and named
 *     deterministically, and the values must survive the round-trip.
 *   - a String switch (hashCode + equals dispatch the compiler lowers to a lookupswitch + if chain).
 *
 * Any divergence - a reordered case, an inverted fall-through, a dropped break, a swapped local - changes
 * the fingerprint, so the oracle catches it precisely.
 */
public class SwitchFallthroughAlgorithms {

    // Descending fall-through: the Murmur3 tail mix written as javac actually emits it.
    static int murmurTail(byte[] data, int tail, int rem) {
        int k1 = 0;
        switch (rem) {
            case 3:
                k1 ^= (data[tail + 2] & 0xff) << 16;
            case 2:
                k1 ^= (data[tail + 1] & 0xff) << 8;
            case 1:
                k1 ^= (data[tail] & 0xff);
        }
        return k1;
    }

    // Ascending fall-through accumulation (no breaks): each higher case includes all lower work.
    static int ladder(int n) {
        int acc = 0;
        switch (n) {
            case 1:
                acc += 1;
            case 2:
                acc += 20;
            case 3:
                acc += 300;
            case 4:
                acc += 4000;
        }
        return acc;
    }

    // Dense consecutive cases -> tableswitch; default in the MIDDLE of the source order.
    static int dense(int n) {
        int r;
        switch (n) {
            case 0:
                r = 7;
                break;
            case 1:
                r = 11;
                break;
            default:
                r = -99;
                break;
            case 2:
                r = 13;
                break;
            case 3:
                r = 17;
                break;
        }
        return r;
    }

    // Sparse, far-apart cases -> lookupswitch with fall-through groups.
    static int sparse(int n) {
        int r = 0;
        switch (n) {
            case 1:
            case 100:
                r = 1;
                break;
            case 1000:
                r = 2;
            case 10000:
                r += 40;
                break;
            default:
                r = -1;
        }
        return r;
    }

    // Empty `default: break;` whose target IS the switch's post-switch merge point (Bug K). Every
    // matched case `break`s to the same point the empty default falls through to, so the merge and the
    // default target coincide. The post-switch code (`return out*10+rem`) must execute AFTER the
    // switch; the historical bug absorbed it into `default:` and dropped every case break (all cases
    // fell through), miscomputing the result. Dense values -> tableswitch with gap/default to 64.
    static int emptyDefaultTable(int rem, int base) {
        int out = base;
        switch (rem) {
            case 2: out += 1; break;
            case 4: out += 2; break;
            case 5: out += 3; break;
            case 7: out += 4; break;
            default: break;
        }
        return out * 10 + rem;
    }

    // Same empty-default-is-merge shape but with genuinely sparse values -> lookupswitch (the exact
    // Base32 decode reproducer family).
    static int emptyDefaultLookup(int sel, int base) {
        int out = base;
        switch (sel) {
            case 5: out += 1; break;
            case 500: out += 2; break;
            case 50000: out += 3; break;
            case 5000000: out += 4; break;
            default: break;
        }
        return out * 7 + (sel & 0xff);
    }

    // Nested switches assigning one shared local across sibling arms: the slot-hoist / naming
    // determinism reproducer (mirrors jasperreports getTextAlignHolder shape).
    static int nestedShared(int a, int b) {
        int shared;
        switch (a) {
            case 0:
                switch (b) {
                    case 0:
                        shared = 2;
                        break;
                    case 1:
                        shared = 3;
                        break;
                    default:
                        shared = 5;
                        break;
                }
                break;
            case 1:
                switch (b) {
                    case 0:
                        shared = 7;
                        break;
                    default:
                        shared = 11;
                        break;
                }
                break;
            default:
                shared = 13;
                break;
        }
        return shared * 31 + a - b;
    }

    // String switch: javac lowers to hashCode lookupswitch + equals if-chain.
    static int classify(String s) {
        switch (s) {
            case "alpha":
                return 1;
            case "beta":
                return 2;
            case "gamma":
                return 3;
            default:
                return 0;
        }
    }

    static int murmur32(String s) {
        byte[] data = new byte[s.length()];
        for (int i = 0; i < s.length(); i++) {
            data[i] = (byte) s.charAt(i);
        }
        int h1 = 0;
        int nblocks = data.length / 4;
        for (int i = 0; i < nblocks; i++) {
            int base = i * 4;
            int k1 = (data[base] & 0xff)
                    | ((data[base + 1] & 0xff) << 8)
                    | ((data[base + 2] & 0xff) << 16)
                    | ((data[base + 3] & 0xff) << 24);
            k1 *= 0xcc9e2d51;
            k1 = (k1 << 15) | (k1 >>> 17);
            k1 *= 0x1b873593;
            h1 ^= k1;
            h1 = (h1 << 13) | (h1 >>> 19);
            h1 = h1 * 5 + 0xe6546b64;
        }
        int tail = nblocks * 4;
        int rem = data.length & 3;
        int k1 = murmurTail(data, tail, rem);
        if (rem > 0) {
            k1 *= 0xcc9e2d51;
            k1 = (k1 << 15) | (k1 >>> 17);
            k1 *= 0x1b873593;
            h1 ^= k1;
        }
        h1 ^= data.length;
        h1 ^= h1 >>> 16;
        h1 *= 0x85ebca6b;
        h1 ^= h1 >>> 13;
        h1 *= 0xc2b2ae35;
        h1 ^= h1 >>> 16;
        return h1;
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

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] inputs = {"", "a", "ab", "abc", "abcd", "abcde", "switch-coverage"};
        for (int i = 0; i < inputs.length; i++) {
            sb.append(hex8(murmur32(inputs[i]))).append(';');
        }
        sb.append('/');

        for (int n = 0; n <= 5; n++) {
            sb.append(ladder(n)).append(',');
        }
        sb.append('/');

        for (int n = -1; n <= 4; n++) {
            sb.append(dense(n)).append(',');
        }
        sb.append('/');

        int[] sparseIn = {0, 1, 100, 1000, 10000, 99};
        for (int i = 0; i < sparseIn.length; i++) {
            sb.append(sparse(sparseIn[i])).append(',');
        }
        sb.append('/');

        for (int a = 0; a <= 2; a++) {
            for (int b = 0; b <= 2; b++) {
                sb.append(nestedShared(a, b)).append(',');
            }
        }
        sb.append('/');

        String[] words = {"alpha", "beta", "gamma", "delta"};
        for (int i = 0; i < words.length; i++) {
            sb.append(classify(words[i]));
        }
        sb.append('/');

        for (int rem = 0; rem <= 8; rem++) {
            sb.append(emptyDefaultTable(rem, 100)).append(',');
        }
        sb.append('/');

        int[] selIn = {0, 5, 500, 50000, 5000000, 7};
        for (int i = 0; i < selIn.length; i++) {
            sb.append(emptyDefaultLookup(selIn[i], 1000)).append(',');
        }

        System.out.println(sb);
    }
}
