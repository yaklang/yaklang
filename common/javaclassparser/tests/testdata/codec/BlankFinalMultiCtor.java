package codec;

// BlankFinalMultiCtor is a self-hosted regression battery for the CROSS-constructor field-initializer
// hoisting guard. It mirrors the commons-codec Base32/Base64 idiom where several overloaded
// constructors each assign the SAME blank-final field exactly once.
//
// The per-constructor assignment count is 1 in every constructor, so the per-body guard alone would
// still wrongly lift one constructor's constant assignment (e.g. `private final int lineLength = 0;`)
// into a field initializer. The other constructors then either keep their own assignment of that
// final (illegal double assignment -> "cannot assign a value to final variable") or, when their
// right-hand side is also hoistable, get silently collapsed to a single shared value (semantic
// divergence). Only the class-wide store count (constructorFieldStoreTotals) sees that the field is
// assigned in more than one place and suppresses the hoist.
//
// Keywords: field initializer hoisting, blank final, multiple constructors, cross-constructor.
public class BlankFinalMultiCtor {
    // size/lineLength are blank finals assigned once in EACH constructor -> total store count > 1,
    // so neither may be hoisted even though each constructor assigns it exactly once.
    private final int size;
    private final int lineLength;
    private final String label;

    BlankFinalMultiCtor() {
        this.size = 4;
        this.lineLength = 0;
        this.label = "default";
    }

    BlankFinalMultiCtor(int n) {
        this.size = n;
        this.lineLength = 8;
        this.label = "n=" + n;
    }

    BlankFinalMultiCtor(int n, int width) {
        this.size = n + width;
        this.lineLength = width;
        this.label = "w" + width;
    }

    int score() {
        return this.size * 31 + this.lineLength * 7 + this.label.length();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(new BlankFinalMultiCtor().score()).append(';');
        sb.append(new BlankFinalMultiCtor(5).score()).append(';');
        sb.append(new BlankFinalMultiCtor(3, 16).score()).append(';');
        sb.append(new BlankFinalMultiCtor(0, 0).score()).append(';');
        sb.append(new BlankFinalMultiCtor(-7).score()).append(';');
        System.out.println(sb.toString());
    }
}
