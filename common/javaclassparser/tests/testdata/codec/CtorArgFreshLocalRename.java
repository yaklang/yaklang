package codec;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Scanner;

// Reproduces the constructor-argument fresh-local rename bug (Bug AN(2), commons-codec
// DaitchMokotoffSoundex.parseRules). A non-trivial method reuses the same JVM slots across two
// disjoint branches (the "=" fold branch and the rule branch) plus a catch handler, so a freshly
// bound local that feeds a constructor argument is minted with a slot-derived `varN` name that
// collides with the array variable's name. Renaming the declaration must also retarget the use
// captured inside `new Rule(...)`'s argument list, or the call site keeps the stale colliding name
// and binds to the wrong same-name variable (a String[] where a String is required).
// Rule is a package-private TOP-LEVEL class (not nested) on purpose: the single-class decompile API
// renders a nested reference as the synthetic `Outer$Rule`, which javac rejects in source and would
// mask this test's signal with the unrelated $-reference defect (Bug AD). A top-level sibling is
// referenced by its simple name and resolves from the classpath in both the OFF and ON runs, so the
// only variable between them is the constructor argument's correctness.
class CtorArgFreshLocalRenameRule {
    final String pattern;
    final String atStart;
    final String beforeVowel;
    final String def;

    CtorArgFreshLocalRenameRule(String pattern, String atStart, String beforeVowel, String def) {
        this.pattern = pattern;
        this.atStart = atStart;
        this.beforeVowel = beforeVowel;
        this.def = def;
    }

    public String toString() {
        return pattern + ">" + atStart + "/" + beforeVowel + "/" + def;
    }
}

public class CtorArgFreshLocalRename {
    static String strip(String s) {
        return s.trim();
    }

    static void parse(Scanner sc, String loc, Map<Character, List<CtorArgFreshLocalRenameRule>> rules, Map<Character, Character> folds) {
        int line = 0;
        boolean multiline = false;
        while (sc.hasNextLine()) {
            line++;
            String rawLine = sc.nextLine();
            String content = rawLine;
            if (multiline) {
                if (content.endsWith("*/")) {
                    multiline = false;
                }
                continue;
            }
            if (content.startsWith("/*")) {
                multiline = true;
            } else {
                int cmt = content.indexOf("//");
                if (cmt >= 0) {
                    content = content.substring(0, cmt);
                }
                content = content.trim();
                if (content.length() == 0) {
                    continue;
                }
                if (content.contains("=")) {
                    String[] parts = content.split("=");
                    if (parts.length != 2) {
                        throw new IllegalArgumentException("bad fold: " + rawLine + " in " + loc);
                    }
                    String from = parts[0];
                    String to = parts[1];
                    if (from.length() != 1 || to.length() != 1) {
                        throw new IllegalArgumentException("bad chars: " + rawLine + " in " + loc);
                    }
                    folds.put(from.charAt(0), to.charAt(0));
                } else {
                    String[] parts = content.split("\\s+");
                    if (parts.length != 4) {
                        throw new IllegalArgumentException("bad rule: " + parts.length + " in " + loc);
                    }
                    try {
                        String p = strip(parts[0]);
                        String q = strip(parts[1]);
                        String r = strip(parts[2]);
                        String s = strip(parts[3]);
                        CtorArgFreshLocalRenameRule rule = new CtorArgFreshLocalRenameRule(p, q, r, s);
                        char key = p.charAt(0);
                        List<CtorArgFreshLocalRenameRule> lst = rules.get(key);
                        if (lst == null) {
                            lst = new ArrayList<CtorArgFreshLocalRenameRule>();
                            rules.put(key, lst);
                        }
                        lst.add(rule);
                    } catch (IllegalArgumentException e) {
                        throw new IllegalStateException("Problem parsing line '" + line + "' in " + loc, e);
                    }
                }
            }
        }
    }

    public static void main(String[] args) {
        Map<Character, List<CtorArgFreshLocalRenameRule>> rules = new LinkedHashMap<Character, List<CtorArgFreshLocalRenameRule>>();
        Map<Character, Character> folds = new LinkedHashMap<Character, Character>();
        String input = "// comment\n"
                + "a=b\n"
                + "aa xx yy zz\n"
                + "ab pp qq rr\n"
                + "/* block\n"
                + "still block */\n"
                + "ba 11 22 33\n";
        parse(new Scanner(input), "fixture", rules, folds);
        StringBuilder sb = new StringBuilder();
        sb.append("folds=").append(folds);
        for (Map.Entry<Character, List<CtorArgFreshLocalRenameRule>> e : rules.entrySet()) {
            sb.append("|").append(e.getKey()).append(":").append(e.getValue());
        }
        System.out.println(sb.toString());
    }
}
