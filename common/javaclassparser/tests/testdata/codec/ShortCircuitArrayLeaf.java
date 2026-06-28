package codec;

// Models commons-codec Nysiis.transcodeRemaining: an if/else chain whose arms RETURN inline
// `new char[]{...}` arrays, guarded by short-circuit conditions of the shape `A && (B || C)` (and its
// De Morgan form `A && !(B && C)`). The multi-opcode array-construction leaf (`iconst; newarray; dup;
// idx; xload; castore`) made an upstream fold reorder the if-node's Next to [true,false] so JmpNode
// pinning captured trueIndex=0; the boolean-merge pass (mergeCondition) then rebuilt Next as
// [false,true] WITHOUT updating the stale TrueNode/FalseNode closures, silently inverting the merged
// condition's polarity (dropping the `!`) and swapping the then/else arms (Bug AL). The result
// compiled cleanly but truncated every encode (`Thompson -> TAN` instead of `TANPSA`). The same shape
// with `int` leaves did NOT reorder and stayed correct, which is why pure syntax/compile checks missed
// it -- only behavioral differential testing catches the inversion. Each path folds into a
// deterministic fingerprint so a control-flow inversion (not just a recompile failure) is caught.
public class ShortCircuitArrayLeaf {
    static boolean isVowel(char c) {
        return c == 'A' || c == 'E' || c == 'I' || c == 'O' || c == 'U';
    }

    // Faithful reduction of Nysiis.transcodeRemaining's tail: the `H` and `W` rules are the two
    // short-circuit guards that returned array leaves.
    static char[] transcode(char prev, char curr, char next) {
        if (curr == 'H' && (!isVowel(prev) || !isVowel(next))) {
            return new char[]{prev};
        }
        if (curr == 'W' && isVowel(prev)) {
            return new char[]{prev};
        }
        if (curr == 'S' && next == 'C') {
            return new char[]{'S', 'S'};
        }
        return new char[]{curr};
    }

    // Pure `A && (B || C)` short-circuit with array leaves, no negation, to exercise the OR-subtree
    // directly (different javac branch sense than transcode's De Morgan form).
    static char[] pick(char a, char b, char c) {
        if (a == 'H' && (b == 'X' || c == 'Y')) {
            return new char[]{a, b};
        }
        return new char[]{c};
    }

    public static void main(String[] args) {
        char[] alphabet = {'A', 'E', 'H', 'S', 'W', 'C', 'X', 'Y', 'M', 'Z'};
        long fp = 1469598103934665603L;
        for (char p : alphabet) {
            for (char c : alphabet) {
                for (char n : alphabet) {
                    char[] t = transcode(p, c, n);
                    char[] k = pick(p, c, n);
                    for (char x : t) {
                        fp = (fp ^ (long) x) * 1099511628211L;
                    }
                    fp = (fp ^ 0x7FL) * 1099511628211L;
                    for (char x : k) {
                        fp = (fp ^ (long) x) * 1099511628211L;
                    }
                    fp = (fp ^ 0x3FL) * 1099511628211L;
                }
            }
        }
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
