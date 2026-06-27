package codec;

/**
 * ChainedAssignAlgorithms - exercises javac's chained-assignment dup idiom `a = b = ... = expr`,
 * where one produced value is duplicated (DUP) into N sequential local stores.
 *
 * This is the round-trip guard for the chained-assignment dup-collapse fix in code_analyser.go.
 * Historically the 2-consumer dup-collapse modelled only the terminal chain `int b = a = 1;`
 * (no downstream read). Applied to a chain whose locals are READ afterward, or to a >=3 slot
 * chain (`a = b = c = expr`), it inlined the value into a single untyped local and emitted
 * non-recompilable `(Object x = 9) * 10` (and, for deeper chains, a phantom loop). The fix keeps
 * the collapse for the terminal / ternary-assignment cases and falls back to the natural
 * `T t = expr; ... t ... t ...` shape when the chained locals are consumed.
 *
 * Covered (all round-trip byte-for-byte):
 *   - 2/3/4-slot int chains whose locals are consumed in an expression
 *   - long chains, mixed-arithmetic chains
 *   - chain followed by independent modification of individual locals (must NOT collapse)
 *   - array-element chains `arr[i] = arr[j] = v`
 *   - terminal chain with no downstream read (the ContinuousAssign-style collapse)
 *
 * Single public top-level class, static only, deterministic.
 */
public class ChainedAssignAlgorithms {

    // ---- 2-slot chain whose locals are both read afterward ----
    public static int twoSlot(int v) {
        int a, b;
        a = b = v;
        return a * 10 + b;
    }

    // ---- 3-slot chain whose locals are all read afterward ----
    public static int threeSlot(int v) {
        int a, b, c;
        a = b = c = v;
        return a * 100 + b * 10 + c;
    }

    // ---- 4-slot chain mixed into an arithmetic expression ----
    public static long fourSlot(long v) {
        long a, b, c, d;
        a = b = c = d = v;
        return a + b * 2L + c * 3L + d * 4L;
    }

    // ---- chain followed by independent modification: locals must stay distinct ----
    public static int chainThenModify(int v) {
        int a, b, c;
        a = b = c = v;
        a += 1;
        b *= 2;
        return a * 100 + b * 10 + c;
    }

    // ---- array-element chain `arr[i] = arr[j] = v` plus an int slot chain ----
    public static int arrayChain(int v) {
        int[] arr = new int[4];
        int i, j;
        i = j = 0;
        arr[i] = arr[j] = v;
        i++;
        arr[i] = v + 5;
        return arr[0] + arr[1] + i + j;
    }

    // ---- terminal chain with no downstream read (the collapse should fire) ----
    public static int terminalChain(int v) {
        int a = v + 1;
        int b = a = v + 2;
        // b/a are not read again; return a literal that does not depend on them
        return (a == b) ? 1 : 0;
    }

    // ---- nested chains feeding another chain ----
    public static long nestedChain(long v) {
        long p, q, r, s;
        p = q = v + 1L;
        r = s = v + 2L;
        return p ^ q ^ r ^ s ^ (p + q + r + s);
    }

    // ---- assignment-as-expression inside a ternary, then a read of the assigned local ----
    // javac dup-collapses each arm into an embedded `x = N` and assigns the whole ternary to y; if
    // the decompiler then single-use-folds the ternary into `return x + y`, the embedded store is
    // reordered AFTER the left `x` read and yields the pre-store value. The store must stay first, so
    // y has to remain an explicit local. f(8)=200 (not 108), f(-3)=400 (not -206). Regression guard
    // for the side-effect-reorder fix in code_analyser.go.
    public static int ternaryAssign(int v) {
        int x = v;
        int y = (v > 0) ? (x = 100) : (x = 200);
        return x + y;
    }

    // ---- ternary assignment where the read is on the RIGHT (already-correct ordering) ----
    public static int ternaryAssignRight(int v) {
        int x = v;
        int y = (v > 0) ? (x = 7) : (x = 9);
        return y * 1000 + x;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(twoSlot(7)).append(',');
        sb.append(threeSlot(9)).append(',');
        sb.append(Long.toHexString(fourSlot(5L))).append(',');
        sb.append(chainThenModify(5)).append(',');
        sb.append(arrayChain(3)).append(',');
        sb.append(terminalChain(4)).append(',');
        sb.append(Long.toHexString(nestedChain(6L))).append(',');
        sb.append(ternaryAssign(8)).append(',');
        sb.append(ternaryAssign(-3)).append(',');
        sb.append(ternaryAssignRight(8)).append(',');
        sb.append(ternaryAssignRight(-3));
        System.out.println(sb);
    }
}
