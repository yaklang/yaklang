package codec;

// BooleanEdge probes Bug AI (boolean<->int confusion). In bytecode boolean shares iconst/istore/iload/
// ireturn with int, so a decompiler that loses the `Z` descriptor renders boolean as int (or vice
// versa), producing "int cannot be converted to boolean" / "boolean cannot be converted to int" on
// recompile. This battery exercises boolean fields, params, returns, ternaries, short-circuit, arrays,
// and boolean<->int interplay, with a deterministic fingerprint to also catch behavioral drift.
public class BooleanEdge {
    private boolean ready;
    private final boolean fixed;
    private boolean[] flags;

    BooleanEdge(boolean fixed) {
        this.fixed = fixed;
        this.ready = false;
        this.flags = new boolean[]{true, false, fixed};
    }

    boolean isReady() {
        return ready;
    }

    void setReady(boolean r) {
        this.ready = r;
    }

    static boolean isPositive(int x) {
        return x > 0;
    }

    static boolean and(boolean a, boolean b) {
        return a && b;
    }

    static boolean either(boolean a, boolean b) {
        return a || b;
    }

    static boolean pick(boolean cond, boolean t, boolean f) {
        return cond ? t : f;
    }

    static int boolToInt(boolean b) {
        return b ? 1 : 0;
    }

    static int countTrue(boolean[] bs) {
        int n = 0;
        for (boolean b : bs) {
            if (b) {
                n++;
            }
        }
        return n;
    }

    public static void main(String[] args) {
        BooleanEdge e = new BooleanEdge(true);
        long fp = 1469598103934665603L;

        fp = (fp ^ (long) boolToInt(e.isReady())) * 1099511628211L;
        e.setReady(true);
        fp = (fp ^ (long) boolToInt(e.isReady())) * 1099511628211L;

        fp = (fp ^ (long) boolToInt(isPositive(5))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(isPositive(-5))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(and(true, false))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(and(true, true))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(either(false, false))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(either(true, false))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(pick(true, false, true))) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(pick(false, false, true))) * 1099511628211L;

        fp = (fp ^ (long) countTrue(e.flags)) * 1099511628211L;
        fp = (fp ^ (long) boolToInt(e.fixed)) * 1099511628211L;

        System.out.println("fp=" + Long.toHexString(fp));
    }
}
