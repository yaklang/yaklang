package codec;

/**
 * GuardChainAlgorithms - a focused battery for early-return guard chains with an intervening local
 * store between the guards. This is the canonical "Bug M" shape from real Spring/Guava string utils:
 *
 *     int extIndex = path.lastIndexOf('.');
 *     if (extIndex == -1) { return ...; }   // guard 1: sibling branch is a bare return
 *     int folderIndex = path.lastIndexOf('/');  // intervening local store on the fall-through branch
 *     if (folderIndex > extIndex) { return ...; }  // guard 2
 *     return ...;
 *
 * The store that gets folded away sits on guard 1's jump-target branch while the sibling branch
 * returns. A previous var-fold rewiring (remove + append) reversed the condition's two successors,
 * silently swapping guard 1's then/else bodies. The fix preserves successor order for non-loop
 * guards whose sibling branch returns; this battery keeps that behaviour locked in behaviorally.
 *
 * It also includes the adversarial cases the fix must NOT disturb: a counted loop whose exit branch
 * flows into the method's return (so the loop header also "has a return sibling", but its body forms
 * a back-edge and must keep the historical ordering), and a guard whose sibling THROWS rather than
 * returns. Pure static methods, single public top-level class, deterministic fingerprint in main().
 *
 * Opcode intent: lastIndexOf/indexOf/charAt/substring, ifeq/ifne/if_icmp* guard branches, local
 * store/load between guards, athrow on a guard sibling, counted for-loop with early return.
 */
public class GuardChainAlgorithms {

    // Canonical Bug M shape: two early-return guards with an intervening local store; the store is a
    // single-use temp the decompiler folds into the second guard.
    public static String extensionOf(String path) {
        int extIndex = path.lastIndexOf('.');
        if (extIndex == -1) {
            return "";
        }
        int folderIndex = path.lastIndexOf('/');
        if (folderIndex > extIndex) {
            return "";
        }
        return path.substring(extIndex + 1);
    }

    public static String stripExtension(String path) {
        int extIndex = path.lastIndexOf('.');
        if (extIndex == -1) {
            return path;
        }
        int folderIndex = path.lastIndexOf('/');
        if (folderIndex > extIndex) {
            return path;
        }
        return path.substring(0, extIndex);
    }

    // Three-guard chain, each pair separated by an intervening store, mixing == / != / > / <
    // comparisons so multiple if_icmp* polarities go through the same fold path.
    public static String classify(String s) {
        int dot = s.indexOf('.');
        if (dot < 0) {
            return "nodot";
        }
        int slash = s.indexOf('/');
        if (slash >= 0) {
            return "hasslash";
        }
        int colon = s.indexOf(':');
        if (colon != dot) {
            return "diff";
        }
        return "same";
    }

    // Guard whose sibling THROWS instead of returns: the fix only re-orders for return siblings, so
    // this verifies the append fallback still produces correct branches for throw siblings.
    public static int requirePositive(int v) {
        if (v <= 0) {
            throw new IllegalArgumentException("non-positive");
        }
        int doubled = v * 2;
        if (doubled > 1000) {
            return 1000;
        }
        return doubled;
    }

    // Adversarial: a counted loop whose exit branch falls into the method's return. The loop header
    // therefore has a "return on the exit branch", but the body is a back-edge and must keep the
    // historical ordering (this is the MyBatis-Plus loop shape that the fix must leave untouched).
    public static int firstMatch(int[] xs, int target) {
        int idx = -1;
        for (int i = 0; i < xs.length; i++) {
            int cur = xs[i];
            if (cur == target) {
                idx = i;
                break;
            }
        }
        return idx;
    }

    // Nested guards with an intervening store at the outer level.
    public static String pick(String a, String b) {
        int la = a.length();
        if (la == 0) {
            return b;
        }
        int lb = b.length();
        if (lb == 0) {
            return a;
        }
        if (la > lb) {
            return a;
        }
        return b;
    }

    private static String tag(String name, String val) {
        return name + "=" + val + ";";
    }

    // Wraps requirePositive so the throw/catch (athrow + exception table) is exercised without a
    // local assigned across both try and catch arms (a separate scope-hoist case).
    private static int safeRequire(int v) {
        try {
            return requirePositive(v);
        } catch (IllegalArgumentException e) {
            return -1;
        }
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] paths = {"/a/b/c.txt", "noext", "/a/b/.hidden", "file.tar.gz", "/dir.with.dot/name", ""};
        for (int i = 0; i < paths.length; i++) {
            sb.append(tag("ext", extensionOf(paths[i])));
            sb.append(tag("strip", stripExtension(paths[i])));
        }
        sb.append(',');

        String[] cls = {"plain", "a.b", "a/b", "a.b:c", "a.b.c", "x:y"};
        for (int i = 0; i < cls.length; i++) {
            sb.append(tag("cls", classify(cls[i])));
        }
        sb.append(',');

        int[] reqs = {1, 7, 600, 2000, 0};
        for (int i = 0; i < reqs.length; i++) {
            sb.append("req=").append(safeRequire(reqs[i])).append(';');
        }
        sb.append(',');

        int[] arr = {5, 3, 9, 7};
        sb.append("fm=").append(firstMatch(arr, 9)).append(';');
        sb.append("fm=").append(firstMatch(arr, 42)).append(';');
        sb.append(',');

        String[][] picks = {{"", "b"}, {"a", ""}, {"abc", "de"}, {"de", "abc"}};
        for (int i = 0; i < picks.length; i++) {
            sb.append(tag("pick", pick(picks[i][0], picks[i][1])));
        }

        System.out.println(sb);
    }
}
