package codec;

// Bug W/Y probe (cross-scope declaration dominance): one JVM local slot is reused for THREE source
// variables on DISJOINT live ranges -- an int temp in an if-arm, a different int in the sibling
// else-arm, and a third int after the if -- the exact shape javac emits for jdk FloatingDecimal-style
// bit manipulation (and fastjson2 TypeUtils). The minted-id path merges all three onto one VariableId
// whose single declaration ends up in the if-arm; the else-arm and post-if uses are then out of scope
// ("cannot find symbol: var4"). A bare-declaration hoist to the block that dominates every use makes
// it recompile while preserving semantics. A clean byte-for-byte round-trip proves the placement.
public class SlotReuseDisjointRanges {

    // do-while with an if/else whose two arms and the trailing code each reuse the same slot for an
    // independent int. Mirrors the FloatingDecimal `if (hi>0){int bit=...} else {int nlz=...} ... int ntz=...`
    // pattern that fastjson2's TypeUtils re-uses verbatim.
    static int mix(int m) {
        int acc = 0;
        do {
            int hi = m >>> 23;
            int lo = m & 0x7fffff;
            if (hi > 0) {
                int bit = 0x800000;
                lo = lo | bit;
            } else {
                int nlz = Integer.numberOfLeadingZeros(lo | 1);
                int sh = nlz - 8;
                lo = lo << (sh & 31);
                hi = 1 - sh;
            }
            hi -= 127;
            int ntz = Integer.numberOfTrailingZeros(lo | 0x100);
            lo = lo >>> (ntz & 31);
            acc += hi + lo + ntz;
            m = m - 1;
        } while (m > 0);
        return acc;
    }

    // a second carrier: nested if/else where the same slot holds a count in one branch and a fresh
    // accumulator in the other, then a loop index after the branch -- three disjoint ranges again.
    static long blend(int a, int b) {
        long out = 1125899906842597L;
        if ((a & 1) == 0) {
            int p = a * 31 + b;
            out += p;
        } else {
            int q = b * 7 - a;
            out ^= ((long) q << 16);
        }
        int k = a ^ b;
        for (int i = 0; i < (k & 7) + 1; i++) {
            out = out * 1315423911L + i;
        }
        return out;
    }

    public static void main(String[] args) {
        long acc = 17L;
        for (int i = 1; i <= 64; i++) {
            acc = acc * 1000003L + mix(i + (i & 3));
            acc ^= blend(i, 64 - i);
        }
        System.out.println("SlotReuseDisjointRanges:" + Long.toHexString(acc));
    }
}
