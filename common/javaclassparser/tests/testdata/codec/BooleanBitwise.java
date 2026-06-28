package codec;

// BooleanBitwise probes the subtle half of Bug AI: non-short-circuit boolean operators (&, |, ^) and
// compound boolean assignment (&=, |=, ^=) compile to iand/ior/ixor, the SAME opcodes as int bitwise
// ops. A decompiler that types the result as int then fails to assign it back to a boolean field/return
// ("int cannot be converted to boolean"). This battery isolates those forms with a deterministic
// fingerprint.
public class BooleanBitwise {
    private boolean acc;

    BooleanBitwise(boolean init) {
        this.acc = init;
    }

    static boolean bitAnd(boolean a, boolean b) {
        return a & b;
    }

    static boolean bitOr(boolean a, boolean b) {
        return a | b;
    }

    static boolean bitXor(boolean a, boolean b) {
        return a ^ b;
    }

    void andInto(boolean b) {
        this.acc &= b;
    }

    void orInto(boolean b) {
        this.acc |= b;
    }

    void xorInto(boolean b) {
        this.acc ^= b;
    }

    boolean get() {
        return this.acc;
    }

    static boolean combine(boolean a, boolean b, boolean c) {
        boolean r = a & b | c;
        r ^= a;
        return r;
    }

    static int toInt(boolean b) {
        return b ? 1 : 0;
    }

    public static void main(String[] args) {
        long fp = 1469598103934665603L;
        fp = (fp ^ (long) toInt(bitAnd(true, true))) * 1099511628211L;
        fp = (fp ^ (long) toInt(bitAnd(true, false))) * 1099511628211L;
        fp = (fp ^ (long) toInt(bitOr(false, false))) * 1099511628211L;
        fp = (fp ^ (long) toInt(bitOr(true, false))) * 1099511628211L;
        fp = (fp ^ (long) toInt(bitXor(true, false))) * 1099511628211L;
        fp = (fp ^ (long) toInt(bitXor(true, true))) * 1099511628211L;
        fp = (fp ^ (long) toInt(combine(true, false, true))) * 1099511628211L;

        BooleanBitwise x = new BooleanBitwise(true);
        x.andInto(true);
        fp = (fp ^ (long) toInt(x.get())) * 1099511628211L;
        x.andInto(false);
        fp = (fp ^ (long) toInt(x.get())) * 1099511628211L;
        x.orInto(true);
        fp = (fp ^ (long) toInt(x.get())) * 1099511628211L;
        x.xorInto(true);
        fp = (fp ^ (long) toInt(x.get())) * 1099511628211L;

        System.out.println("fp=" + Long.toHexString(fp));
    }
}
