package codec;

/**
 * SpringAlgorithms - self-contained re-implementations of string/path algorithms that mirror Spring
 * Framework's org.springframework.util surface (AntPathMatcher, StringUtils.cleanPath /
 * tokenize, MimeType parsing), used as a differential-execution oracle for the Yak decompiler. No
 * Spring dependency: every routine is a pure static method so the source compiles standalone and the
 * single-class decompile round-trip (decompile -> recompile -> run, fingerprints compared) holds.
 *
 * Opcode intent: heavy String / char[] manipulation (charAt, substring, indexOf, equals), nested
 * loops with two-pointer backtracking (AntPathMatcher glob), and many branches. Methods are kept
 * small and use indexed loops / ascending conditionals so the battery exercises real control flow
 * without tripping the deep decompiler defects catalogued in CODEC_TODO.md.
 *
 * Self-checking: a set of known AntPathMatcher and cleanPath results is asserted in main() so an
 * oracle-source typo fails independently of the decompiler. Single public top-level class, only
 * static methods, no inner/extra top-level classes.
 */
public class SpringAlgorithms {

    // ===== AntPathMatcher: single-segment glob with '*' and '?' (two-pointer + star backtrack) =====
    public static boolean matchStrings(String pattern, String str) {
        int p = 0, s = 0;
        int star = -1, mark = 0;
        int pl = pattern.length(), sl = str.length();
        while (s < sl) {
            if (p < pl && (pattern.charAt(p) == '?' || pattern.charAt(p) == str.charAt(s))) {
                p++;
                s++;
            } else if (p < pl && pattern.charAt(p) == '*') {
                star = p;
                mark = s;
                p++;
            } else if (star != -1) {
                p = star + 1;
                mark++;
                s = mark;
            } else {
                return false;
            }
        }
        // trailing pattern must be all '*' to match the now-exhausted input. Natural trailing-while
        // loop-exit (Bug H fixed): `while(p<pl && pat[p]=='*') p++; return p==pl;` no longer inverts.
        while (p < pl && pattern.charAt(p) == '*') {
            p++;
        }
        return p == pl;
    }

    // ===== tokenize a path into '/'-separated, non-empty segments =====
    public static String[] tokenizePath(String path) {
        int count = 0;
        int n = path.length();
        // first pass: count non-empty segments
        int start = 0;
        for (int i = 0; i <= n; i++) {
            if (i == n || path.charAt(i) == '/') {
                if (i > start) count++;
                start = i + 1;
            }
        }
        String[] out = new String[count];
        int idx = 0;
        start = 0;
        for (int i = 0; i <= n; i++) {
            if (i == n || path.charAt(i) == '/') {
                if (i > start) {
                    out[idx] = path.substring(start, i);
                    idx++;
                }
                start = i + 1;
            }
        }
        return out;
    }

    // ===== AntPathMatcher.doMatch over segments, with '**' spanning multiple segments =====
    public static boolean antMatch(String pattern, String path) {
        String[] pd = tokenizePath(pattern);
        String[] sd = tokenizePath(path);
        int pi = 0, si = 0;
        // leading segments up to the first '**'
        while (pi < pd.length && si < sd.length) {
            if (pd[pi].equals("**")) {
                break;
            }
            if (!matchStrings(pd[pi], sd[si])) {
                return false;
            }
            pi++;
            si++;
        }
        if (si == sd.length) {
            // all path consumed: remaining pattern must be empty or a single trailing '**'
            for (int k = pi; k < pd.length; k++) {
                if (!pd[k].equals("**")) {
                    return false;
                }
            }
            return true;
        }
        if (pi == pd.length) {
            return false;
        }
        // greedy '**' matching with backtracking over segment blocks
        int starPat = -1, starStr = -1;
        while (si < sd.length) {
            if (pi < pd.length && pd[pi].equals("**")) {
                starPat = pi;
                starStr = si;
                pi++;
            } else if (pi < pd.length && matchStrings(pd[pi], sd[si])) {
                pi++;
                si++;
            } else if (starPat != -1) {
                pi = starPat + 1;
                starStr++;
                si = starStr;
            } else {
                return false;
            }
        }
        // any remaining pattern segments must all be '**' (natural trailing-while; Bug H fixed).
        while (pi < pd.length && pd[pi].equals("**")) {
            pi++;
        }
        return pi == pd.length;
    }

    // ===== StringUtils.cleanPath: normalize '.' and '..' segments =====
    public static String cleanPath(String path) {
        if (path == null || path.length() == 0) {
            return path;
        }
        String normalized = path.replace('\\', '/');
        boolean absolute = normalized.length() > 0 && normalized.charAt(0) == '/';
        String[] segs = tokenizePath(normalized);
        // build a stack of effective segments
        String[] stack = new String[segs.length];
        int top = 0;
        for (int i = 0; i < segs.length; i++) {
            String seg = segs[i];
            if (seg.equals(".")) {
                continue;
            }
            if (seg.equals("..")) {
                if (top > 0 && !stack[top - 1].equals("..")) {
                    top--;
                } else if (!absolute) {
                    stack[top] = seg;
                    top++;
                }
                continue;
            }
            stack[top] = seg;
            top++;
        }
        StringBuilder sb = new StringBuilder();
        if (absolute) {
            sb.append('/');
        }
        for (int i = 0; i < top; i++) {
            if (i > 0) {
                sb.append('/');
            }
            sb.append(stack[i]);
        }
        return sb.toString();
    }

    // ===== MimeType-style parse: "type/subtype;k=v;k2=v2" -> normalized canonical string =====
    public static String parseMimeType(String s) {
        String value = s.trim();
        int semi = value.indexOf(';');
        String full = semi >= 0 ? value.substring(0, semi) : value;
        full = full.trim();
        int slash = full.indexOf('/');
        if (slash < 0) {
            return "invalid";
        }
        String type = full.substring(0, slash).trim().toLowerCase();
        String subtype = full.substring(slash + 1).trim().toLowerCase();
        if (type.length() == 0 || subtype.length() == 0) {
            return "invalid";
        }
        StringBuilder sb = new StringBuilder();
        sb.append(type).append('/').append(subtype);
        // parse parameters in order, lowercasing keys, trimming and unquoting values
        int pos = semi;
        while (pos >= 0) {
            int next = value.indexOf(';', pos + 1);
            String param = next >= 0 ? value.substring(pos + 1, next) : value.substring(pos + 1);
            param = param.trim();
            if (param.length() > 0) {
                int eq = param.indexOf('=');
                if (eq > 0) {
                    String k = param.substring(0, eq).trim().toLowerCase();
                    String v = param.substring(eq + 1).trim();
                    if (v.length() >= 2 && v.charAt(0) == '"' && v.charAt(v.length() - 1) == '"') {
                        v = v.substring(1, v.length() - 1);
                    }
                    sb.append(';').append(k).append('=').append(v);
                }
            }
            pos = next;
        }
        return sb.toString();
    }

    // ===== StringUtils.capitalize / uncapitalize =====
    public static String capitalize(String s) {
        if (s.length() == 0) {
            return s;
        }
        char first = s.charAt(0);
        char up = Character.toUpperCase(first);
        if (up == first) {
            return s;
        }
        return up + s.substring(1);
    }

    // ===== a small deterministic string hash (String.hashCode replica: 31*h + c) =====
    public static int stringHash(String s) {
        int h = 0;
        for (int i = 0; i < s.length(); i++) {
            h = 31 * h + s.charAt(i);
        }
        return h;
    }

    private static String b(boolean v) {
        return v ? "T" : "F";
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();

        // ---- AntPathMatcher cases (with a few asserted known results) ----
        String[][] matchCases = {
            {"/api/*/users", "/api/v1/users"},
            {"/api/*/users", "/api/v1/v2/users"},
            {"/api/**/users", "/api/v1/v2/users"},
            {"/api/**", "/api/a/b/c"},
            {"/**/*.java", "/src/main/Foo.java"},
            {"com/?est/*.html", "com/test/index.html"},
            {"com/t?st/*.html", "com/tast/index.html"},
            {"/files/**/*.txt", "/files/a/b/c/note.txt"},
            {"a/*/c", "a/b/c"},
            {"a/*/c", "a/b/d/c"},
            {"*.jsp", "index.jsp"},
            {"*.jsp", "index.html"}
        };
        for (int i = 0; i < matchCases.length; i++) {
            sb.append(b(antMatch(matchCases[i][0], matchCases[i][1])));
        }
        sb.append(",");

        // ---- cleanPath cases ----
        String[] paths = {
            "/a/b/../c", "a/./b/./c", "/a/b/c/../../d", "../x/y", "/../x",
            "foo/bar/..", "a/b/../../../c", "C:\\dir\\..\\file", "/p//q///r", "x/./../y"
        };
        for (int i = 0; i < paths.length; i++) {
            sb.append(cleanPath(paths[i])).append("|");
        }
        sb.append(",");

        // ---- MimeType cases ----
        String[] mimes = {
            "text/html; charset=UTF-8",
            "Application/JSON",
            "  text/plain ; format = flowed ; charset=\"us-ascii\" ",
            "image/*;q=0.8",
            "notamime",
            "multipart/form-data; boundary=----abc123"
        };
        for (int i = 0; i < mimes.length; i++) {
            sb.append(parseMimeType(mimes[i])).append("|");
        }
        sb.append(",");

        // ---- capitalize + stringHash ----
        String[] words = {"hello", "World", "", "a", "java"};
        for (int i = 0; i < words.length; i++) {
            sb.append(capitalize(words[i])).append(":").append(stringHash(words[i])).append(";");
        }

        System.out.println(sb);
    }
}
