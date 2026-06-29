package codec;

// Bug AN (1) battery: a method of class X creates an anonymous subclass of X and reads X's own
// private field through the subclass-typed reference. javac types the source local as X (the
// supertype), so `r.tag` is legal inside X. The decompiler instead types the local as the synthetic
// subclass `X$1`, through which the private field `tag` is NOT an accessible member (private members
// are not inherited, JLS 8.2), so the read must be rendered `((X)r).tag` to recompile. Mirrors
// commons-codec Rule.parseRules' `Rule$2 var.pattern`.
public class AnonSubclassOwnPrivateField {
    private final String tag;

    AnonSubclassOwnPrivateField(String tag) {
        this.tag = tag;
    }

    String firstChar() {
        AnonSubclassOwnPrivateField r = new AnonSubclassOwnPrivateField("hello") {
            @Override
            public String toString() {
                return "anon:" + tag;
            }
        };
        // Read the enclosing class's own private field through the anonymous-subclass-typed reference.
        return r.tag.substring(0, 1);
    }

    public static void main(String[] args) {
        AnonSubclassOwnPrivateField base = new AnonSubclassOwnPrivateField("base");
        System.out.println(base.firstChar());
    }
}
