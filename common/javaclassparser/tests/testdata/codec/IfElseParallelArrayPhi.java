package codec;

// If/else parallel-typed phi battery (Bug AL cross-VarUid same-type if/else def family).
// Both arms of the if assign the SAME jvm slot a long[] (a phi at the merge) that is READ after the
// if; the array's slot is then REUSED for a different-typed scalar (the `long extra`, an lstore into
// the array's astore slot - confirmed in the bytecode). javac's DFS lowering explores one arm's
// fall-through - and that later scalar reuse - before backtracking to the other arm, so the simulator
// clobbers the slot table to `long` and mints a FRESH ref for the second arm's long[] store.
// prebindEscapingIfElseSlots only unifies arms sharing a VarUid, so this cross-VarUid same-type phi
// slips past it: without prebindParallelTypedIfElseDefs both arms keep their own `long[] data = ...`
// declaration and the post-if `data[...]` reads bind to only one arm's id, rendering the variable out
// of scope ("cannot find symbol: variable varN"). Mirrors fastjson2 ObjectReaderProvider.<init>'s
// long[] acceptHashCodes. The enclosing block scope on `data`/`total` is what frees their slots for
// the trailing `extra` reuse that triggers the split. Kill-switch: JDEC_IFELSE_PARALLEL_PREBIND_OFF.
public class IfElseParallelArrayPhi {
    static long compute(int n) {
        long result;
        {
            long[] data;
            if (n <= 0) {
                data = new long[1];
            } else {
                data = new long[n + 1];
                int i = 0;
                while (i < n) {
                    data[i] = ((long) i) * 31L;
                    i++;
                }
            }
            data[data.length - 1] = 999983L;
            long total = 0L;
            int j = 0;
            while (j < data.length) {
                total += data[j];
                j++;
            }
            result = total;
        }
        long extra = (result << 3) ^ 0x5DEECE66DL;
        return result + extra;
    }

    public static void main(String[] args) {
        long acc = 0L;
        for (int n = 0; n < 7; n++) {
            acc = (acc * 1000003L) + compute(n);
        }
        System.out.println("fingerprint=" + acc);
    }
}
