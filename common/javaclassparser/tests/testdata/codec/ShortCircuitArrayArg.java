package codec;

// Models commons-codec DoubleMetaphone.conditionC0: a method returning a short-circuit
// `(a && b) || <varargs-call>` whose right operand is a method call whose last argument is an
// inline String[] built by the javac `anewarray; dup; idx; ldc; aastore` idiom. The array-element
// store made the principled shared-leaf ternary builder decline (it treated the inline array build as
// statement dispatch), dropping into the legacy combiner which mis-wired the leading condition and
// emitted a missing-return method (Bug AK). Each path's result is folded into a deterministic
// fingerprint so a control-flow inversion (not just a recompile failure) is also caught.
public class ShortCircuitArrayArg {
    static boolean contains(String value, int start, int length, String... criteria) {
        boolean result = false;
        if (start >= 0 && start + length <= value.length()) {
            String target = value.substring(start, start + length);
            for (String c : criteria) {
                if (target.equals(c)) {
                    result = true;
                    break;
                }
            }
        }
        return result;
    }

    static char charAt(String value, int index) {
        if (index < 0 || index >= value.length()) {
            return '*';
        }
        return value.charAt(index);
    }

    // (c != 'I' && c != 'E') || contains(value, index - 2, 6, "BACHER", "MACHER")
    static boolean conditionC0(String value, int index) {
        char c = charAt(value, index + 2);
        return (c != 'I' && c != 'E') || contains(value, index - 2, 6, "BACHER", "MACHER");
    }

    // single-element array variant guards the 1-element idiom too
    static boolean conditionC1(String value, int index) {
        char c = charAt(value, index + 1);
        return (c == 'A') || contains(value, index, 3, "ZZA");
    }

    public static void main(String[] args) {
        String[] inputs = {"xxBACHERyy", "xxIACHERyy", "xxEMACHER", "MACHERzz", "ZZAxx", "short", ""};
        long fp = 1469598103934665603L;
        for (String s : inputs) {
            for (int idx = 0; idx < 9; idx++) {
                boolean r0 = conditionC0(s, idx);
                boolean r1 = conditionC1(s, idx);
                fp = (fp ^ (r0 ? 1L : 0L)) * 1099511628211L;
                fp = (fp ^ (r1 ? 1L : 0L)) * 1099511628211L;
            }
        }
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
