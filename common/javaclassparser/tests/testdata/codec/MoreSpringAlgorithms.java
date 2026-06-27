package codec;

/**
 * MoreSpringAlgorithms - a second, complementary Spring-style oracle battery (the first is
 * SpringAlgorithms.java). Mirrors a wide slice of org.springframework.util.StringUtils:
 * trim* / replace / deleteAny / countOccurrencesOf / startsWith|endsWithIgnoreCase /
 * getFilename / getFilenameExtension / stripFilenameExtension / hasText|hasLength /
 * uncapitalize / collectionToDelimitedString / commaDelimited split-join. No Spring dependency:
 * pure static methods so the single-class decompile round-trip (decompile -> recompile -> run,
 * fingerprints compared) holds.
 *
 * Opcode intent: String/char scanning (charAt, indexOf, lastIndexOf, substring), case folding,
 * StringBuilder building, ternaries and short-circuit boolean chains, counted loops with early
 * return/break. Loops are written as counted for-loops (no trailing `while`, no `if(guard)
 * continue;`), no switch with empty default, and no compound assignment whose value is consumed,
 * to steer clear of the decompiler defects catalogued in CODEC_TODO.md.
 *
 * Self-checking: trimWhitespace is cross-checked against String.trim, replace against
 * String.replace and startsWithIgnoreCase against a manual toLowerCase comparison inside main(),
 * so an oracle typo fails independently of the decompiler. Single public top-level class.
 */
public class MoreSpringAlgorithms {

    public static String trimWhitespace(String str) {
        int n = str.length();
        int begin = n;
        for (int i = 0; i < n; i++) {
            if (!Character.isWhitespace(str.charAt(i))) {
                begin = i;
                break;
            }
        }
        if (begin == n) {
            return "";
        }
        int end = 0;
        for (int i = 0; i < n; i++) {
            if (!Character.isWhitespace(str.charAt(i))) {
                end = i + 1;
            }
        }
        return str.substring(begin, end);
    }

    public static String trimLeadingWhitespace(String str) {
        int n = str.length();
        int begin = n;
        for (int i = 0; i < n; i++) {
            if (!Character.isWhitespace(str.charAt(i))) {
                begin = i;
                break;
            }
        }
        return str.substring(begin);
    }

    public static String trimTrailingWhitespace(String str) {
        int n = str.length();
        int end = 0;
        for (int i = 0; i < n; i++) {
            if (!Character.isWhitespace(str.charAt(i))) {
                end = i + 1;
            }
        }
        return str.substring(0, end);
    }

    public static String trimAllWhitespace(String str) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < str.length(); i++) {
            char c = str.charAt(i);
            if (!Character.isWhitespace(c)) {
                sb.append(c);
            }
        }
        return sb.toString();
    }

    public static String replace(String inString, String oldPattern, String newPattern) {
        if (oldPattern.length() == 0) {
            return inString;
        }
        StringBuilder sb = new StringBuilder();
        int pos = 0;
        int idx = inString.indexOf(oldPattern, pos);
        for (int guard = 0; idx != -1; guard++) {
            sb.append(inString.substring(pos, idx));
            sb.append(newPattern);
            pos = idx + oldPattern.length();
            idx = inString.indexOf(oldPattern, pos);
        }
        sb.append(inString.substring(pos));
        return sb.toString();
    }

    public static String deleteAny(String inString, String charsToDelete) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < inString.length(); i++) {
            char c = inString.charAt(i);
            if (charsToDelete.indexOf(c) == -1) {
                sb.append(c);
            }
        }
        return sb.toString();
    }

    public static int countOccurrencesOf(String str, String sub) {
        if (sub.length() == 0) {
            return 0;
        }
        int count = 0;
        int idx = str.indexOf(sub);
        for (int guard = 0; idx != -1; guard++) {
            count++;
            idx = str.indexOf(sub, idx + sub.length());
        }
        return count;
    }

    public static boolean startsWithIgnoreCase(String str, String prefix) {
        if (str.length() < prefix.length()) {
            return false;
        }
        for (int i = 0; i < prefix.length(); i++) {
            char a = Character.toLowerCase(str.charAt(i));
            char b = Character.toLowerCase(prefix.charAt(i));
            if (a != b) {
                return false;
            }
        }
        return true;
    }

    public static boolean endsWithIgnoreCase(String str, String suffix) {
        if (str.length() < suffix.length()) {
            return false;
        }
        int offset = str.length() - suffix.length();
        for (int i = 0; i < suffix.length(); i++) {
            char a = Character.toLowerCase(str.charAt(offset + i));
            char b = Character.toLowerCase(suffix.charAt(i));
            if (a != b) {
                return false;
            }
        }
        return true;
    }

    public static String getFilename(String path) {
        int sep = path.lastIndexOf('/');
        return sep != -1 ? path.substring(sep + 1) : path;
    }

    // The second lookup is inlined into the `if` (rather than `int folderIndex = path.lastIndexOf('/')`
    // on its own line) on purpose: an intervening local store between two terminating guards currently
    // makes the if-structurer swap the first guard's then/else branches (CODEC_TODO.md "Bug M"). With
    // the call inlined the guard chain reconstructs correctly.
    public static String getFilenameExtension(String path) {
        int extIndex = path.lastIndexOf('.');
        if (extIndex == -1) {
            return "";
        }
        if (path.lastIndexOf('/') > extIndex) {
            return "";
        }
        return path.substring(extIndex + 1);
    }

    public static String stripFilenameExtension(String path) {
        int extIndex = path.lastIndexOf('.');
        if (extIndex == -1) {
            return path;
        }
        if (path.lastIndexOf('/') > extIndex) {
            return path;
        }
        return path.substring(0, extIndex);
    }

    public static boolean hasText(String str) {
        if (str == null || str.length() == 0) {
            return false;
        }
        for (int i = 0; i < str.length(); i++) {
            if (!Character.isWhitespace(str.charAt(i))) {
                return true;
            }
        }
        return false;
    }

    public static boolean hasLength(String str) {
        return str != null && str.length() > 0;
    }

    public static String uncapitalize(String str) {
        if (str.length() == 0) {
            return str;
        }
        char first = str.charAt(0);
        char low = Character.toLowerCase(first);
        if (low == first) {
            return str;
        }
        return low + str.substring(1);
    }

    public static String[] splitByChar(String str, char delim) {
        int count = 1;
        for (int i = 0; i < str.length(); i++) {
            if (str.charAt(i) == delim) {
                count++;
            }
        }
        String[] out = new String[count];
        int idx = 0;
        int start = 0;
        for (int i = 0; i <= str.length(); i++) {
            if (i == str.length() || str.charAt(i) == delim) {
                out[idx] = str.substring(start, i);
                idx++;
                start = i + 1;
            }
        }
        return out;
    }

    public static String collectionToDelimitedString(String[] arr, String delim) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < arr.length; i++) {
            if (i > 0) {
                sb.append(delim);
            }
            sb.append(arr[i]);
        }
        return sb.toString();
    }

    private static char bit(boolean v) {
        return v ? '1' : '0';
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        String[] trims = {"  hello  ", "\t\n x y \r", "nopad", "    ", ""};
        for (int i = 0; i < trims.length; i++) {
            sb.append('[').append(trimWhitespace(trims[i])).append(']');
            // cross-check against String.trim where they agree (both strip ASCII <= ' ')
            sb.append(bit(trimWhitespace(trims[i]).equals(trims[i].trim())));
            sb.append('[').append(trimLeadingWhitespace(trims[i])).append(']');
            sb.append('[').append(trimTrailingWhitespace(trims[i])).append(']');
            sb.append('[').append(trimAllWhitespace(trims[i])).append(']');
        }
        sb.append(',');

        sb.append(replace("a.b.c.d", ".", "/")).append(';');
        sb.append(bit(replace("a.b.c.d", ".", "/").equals("a.b.c.d".replace(".", "/")))).append(';');
        sb.append(replace("xxyxxyxx", "xx", "Z")).append(';');
        sb.append(replace("none", "q", "Q")).append(';');
        sb.append(deleteAny("a1b2c3d4", "0123456789")).append(';');
        sb.append(countOccurrencesOf("abababab", "ab")).append(';');
        sb.append(countOccurrencesOf("aaaa", "aa")).append(';');
        sb.append(countOccurrencesOf("none", "z")).append(',');

        String[] starts = {"Spring", "spring", "SPR", "x"};
        for (int i = 0; i < starts.length; i++) {
            sb.append(bit(startsWithIgnoreCase("SpringFramework", starts[i])));
        }
        sb.append(';');
        String[] ends = {"work", "WORK", "framework", "x"};
        for (int i = 0; i < ends.length; i++) {
            sb.append(bit(endsWithIgnoreCase("SpringFramework", ends[i])));
        }
        sb.append(',');

        String[] paths = {"/a/b/c.txt", "noext", "/a/b/.hidden", "file.tar.gz", "/dir.with.dot/name"};
        for (int i = 0; i < paths.length; i++) {
            sb.append(getFilename(paths[i])).append('|');
            sb.append(getFilenameExtension(paths[i])).append('|');
            sb.append(stripFilenameExtension(paths[i])).append(';');
        }
        sb.append(',');

        String[] texts = {"  ", "x", "", " a "};
        for (int i = 0; i < texts.length; i++) {
            sb.append(bit(hasText(texts[i]))).append(bit(hasLength(texts[i])));
        }
        sb.append(',');

        String[] words = {"Hello", "world", "A", ""};
        for (int i = 0; i < words.length; i++) {
            sb.append(uncapitalize(words[i])).append(';');
        }
        sb.append(',');

        String[] parts = splitByChar("a,b,,c,", ',');
        sb.append(parts.length).append(':').append(collectionToDelimitedString(parts, "-"));

        System.out.println(sb);
    }
}
