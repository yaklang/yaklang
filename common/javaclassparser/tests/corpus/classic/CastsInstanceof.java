public class CastsInstanceof {
    public String classify(Object o) {
        if (o instanceof String) {
            return "str:" + ((String) o).toUpperCase();
        } else if (o instanceof Integer) {
            return "int:" + ((Integer) o).intValue();
        } else if (o instanceof Number) {
            return "num:" + ((Number) o).doubleValue();
        }
        return "unknown";
    }

    public int narrow(long l) {
        int i = (int) l;
        short s = (short) i;
        byte b = (byte) s;
        char c = (char) b;
        return c + s + b + i;
    }

    public Object chainCast(Object o) {
        return (Comparable<?>) (CharSequence) o;
    }
}
