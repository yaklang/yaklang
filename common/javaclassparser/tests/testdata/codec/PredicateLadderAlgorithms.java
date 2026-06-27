package codec;

/**
 * PredicateLadderAlgorithms - self-hosted, dependency-free boolean-predicate ladders modelled on
 * Guava CharMatcher / Spring StringUtils shapes. Its sole purpose is to exercise the short-circuit
 * boolean reconstruction in the decompiler across EVERY common shape, because javac materializes
 * `boolean b = <short-circuit expr>; if (b) ...` via the iconst_1/goto/iconst_0 idiom into a local
 * slot and the value-merge ternary rebuilder must fold the WHOLE chain (a partial fold drops the
 * then-arm body / shared loop increment and silently produces an infinite loop).
 *
 * Shapes covered (each as a stored boolean used by an if AND inside a loop transformation):
 *   - pure ||-chain of equality tests (isAsciiWhitespace);
 *   - ||-chain whose operands are compound `lo<=c && c<=hi` ranges (isHexDigit, isUnreserved);
 *   - pure &&-chain (isPrintableNonPercent);
 *   - mixed precedence `a && b || c && d` (looksLikeWordChar);
 *   - negation of a parenthesised disjunction (isNotControl);
 *   - predicate reused as a loop guard with a counter (countMatching / stripNonWord / squeezeRuns).
 *
 * Single public top-level class, static only, plain counted loops + ascending if/else, fingerprint
 * is a human-recognisable concatenation so a divergence is obvious.
 */
public class PredicateLadderAlgorithms {

    // ||-chain of equality tests.
    private static boolean isAsciiWhitespace(int c) {
        return (c == ' ') || (c == '\t') || (c == '\n') || (c == '\r') || (c == '\f');
    }

    // ||-chain whose operands are compound `lo<=c && c<=hi` ranges.
    private static boolean isHexDigit(int c) {
        return ((c >= '0') && (c <= '9')) || ((c >= 'a') && (c <= 'f')) || ((c >= 'A') && (c <= 'F'));
    }

    // The classic RFC 3986 unreserved predicate: several compound ranges then several equalities.
    private static boolean isUnreserved(int c) {
        return ((c >= 'A') && (c <= 'Z'))
                || ((c >= 'a') && (c <= 'z'))
                || ((c >= '0') && (c <= '9'))
                || (c == '-') || (c == '_') || (c == '.') || (c == '~');
    }

    // pure &&-chain.
    private static boolean isPrintableNonPercent(int c) {
        return (c >= 32) && (c <= 126) && (c != '%') && (c != '+');
    }

    // mixed precedence `a && b || c && d`: a letter OR a digit-with-underscore-context proxy.
    private static boolean looksLikeWordChar(int c, boolean prevWasLetter) {
        return ((c >= 'a') && (c <= 'z')) || (prevWasLetter && ((c >= '0') && (c <= '9')));
    }

    // negation of a parenthesised disjunction.
    private static boolean isNotControl(int c) {
        return !(((c >= 0) && (c < 32)) || (c == 127));
    }

    // ---- transformations that use the predicates as loop guards ----

    private static int countMatching(String s, int kind) {
        int n = 0;
        for (int i = 0; i < s.length(); i++) {
            int c = s.charAt(i);
            boolean hit;
            if (kind == 0) {
                hit = isAsciiWhitespace(c);
            } else if (kind == 1) {
                hit = isHexDigit(c);
            } else if (kind == 2) {
                hit = isUnreserved(c);
            } else if (kind == 3) {
                hit = isPrintableNonPercent(c);
            } else {
                hit = isNotControl(c);
            }
            if (hit) {
                n++;
            }
        }
        return n;
    }

    private static String stripNonWord(String s) {
        StringBuilder sb = new StringBuilder();
        boolean prevLetter = false;
        for (int i = 0; i < s.length(); i++) {
            int c = s.charAt(i);
            if (looksLikeWordChar(c, prevLetter)) {
                sb.append((char) c);
            }
            prevLetter = (c >= 'a') && (c <= 'z');
        }
        return sb.toString();
    }

    // Collapse runs of whitespace to a single space and trim; uses isAsciiWhitespace as a guard
    // inside a variable-stride inner loop (Spring StringUtils.trimAllWhitespace flavor).
    private static String squeezeWhitespace(String s) {
        StringBuilder sb = new StringBuilder();
        int i = 0;
        while (i < s.length()) {
            int c = s.charAt(i);
            if (isAsciiWhitespace(c)) {
                while ((i < s.length()) && isAsciiWhitespace(s.charAt(i))) {
                    i++;
                }
                if ((sb.length() > 0) && (i < s.length())) {
                    sb.append(' ');
                }
            } else {
                sb.append((char) c);
                i++;
            }
        }
        return sb.toString();
    }

    private static String packBits(boolean[] flags) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < flags.length; i++) {
            sb.append(flags[i] ? '1' : '0');
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        // Per-character predicate bitmaps over a fixed probe alphabet, so any predicate mis-merge
        // flips a recognizable bit.
        int[] probes = {' ', '\t', '0', '9', 'a', 'f', 'g', 'A', 'F', 'Z', '-', '_', '~', '%', '+', 127, 7};

        boolean[] ws = new boolean[probes.length];
        boolean[] hex = new boolean[probes.length];
        boolean[] un = new boolean[probes.length];
        boolean[] pr = new boolean[probes.length];
        boolean[] nc = new boolean[probes.length];
        for (int i = 0; i < probes.length; i++) {
            ws[i] = isAsciiWhitespace(probes[i]);
            hex[i] = isHexDigit(probes[i]);
            un[i] = isUnreserved(probes[i]);
            pr[i] = isPrintableNonPercent(probes[i]);
            nc[i] = isNotControl(probes[i]);
        }
        sb.append(packBits(ws)).append('|');
        sb.append(packBits(hex)).append('|');
        sb.append(packBits(un)).append('|');
        sb.append(packBits(pr)).append('|');
        sb.append(packBits(nc)).append('/');

        String[] texts = {"Hello, World!", "  a1b2  c3 ", "ff00AAzz", "a-b_c.d~e%f+g"};
        for (int t = 0; t < texts.length; t++) {
            String s = texts[t];
            sb.append(countMatching(s, 0)).append(',');
            sb.append(countMatching(s, 1)).append(',');
            sb.append(countMatching(s, 2)).append(',');
            sb.append(countMatching(s, 3)).append(',');
            sb.append(countMatching(s, 4)).append(';');
        }
        sb.append('/');

        for (int t = 0; t < texts.length; t++) {
            sb.append(stripNonWord(texts[t])).append('~');
        }
        sb.append('/');

        for (int t = 0; t < texts.length; t++) {
            sb.append('[').append(squeezeWhitespace(texts[t])).append(']');
        }

        System.out.println(sb);
    }
}
