package codec;

/**
 * TryCatchLocalAlgorithms - a battery for the "cross-arm local" exception-handling shape: a local
 * assigned in BOTH the try arm and a catch arm (and/or a finally), then read AFTER the try.
 *
 *     int r;
 *     try { r = parse(s); } catch (NumberFormatException e) { r = -1; }
 *     return r;
 *
 * rewriteVar descends into the try and each catch as disjoint sub-scopes, so the try arm's in-scope
 * variable rename never reaches the catch arm: each arm used to re-mint its OWN id for the shared
 * slot while the post-try read kept the slot's original id, yielding three disagreeing names that
 * javac rejects ("cannot find symbol"). The if/else lowering has the same structure but happened to
 * survive by a naming coincidence that try/catch breaks (the catch exception parameter consumes the
 * intervening name). The fix pre-binds a slot assigned in two or more arms to a single id before
 * descending, so the arms reuse it and the single `T x;` declaration is hoisted ahead of the try.
 *
 * Covered shapes: basic cross-arm (int / String / reference), cross-arm with finally (javac inlines
 * the finally into every path), multi-catch cross-arm, cross-arm inside a loop with the accumulator
 * read each iteration, nested try with an outer cross-arm local, and an assign-only-in-try read after
 * (the read legitimately sinks into the try body). Pure static methods, single public top-level
 * class, deterministic fingerprint in main().
 *
 * Opcode intent: athrow / exception tables / multi-catch type unions, astore of the caught exception,
 * local store-load merged across try/catch/finally arms, StringBuilder concatenation in arms.
 */
public class TryCatchLocalAlgorithms {

    static int parseStrict(String s) {
        return Integer.parseInt(s);
    }

    static int crossArmInt(String s) {
        int r;
        try {
            r = parseStrict(s);
        } catch (NumberFormatException e) {
            r = -1;
        }
        return r + 1;
    }

    static String crossArmStr(String s) {
        String out;
        try {
            out = "v" + parseStrict(s);
        } catch (NumberFormatException e) {
            out = "err";
        }
        return out + "!";
    }

    static int crossArmFinally(String s) {
        int r = 0;
        try {
            r = parseStrict(s);
        } catch (NumberFormatException e) {
            r = -1;
        } finally {
            r = r + 5;
        }
        return r;
    }

    static int multiCatchCrossArm(Object o) {
        int r;
        try {
            if (o == null) {
                throw new IllegalStateException("null");
            }
            r = o.toString().length();
        } catch (IllegalStateException | NullPointerException e) {
            r = -2;
        }
        return r;
    }

    static int loopCrossArm(String[] items) {
        int sum = 0;
        for (int i = 0; i < items.length; i++) {
            int v;
            try {
                v = parseStrict(items[i]);
            } catch (NumberFormatException e) {
                v = 0;
            }
            sum = sum + v;
        }
        return sum;
    }

    static int nestedTryCrossArm(String a, String b) {
        int total;
        try {
            int x;
            try {
                x = parseStrict(a);
            } catch (NumberFormatException e) {
                x = 10;
            }
            total = x + parseStrict(b);
        } catch (NumberFormatException e) {
            total = -7;
        }
        return total;
    }

    static int tryOnlyReadAfter(String s) {
        int r;
        try {
            r = parseStrict(s);
        } catch (NumberFormatException e) {
            return -100;
        }
        return r * 2;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] ints = {"7", "x", "0", "-3", "abc"};
        for (int i = 0; i < ints.length; i++) {
            sb.append("ci=").append(crossArmInt(ints[i])).append(';');
            sb.append("cs=").append(crossArmStr(ints[i])).append(';');
            sb.append("cf=").append(crossArmFinally(ints[i])).append(';');
            sb.append("to=").append(tryOnlyReadAfter(ints[i])).append(';');
        }
        sb.append(',');

        Object[] objs = {null, "hello", "", "abcd"};
        for (int i = 0; i < objs.length; i++) {
            sb.append("mc=").append(multiCatchCrossArm(objs[i])).append(';');
        }
        sb.append(',');

        String[] batch = {"1", "2", "bad", "4"};
        sb.append("loop=").append(loopCrossArm(batch)).append(';');
        sb.append(',');

        sb.append("n1=").append(nestedTryCrossArm("3", "4")).append(';');
        sb.append("n2=").append(nestedTryCrossArm("x", "4")).append(';');
        sb.append("n3=").append(nestedTryCrossArm("3", "y")).append(';');

        System.out.println(sb);
    }
}
