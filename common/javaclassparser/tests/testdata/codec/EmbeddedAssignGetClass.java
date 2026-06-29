package codec;

// Differential-execution regression battery for the embedded-assignment getClass() reference type.
//
// Mirrors the EmbeddedAssignRef shape but with an INSTANCE-method-call value (obj.getClass()): the
// local's only definition is the embedded assignment in the loop condition ((c = objs[i].getClass())
// != exclude) and every read (c.getName()/c.getSimpleName()) lives in the loop BODY, so javac
// dup-collapses the store and the local keeps no standalone `Class c = ...;` declaration. The dumper
// must synthesize one. Because the value is `obj.getClass()` (which bareCallRHSRe cannot match, the
// leading `obj.` breaks it), the reference-type recovery must special-case getClass()->Class; a
// default `Object c = null` makes the later Class member access fail to recompile ("cannot find
// symbol"). Mirrors fastjson2 JSONWriter.checkAndWriteTypeName (the objectClass local).
// Kill-switch for the reference-type recovery: JDEC_NO_EMBED_ASSIGN_REF=1.
public class EmbeddedAssignGetClass {

    static int scan(Object[] objs, Class<?> exclude) {
        int total = 0;
        Class<?> c;
        for (int i = 0; i < objs.length; i++) {
            if ((c = objs[i].getClass()) != exclude) {
                total += c.getName().length() * 31 + c.getSimpleName().length();
            }
        }
        return total;
    }

    public static void main(String[] args) {
        Object[] data = {"alpha", Integer.valueOf(7), new java.util.ArrayList<Object>(), "beta", new java.util.HashMap<Object, Object>()};
        int a = scan(data, String.class);
        int b = scan(data, Integer.class);
        StringBuilder sb = new StringBuilder();
        sb.append("EmbeddedAssignGetClass:");
        sb.append(a);
        sb.append(":");
        sb.append(b);
        System.out.println(sb.toString());
    }
}
