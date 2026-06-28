package codec;

// NullInitSlotReuse exercises the dominance-gated null-init slot adoption fix.
// javac reuses one JVM local slot for a null-initialized Throwable holder (assigned in a
// dominated catch on one branch) and an unrelated-typed String accumulator on a disjoint branch.
// The single global slot table makes the sibling-branch String store see the Throwable's null
// initializer purely by DFS order; adopting it would unify the two variables onto one mis-typed
// name (Throwable used as String -> "Throwable cannot be converted to String" /
// "cannot find symbol getMessage"). Dominance gating keeps them as two correctly-typed variables.
// Keywords: null-init adoption, slot reuse, try/catch holder, branch dominance.
public class NullInitSlotReuse {

    static int risky(int x) {
        if (x == 7) {
            throw new RuntimeException("boom7");
        }
        return x * 2;
    }

    // mode==1: a Throwable holder (null-initialized) reused across a loop with a dominated catch.
    // else:    a String accumulator that javac packs into the SAME slot as the holder.
    static String classify(int mode, int[] data, String[] words) {
        if (mode == 1) {
            Throwable holder = null;
            int sum = 0;
            for (int i = 0; i < data.length; i++) {
                try {
                    sum += risky(data[i]);
                } catch (RuntimeException ex) {
                    holder = ex;
                }
            }
            if (holder != null) {
                return "err:" + holder.getMessage() + ":" + sum;
            }
            return "ok:" + sum;
        } else {
            String acc = words.length > 0 ? words[0] : "none";
            for (int i = 1; i < words.length; i++) {
                acc = acc + "," + words[i];
            }
            return "join:" + acc;
        }
    }

    // A second shape: an Exception holder null-init then read after the try, with the slot reused by
    // a long counter in the other branch (reference vs primitive split must still hold).
    static String tally(boolean failing, int n) {
        if (failing) {
            Exception err = null;
            try {
                if (n < 0) {
                    throw new IllegalStateException("neg");
                }
            } catch (Exception e) {
                err = e;
            }
            return err == null ? "clean" : "caught:" + err.getMessage();
        } else {
            long total = 0L;
            for (int i = 0; i < n; i++) {
                total += (long) i * 3L;
            }
            return "total:" + total;
        }
    }

    public static void main(String[] args) {
        StringBuilder fp = new StringBuilder();
        fp.append(classify(1, new int[]{1, 2, 3}, new String[0])).append('|');
        fp.append(classify(1, new int[]{1, 7, 3}, new String[0])).append('|');
        fp.append(classify(0, null, new String[]{"x", "y", "z"})).append('|');
        fp.append(classify(0, null, new String[0])).append('|');
        fp.append(tally(true, 5)).append('|');
        fp.append(tally(true, -1)).append('|');
        fp.append(tally(false, 6)).append('|');
        System.out.println(fp.toString());
    }
}
