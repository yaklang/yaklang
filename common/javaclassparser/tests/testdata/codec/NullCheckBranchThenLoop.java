package codec;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * NullCheckBranchThenLoop - locks the commons-codec Md5Crypt "salt ignored" regression: an
 * `if (ref == null) { random } else { parse }` guard (compiled to `ifnonnull`) whose then/else were
 * silently swapped, so a provided salt took the random branch and was ignored.
 *
 * The minimal trigger (distilled from Md5Crypt.md5Crypt): an object-vs-null guard that
 *   1. merges an OBJECT-typed local (saltString) assigned in both branches at the if-join,
 *   2. has a nested if+throw in the jump-target branch, and
 *   3. is FOLLOWED by test-at-bottom loops (one decrementing loop reading a pre-if value, one
 *      counting loop with an inner if).
 * javac emits the loops as `goto test; body; test: if(cond) goto body`; the node-graph rebuild for
 * that combination reordered the guard's two successors (node.Next) AFTER the condition sense was
 * already fixed, so the position-based true/false pick in RemoveGotoStatement chose the wrong branch.
 * The fix pins the true branch by node identity (Node.JmpNode) at build time. Kill-switch for the
 * negative test: JDEC_IFBRANCH_PIN_OFF=1.
 *
 * The bug is silent (passes ANTLR syntax validation, inverts behaviour), so only differential
 * execution catches it: with a swapped guard a null input would try to parse null (NPE / wrong) and
 * a non-null input would return the constant random tag.
 */
public class NullCheckBranchThenLoop {

    static String pseudoRandom(int n) {
        StringBuilder sb = new StringBuilder();
        for (int k = 0; k < n; k++) {
            sb.append((char) ('a' + ((k * 7 + 3) % 26)));
        }
        return sb.toString();
    }

    static int derive(byte[] key, String salt, String prefix) {
        int keyLen = key.length;
        String saltString;
        if (salt == null) {
            saltString = pseudoRandom(8);
        } else {
            Pattern p = Pattern.compile("^" + prefix.replace("$", "\\$") + "([a-z]+).*");
            Matcher m = p.matcher(salt);
            if (!m.find()) {
                throw new IllegalArgumentException("Invalid salt value: " + salt);
            }
            saltString = m.group(1);
        }
        byte[] saltBytes = saltString.getBytes();
        int acc = saltString.length();
        int ii = keyLen;
        while (ii > 0) {
            acc += saltBytes[0];
            ii -= 16;
        }
        int i = 0;
        while (i < 64) {
            if ((i & 1) != 0) {
                acc += key.length == 0 ? 0 : key[0];
            } else {
                acc += saltBytes[i % saltBytes.length];
            }
            i++;
        }
        return (acc * 31) + saltString.charAt(0);
    }

    public static void main(String[] z) {
        StringBuilder sb = new StringBuilder();
        byte[] key = {3, 5, 7};
        sb.append(derive(key, null, "x")).append(",");
        sb.append(derive(key, "xabcd", "x")).append(",");
        sb.append(derive(key, "xqz", "x")).append(",");
        sb.append(derive(new byte[0], "xhello", "x")).append(",");
        try {
            derive(key, "ZZZ", "x");
            sb.append("nothrow");
        } catch (IllegalArgumentException e) {
            sb.append("threw:").append(e.getMessage());
        }
        System.out.println(sb.toString());
    }
}
