package codec;

// NarrowParamField reproduces guava ArrayBasedCharEscaper's failure mode: a constructor takes narrow
// int-category parameters (char/byte/short) and stores them into same-typed fields. The JVM keeps
// char/byte/short locals in int-sized slots using the very same iload/istore opcodes as int, so once an
// in-body int-constant reassignment unifies a parameter's slot to int, the decompiler renders the
// PARAMETER as `int`. Assigning that int parameter back to its `char` field is then a javac error
// ("possible lossy conversion from int to char"). The descriptor is the ground truth for primitive
// parameter types; Bug AH's fix re-declares the parameter with its descriptor type. A deterministic
// FNV-1a fingerprint over the accessors also guards against behavioral drift, not just a recompile error.
public class NarrowParamField {
    private final char safeMin;
    private final char safeMax;
    private final byte tag;
    private final short level;

    NarrowParamField(char min, char max, byte tag, short level) {
        // Reassigning to int-typed constants forces the slot of min/max to unify to int in the
        // decompiler, the exact trigger that widens the rendered parameter type.
        if (min > max) {
            min = 0;
            max = 65535;
        }
        this.safeMin = min;
        this.safeMax = max;
        this.tag = tag;
        this.level = level;
    }

    char min() {
        return safeMin;
    }

    char max() {
        return safeMax;
    }

    byte tag() {
        return tag;
    }

    short level() {
        return level;
    }

    public static void main(String[] args) {
        NarrowParamField a = new NarrowParamField((char) 'A', (char) 'Z', (byte) 7, (short) 300);
        NarrowParamField b = new NarrowParamField((char) 90, (char) 65, (byte) -3, (short) -1);
        long fp = 1469598103934665603L;
        fp = (fp ^ (long) a.min()) * 1099511628211L;
        fp = (fp ^ (long) a.max()) * 1099511628211L;
        fp = (fp ^ (long) a.tag()) * 1099511628211L;
        fp = (fp ^ (long) a.level()) * 1099511628211L;
        fp = (fp ^ (long) b.min()) * 1099511628211L;
        fp = (fp ^ (long) b.max()) * 1099511628211L;
        fp = (fp ^ (long) b.tag()) * 1099511628211L;
        fp = (fp ^ (long) b.level()) * 1099511628211L;
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
