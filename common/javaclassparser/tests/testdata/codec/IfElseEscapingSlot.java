package codec;

// Faithful model of the commons-codec Nysiis / Metaphone "transcode" shape that exposed Bug AJ.
//
// In each method below a primitive local (kind / h) is assigned in BOTH arms of an if/else (one JVM
// slot S, two live ranges), then a following loop READS that local while incrementing its OWN counter
// (slot S+1, whose live range OVERLAPS slot S's second range -> they are provably DIFFERENT slots),
// and a final return READS the if/else local once more. javac reuses slot S for kind across both
// ranges; the decompiler used to mint a separate id per arm, keep the post-if reads on the slot's
// original (un-minted) id, and -- because the arm mints never advanced the parent name counter --
// hand the loop counter the SAME varN as kind. The result compiled (all the same primitive type) but
// silently read the counter where the if/else local was meant, truncating every computation. Only
// differential execution (decompile -> recompile -> run -> compare fingerprint) catches it.
public class IfElseEscapingSlot {

    static int classify(String ref, int seed) {
        int acc = seed;
        int kind;
        if (ref == null) {
            kind = 1;
            acc += 7;
        } else {
            kind = 2 + (ref.charAt(0) & 7);
            acc += ref.length();
        }
        int i = 0;
        while (i < 4) {
            acc = (acc * 31) + kind;
            i++;
        }
        return (acc << 3) + kind;
    }

    // long variant: the if/else local `h` escapes into a do-while plus the return, and a sibling local
    // `acc` is introduced AFTER the if so it would alias the merged name under the old bug.
    static long mix(int n) {
        long h;
        if ((n & 1) == 0) {
            h = 0x9E3779B97F4A7C15L;
        } else {
            h = 0xC2B2AE3D27D4EB4FL;
        }
        int j = 0;
        long acc = 0L;
        do {
            acc = (acc ^ h) * 1099511628211L + j;
            j++;
        } while (j < 5);
        return acc + h;
    }

    public static void main(String[] args) {
        String[] inputs = {null, "A", "hello", "Zoo", "", "Thompson"};
        long fp = 1469598103934665603L;
        for (String s : inputs) {
            for (int seed = 0; seed < 6; seed++) {
                int r;
                try {
                    r = classify(s, seed);
                } catch (Exception e) {
                    r = -1;
                }
                fp = (fp ^ (long) r) * 1099511628211L;
            }
        }
        for (int n = 0; n < 20; n++) {
            fp = (fp ^ mix(n)) * 1099511628211L;
        }
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
