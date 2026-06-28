package codec;

/**
 * SwitchEmptyLeadingCaseAlgorithms - locks the commons-codec BaseNCodec.encode/Base32/Base64 EOF
 * switch shape that regressed with "IllegalStateException: Impossible modulus N".
 *
 * The shape: a dense tableswitch whose LEADING case (case 0) is EMPTY (a no-op that only breaks to
 * the post-switch code), real work in cases 1/2 each guarded by an inner if, a `default: throw`, and
 * a tail AFTER the switch that every non-throwing case must reach. After goto-folding, the empty
 * leading case's start node IS the post-switch merge point. A broken structurizer either
 *   - shifted the case<->body mapping (put the throw on a real case, absorbed the tail into default),
 *     making every non-block-aligned input fall through to the throw, or
 *   - swapped the inner if's then/else (padding applied to the wrong branch).
 * Both are silent: they pass syntax validation but change behaviour, so only execution catches them.
 */
public class SwitchEmptyLeadingCaseAlgorithms {
    static final int[] TABLE = {7, 11, 13, 17, 19, 23, 29, 31};

    static int encodeBlock(int modulus, int work, boolean standard) {
        int pos = 0;
        switch (modulus) {
            case 0:
                break;
            case 1:
                pos += TABLE[(work >> 2) & 7];
                pos += TABLE[(work << 4) & 7];
                if (standard) {
                    pos += 100;
                    pos += 100;
                }
                break;
            case 2:
                pos += TABLE[(work >> 10) & 7];
                pos += TABLE[(work >> 4) & 7];
                pos += TABLE[(work << 2) & 7];
                if (standard) {
                    pos += 100;
                }
                break;
            default:
                throw new IllegalStateException("Impossible modulus " + modulus);
        }
        // Tail after the switch: must be reached by case 0/1/2 alike (this is the merge point that
        // the empty leading case collapses onto).
        pos += 1;
        if (pos > 0) {
            pos += 7;
        }
        return pos;
    }

    public static void main(String[] z) {
        StringBuilder sb = new StringBuilder();
        for (int m = 0; m <= 2; m++) {
            sb.append(encodeBlock(m, 0x12345, true)).append(",");
            sb.append(encodeBlock(m, 0x12345, false)).append(",");
        }
        try {
            encodeBlock(3, 0, true);
            sb.append("nothrow");
        } catch (IllegalStateException e) {
            sb.append("threw:").append(e.getMessage());
        }
        System.out.println(sb.toString());
    }
}
