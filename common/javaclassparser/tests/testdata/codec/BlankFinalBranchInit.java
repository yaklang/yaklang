package codec;

// BlankFinalBranchInit is a self-hosted regression battery for the field-initializer hoisting
// guard. It mirrors the commons-codec Base32 idiom where blank-final fields are assigned in
// multiple constructor branches and one field is derived from another instance field.
//
// Before the fix, the decompiler wrongly lifted one branch's assignment of a blank final into a
// field initializer (e.g. `private final int size = 8;`) while leaving the other branch's
// assignment in the constructor body, producing an illegal double-assignment of a final
// ("cannot assign a value to final variable") and silently dropping the other branch. It also
// hoisted `derived = this.size - 1` into a field initializer that reads size before the
// constructor sets it (reading the default 0). Both defects make the round-trip diverge.
//
// Keywords: field initializer hoisting, blank final, constructor branch assignment.
public class BlankFinalBranchInit {
    static final byte[] TABLE_A = new byte[]{1, 2, 3, 4};
    static final byte[] TABLE_B = new byte[]{9, 8, 7, 6};

    // size/table/label are blank finals assigned on BOTH branches -> must NOT be hoisted.
    private final int size;
    private final byte[] table;
    private final String label;
    // derived reads another instance field -> must stay a constructor assignment, not an initializer.
    private final int derived;
    // tag is a genuine single, constant field initializer -> hoisting it is correct.
    private final int tag = 42;

    BlankFinalBranchInit(int n, boolean useB) {
        if (useB) {
            this.table = TABLE_B;
            this.label = "B";
        } else {
            this.table = TABLE_A;
            this.label = "A";
        }
        if (n > 0) {
            this.size = 8 + n;
        } else {
            this.size = 8;
        }
        this.derived = this.size - 1;
    }

    int sum() {
        int s = 0;
        for (byte b : this.table) {
            s += b;
        }
        return s + this.size + this.derived + this.tag + this.label.length();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        int[] ns = {-5, 0, 1, 7, 100};
        boolean[] flags = {true, false};
        for (int n : ns) {
            for (boolean f : flags) {
                BlankFinalBranchInit x = new BlankFinalBranchInit(n, f);
                sb.append(x.size).append(',')
                        .append(x.derived).append(',')
                        .append(x.tag).append(',')
                        .append(x.label).append(',')
                        .append(x.sum()).append(';');
            }
        }
        System.out.println(sb.toString());
    }
}
