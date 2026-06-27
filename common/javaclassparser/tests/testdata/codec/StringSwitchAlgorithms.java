package codec;

/**
 * StringSwitchAlgorithms - exercises javac's String-switch lowering: a first switch on
 * String.hashCode() (lookupswitch/tableswitch) followed by String.equals() guards and a second
 * switch on a synthetic index, plus default/fall-through arms. This is a structurally distinct,
 * frequently mis-decompiled area (the two-stage switch + equals chain) verified end-to-end by the
 * round-trip oracle.
 *
 * Also covers: char-level switch, nested switch, switch fall-through, and a small tokenizer state
 * machine driving the dispatch so control flow is non-trivial.
 *
 * Single public top-level class, static only, deterministic.
 */
public class StringSwitchAlgorithms {

    private static int classifyWord(String s) {
        switch (s) {
            case "alpha":
                return 1;
            case "beta":
                return 2;
            case "gamma":
                return 3;
            case "delta":
                return 4;
            case "epsilon":
                return 5;
            case "zeta":
            case "eta":
                return 6; // shared arm
            default:
                return 0;
        }
    }

    // Regression guard for "Bug S" (String-switch temp slot reused for a later int). javac compiles
    // the String-switch into a hashCode()/equals() + synthetic-index switch whose temp String slot is
    // then reused by the int `base`, while `extra` reuses a second slot. The decompiler must (a) bind
    // the equals() receiver to the live String version of the slot and (b) keep `base` and `extra` as
    // distinct, correctly-scoped variables when their generated names collide. This single combined
    // method exercises both the String-switch base and the char-switch extra in ONE frame so the
    // cross-slot reuse is present, and the round-trip oracle verifies the result end-to-end.
    private static int opcodeOf(String mnem) {
        int base;
        switch (mnem) {
            case "add":
                base = 100;
                break;
            case "sub":
                base = 200;
                break;
            case "mul":
                base = 300;
                break;
            case "div":
                base = 400;
                break;
            default:
                base = 900;
                break;
        }
        int extra = 0;
        switch (mnem.charAt(0)) {
            case 'a':
                extra += 1;
                // fall through
            case 'b':
                extra += 2;
                break;
            case 'd':
                extra += 8;
                break;
            default:
                extra += 16;
                break;
        }
        return base + extra;
    }

    private static long tokenize(String src) {
        long acc = 0;
        int i = 0;
        int n = src.length();
        while (i < n) {
            char c = src.charAt(i);
            if (c == ' ') {
                i++;
                continue;
            }
            int start = i;
            while (i < n && src.charAt(i) != ' ') {
                i++;
            }
            String tok = src.substring(start, i);
            int cls = classifyWord(tok);
            int op = opcodeOf(tok);
            acc = acc * 1000003 + cls;
            acc = acc * 1000003 + op;
        }
        return acc;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        String[] words = {"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "add", "sub", "mul", "div", "xor"};
        for (int i = 0; i < words.length; i++) {
            sb.append(classifyWord(words[i])).append(':').append(opcodeOf(words[i])).append(',');
        }
        sb.append(Long.toHexString(tokenize("add alpha sub beta mul gamma div delta xor epsilon zeta eta")));
        System.out.println(sb);
    }
}
