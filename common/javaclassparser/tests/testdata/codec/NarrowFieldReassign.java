package codec;

// NarrowFieldReassign reproduces fastjson2 JSONWriter's failure mode: a char/byte field is REASSIGNED
// (not declared) from a non-constant int-category expression, here a conditional `single ? '\'' : '"'`.
// javac types the conditional as char (both arms are char literals) and lowers it to a branch that
// pushes the char constants as ints (the JVM operand stack has no char category) followed by
// `putfield ... C`, which truncates implicitly -- so NO i2c opcode is emitted and the decompiler
// recovers the arms as int literals and the conditional as int-typed. Rendered verbatim that is
// `this.quote = single ? 39 : 34`, which javac rejects in assignment context ("possible lossy
// conversion from int to char"): a non-constant conditional is not a constant expression, so the
// constant-narrowing exception (which makes a bare `char q = 39;` legal) does not apply. The
// narrowing-reassignment cast in AssignStatement.String emits `this.quote = (char)(single ? 39 : 34)`,
// exactly the truncation the store opcode performs, so behavior is identical. A deterministic FNV-1a
// fingerprint over the accessors also guards against behavioral drift, not just a recompile error.
public class NarrowFieldReassign {
    private char quote;
    private byte mode;
    private short scale;

    NarrowFieldReassign(boolean single, boolean compact, boolean wide) {
        this.quote = single ? '\'' : '"';
        this.mode = compact ? (byte) 1 : (byte) 2;
        this.scale = wide ? (short) 1000 : (short) 7;
    }

    char quote() {
        return quote;
    }

    byte mode() {
        return mode;
    }

    short scale() {
        return scale;
    }

    public static void main(String[] args) {
        NarrowFieldReassign a = new NarrowFieldReassign(true, true, false);
        NarrowFieldReassign b = new NarrowFieldReassign(false, false, true);
        long fp = 1469598103934665603L;
        fp = (fp ^ (long) a.quote()) * 1099511628211L;
        fp = (fp ^ (long) a.mode()) * 1099511628211L;
        fp = (fp ^ (long) a.scale()) * 1099511628211L;
        fp = (fp ^ (long) b.quote()) * 1099511628211L;
        fp = (fp ^ (long) b.mode()) * 1099511628211L;
        fp = (fp ^ (long) b.scale()) * 1099511628211L;
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
