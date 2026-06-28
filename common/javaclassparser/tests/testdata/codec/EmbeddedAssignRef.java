package codec;

// Bug Y residual probe: a REFERENCE-typed local that only ever receives its value through an
// embedded assignment inside a condition ((s = next(...)) != null), then is immediately used via a
// member access (s.length()). javac dup-collapses the store so the local has no standalone
// declaration; Yak's string-level safety net historically guessed `Object s = null`, which makes the
// subsequent `s.length()` fail to recompile with `cannot find symbol`. A clean byte-for-byte
// round-trip proves the reference type is recovered (declared as String, not Object).
public class EmbeddedAssignRef {
    static String next(String[] arr, int i) {
        // returns null for empty strings so the != null guard is real
        String v = arr[i];
        if (v.isEmpty()) {
            return null;
        }
        return v;
    }

    static int scan(String[] parts) {
        int total = 0;
        String s;
        for (int i = 0; i < parts.length; i++) {
            // embedded assignment in the condition; s used right after via a reference method call
            if ((s = next(parts, i)) != null) {
                total += s.length() * 31 + s.charAt(0);
            }
        }
        return total;
    }

    static int scanWhile(String[] parts) {
        int total = 0;
        int i = 0;
        String s;
        // a second shape: embedded assign drives the loop condition, body uses the ref local
        while (i < parts.length && (s = parts[i]) != null) {
            if (!s.isEmpty()) {
                total = total * 131 + s.length();
            }
            i++;
        }
        return total;
    }

    public static void main(String[] args) {
        String[] data = {"alpha", "", "beta", "g", "", "delta"};
        int a = scan(data);
        int b = scanWhile(data);
        StringBuilder sb = new StringBuilder();
        sb.append("EmbeddedAssignRef:");
        sb.append(a);
        sb.append(":");
        sb.append(b);
        System.out.println(sb.toString());
    }
}
