package codec;

import java.util.function.IntSupplier;

// Lambda-body local rename battery (Bug AL / lambda-inlining shadow family).
// javac emits each lambda as a separate `lambda$...` method, so its body locals start a FRESH jvm
// slot namespace (slot0, slot1, ...). The decompiler inlines that body as an arrow expression inside
// the enclosing method, where those slots render as var0, var1, ... - the SAME names the enclosing
// method already uses for its own parameters/locals. Java forbids a local declared in a lambda body
// from shadowing an in-scope enclosing local, so the naive inline produces
// "variable var0 is already defined in method compute(int)" and fails to recompile.
// renameLambdaBodyLocals lifts each lambda body's own locals into a private `lv<seq>_N` namespace so
// they never collide. Kill-switch: JDEC_NO_LAMBDA_LOCAL_RENAME.
public class LambdaLocalShadowsCapture {
    // compute's parameter occupies slot 0 (var0) and the IntSupplier local slot 1 (var1); the
    // zero-capture lambda body's own locals also begin at slot 0/1, so without the rename they shadow
    // the enclosing var0/var1.
    static int compute(int seed) {
        IntSupplier s = () -> {
            int acc = 7;
            int i = 0;
            while (i < 3) {
                acc = (acc * 31) + i;
                i++;
            }
            return acc;
        };
        return seed + s.getAsInt();
    }

    public static void main(String[] args) {
        long total = 0L;
        for (int n = 0; n < 5; n++) {
            total += compute(n);
        }
        System.out.println("fingerprint=" + total);
    }
}
