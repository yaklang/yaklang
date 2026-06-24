public class PatternMatching {
    public String describe(Object o) {
        if (o instanceof String s) {
            return "string of length " + s.length();
        } else if (o instanceof Integer i && i > 0) {
            return "positive int " + i;
        }
        return "other";
    }

    public int len(Object o) {
        return o instanceof CharSequence cs ? cs.length() : -1;
    }
}
