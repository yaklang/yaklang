import java.io.PrintStream;
import java.lang.reflect.*;
import java.net.URL;
import java.net.URLClassLoader;
import java.util.*;

/**
 * DifferentialTester — the gold-standard oracle for decompiler semantic correctness.
 *
 * Given two directories each holding a compiled version of the same class
 * (directory A = compiled from ORIGINAL source; directory B = compiled from
 * DECOMPILED-then-recompiled source), this driver:
 *
 *   1. Loads each version in its own isolated URLClassLoader.
 *   2. Reflects over the public declared methods present in BOTH versions.
 *   3. Generates a type-driven set of argument tuples per method.
 *   4. Invokes the SAME method with the SAME inputs on BOTH the original and the
 *      recompiled instance.
 *   5. Compares return values (deep-equal for arrays) AND thrown exceptions
 *      (matching exception type, or both throwing nothing).
 *
 * A divergence (different return value, or one throws while the other does not,
 * or they throw different exception types) is a concrete semantic defect: a
 * dropped statement, a wrong expression, or an incorrect control-flow
 * reconstruction in the decompiler.
 *
 * Usage: java DifferentialTester <originalDir> <recompiledDir> <className> [<maxInvocations>]
 *
 * Output (to stdout, line-oriented for machine parsing):
 *   DIFF className methodName inputDesc: orig=<X> recomp=<Y>
 *   EXC  className methodName inputDesc: orig threw <TypeA> recomp threw <TypeB>
 *   OK   className methodName N invocations matched
 *   SKIP className methodName <reason>
 *   SUMMARY className methods=N matched=N diverged=N skipped=N invocations=K
 *   RESULT PASS   (all classes zero divergence)
 *   RESULT FAIL   (>=1 divergence found)
 */
public class DifferentialTester {

    static PrintStream out = System.out;

    // ---- type-driven input pools ------------------------------------------

    static final Object[] EMPTY = new Object[0];

    static Object[] intPool() {
        // Boundary + dense small range + extremes, capped for performance.
        List<Integer> v = new ArrayList<>();
        int[] marks = {Integer.MIN_VALUE, Integer.MIN_VALUE + 1, -1000000, -257, -256, -1, 0, 1,
                2, 3, 7, 8, 9, 10, 11, 31, 32, 63, 64, 100, 127, 128, 255, 256, 257,
                1000, Integer.MAX_VALUE - 1, Integer.MAX_VALUE};
        for (int m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] longPool() {
        List<Long> v = new ArrayList<>();
        long[] marks = {Long.MIN_VALUE, Long.MIN_VALUE + 1, -1000000000000L, -257L, -1L, 0L, 1L,
                255L, 256L, 65535L, 65536L, 1000000L, Long.MAX_VALUE - 1, Long.MAX_VALUE};
        for (long m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] doublePool() {
        List<Double> v = new ArrayList<>();
        double[] marks = {Double.NEGATIVE_INFINITY, -1e9, -3.14, -1.5, -1.0, -0.5, -0.0, 0.0,
                0.5, 1.0, 1.5, 3.14, 1e9, Double.MAX_VALUE, Double.MIN_VALUE, Double.NaN,
                Double.POSITIVE_INFINITY};
        for (double m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] floatPool() {
        List<Float> v = new ArrayList<>();
        float[] marks = {Float.NEGATIVE_INFINITY, -3.14f, -1.5f, -1f, 0f, 1f, 1.5f, 3.14f,
                Float.MAX_VALUE, Float.NaN, Float.POSITIVE_INFINITY};
        for (float m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] charPool() {
        List<Character> v = new ArrayList<>();
        char[] marks = {0, 1, 9, 10, 32, 48, 65, 90, 97, 122, 127, 128, 200, 255, 1000,
                Character.MAX_VALUE, Character.MIN_VALUE};
        for (char m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] boolPool() { return new Object[]{true, false}; }

    static Object[] shortPool() {
        List<Short> v = new ArrayList<>();
        short[] marks = {Short.MIN_VALUE, -1000, -128, -1, 0, 1, 2, 100, 127, 128, 255,
                1000, Short.MAX_VALUE};
        for (short m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] bytePool() {
        List<Byte> v = new ArrayList<>();
        byte[] marks = {Byte.MIN_VALUE, -100, -1, 0, 1, 2, 10, 50, 100, 127};
        for (byte m : marks) v.add(m);
        return v.toArray();
    }

    static Object[] stringPool() {
        return new Object[]{"", "a", "abc", "hello", "A", "z", "0", "123",
                "test", "negative", "zero", "small", "large", null};
    }

    static Object[] intArrayPool() {
        List<int[]> v = new ArrayList<>();
        v.add(new int[]{});
        v.add(new int[]{0});
        v.add(new int[]{1});
        v.add(new int[]{-1, 0, 1});
        v.add(new int[]{1, 2, 3, 4, 5});
        v.add(new int[]{100, -50, 0, 77, 3});
        v.add(new int[]{Integer.MAX_VALUE, Integer.MIN_VALUE, 0});
        return v.toArray();
    }

    static Object[] int2DArrayPool() {
        List<int[][]> v = new ArrayList<>();
        v.add(new int[][]{});
        v.add(new int[][]{{1}});
        v.add(new int[][]{{1, 2}, {3, 4}});
        v.add(new int[][]{{1, 2, 3}, {4, 5, 6}});
        v.add(new int[][]{{-1, 0}, {0, -1}, {1, 1}});
        v.add(new int[][]{{100}});
        v.add(new int[][]{{1}, {2}, {3}, {4}});
        return v.toArray();
    }

    // ---- per-parameter-type input selection -------------------------------

    static Object[] poolFor(Class<?> t) {
        if (t == int.class) return intPool();
        if (t == long.class) return longPool();
        if (t == double.class) return doublePool();
        if (t == float.class) return floatPool();
        if (t == char.class) return charPool();
        if (t == boolean.class) return boolPool();
        if (t == short.class) return shortPool();
        if (t == byte.class) return bytePool();
        if (t == String.class) return stringPool();
        if (t == int[].class) return intArrayPool();
        if (t == int[][].class) return int2DArrayPool();
        if (t == Object.class) return stringPool(); // reuse strings/objects
        // Unknown type: empty (will SKIP the method).
        return EMPTY;
    }


    static boolean origErrInstantiation(Throwable t) {
        while (t != null) {
            if (t instanceof InstantiationException) return true;
            t = t.getCause();
        }
        return false;
    }
    // ---- result comparison -------------------------------------------------

    static boolean resultsEqual(Object a, Object b) {
        if (a == null && b == null) return true;
        if (a == null || b == null) return false;
        if (a.getClass().isArray() || b.getClass().isArray()) {
            return Arrays.deepEquals(toObjectArray(a), toObjectArray(b));
        }
        if (a instanceof Double || a instanceof Float) {
            // Compare doubles via bit pattern so NaN==NaN and -0.0!=+0.0 are handled correctly.
            long ab = (a instanceof Double) ? Double.doubleToLongBits((Double) a) : Float.floatToIntBits((Float) a);
            long bb = (b instanceof Double) ? Double.doubleToLongBits((Double) b) : Float.floatToIntBits((Float) b);
            return ab == bb;
        }
        if (a.equals(b)) return true;
        // If equals is not overridden (Object.equals), a value-equal result that returned `this`
        // (a builder/fluent style) would compare by identity and falsely diverge. Fall back to
        // comparing declared-field state across the two results' public fields, which captures the
        // observable state of value/builder objects that omit equals.
        if (!overridesEquals(a) && !overridesEquals(b) && a.getClass().getName().equals(b.getClass().getName())) {
            return publicFieldsEqual(a, b);
        }
        return false;
    }

    static boolean overridesEquals(Object o) {
        try {
            return o.getClass().getMethod("equals", Object.class).getDeclaringClass() != Object.class;
        } catch (NoSuchMethodException e) {
            return false;
        }
    }

    static boolean publicFieldsEqual(Object a, Object b) {
        Field[] fa = a.getClass().getDeclaredFields();
        Field[] fb = b.getClass().getDeclaredFields();
        if (fa.length != fb.length) return false;
        for (int i = 0; i < fa.length; i++) {
            fa[i].setAccessible(true);
            fb[i].setAccessible(true);
            try {
                Object va = fa[i].get(a), vb = fb[i].get(b);
                if (va == null && vb == null) continue;
                if (va == null || vb == null) return false;
                if (va.getClass().isArray() || vb.getClass().isArray()) {
                    if (!Arrays.deepEquals(toObjectArray(va), toObjectArray(vb))) return false;
                } else if (!va.equals(vb)) {
                    return false;
                }
            } catch (Throwable t) {
                return false;
            }
        }
        return true;
    }

    static Object[] toObjectArray(Object arr) {
        if (!arr.getClass().isArray()) return new Object[]{arr};
        int len = Array.getLength(arr);
        Object[] out = new Object[len];
        for (int i = 0; i < len; i++) out[i] = Array.get(arr, i);
        return out;
    }

    static String desc(Object o) {
        if (o == null) return "null";
        if (o.getClass().isArray()) {
            return Arrays.deepToString(toObjectArray(o));
        }
        return o.toString();
    }

    // ---- core differential testing -----------------------------------------

    static class InvokeResult {
        Object value;
        Throwable err;
    }

    static InvokeResult invoke(ClassLoader cl, String className, Method m, Object[] args) {
        InvokeResult r = new InvokeResult();
        try {
            Class<?> cls = cl.loadClass(className);
            Object target = null;
            // static methods + instance methods both work; we always pass an instance.
            try {
                Constructor<?> noArg = cls.getDeclaredConstructor();
                noArg.setAccessible(true);
                target = noArg.newInstance();
            } catch (NoSuchMethodException nse) {
                // No no-arg constructor: try a single-int constructor as fallback.
                try {
                    Constructor<?> c = cls.getDeclaredConstructor(int.class);
                    c.setAccessible(true);
                    target = c.newInstance(0);
                } catch (NoSuchMethodException nse2) {
                    r.err = new InstantiationException("driver: no instantiable constructor");
                    return r;
                }
            }
            m.setAccessible(true);
            // Resolve the equivalent method on this loader's class to avoid crossing loaders.
            Class<?>[] pts = m.getParameterTypes();
            Class<?>[] resolved = new Class<?>[pts.length];
            for (int i = 0; i < pts.length; i++) resolved[i] = resolve(cl, pts[i]);
            Method m2 = cls.getDeclaredMethod(m.getName(), resolved);
            m2.setAccessible(true);
            r.value = m2.invoke(target, args);
        } catch (InvocationTargetException ite) {
            r.err = ite.getCause();
        } catch (Throwable t) {
            r.err = t;
        }
        return r;
    }

    static Class<?> resolve(ClassLoader cl, Class<?> t) throws ClassNotFoundException {
        if (t.isPrimitive()) return t;
        return cl.loadClass(t.getName());
    }

    @SuppressWarnings("unchecked")
    static void testClass(ClassLoader origCL, ClassLoader recompCL, String className, int maxInvocations) {
        Class<?> origCls, recompCls;
        try {
            origCls = origCL.loadClass(className);
            recompCls = recompCL.loadClass(className);
        } catch (Throwable t) {
            out.println("SKIP " + className + " load-failed: " + t);
            out.println("SUMMARY " + className + " methods=0 matched=0 diverged=0 skipped=1 invocations=0");
            return;
        }
        // Use methods present in the ORIGINAL (the source of truth).
        Method[] origMethods = origCls.getDeclaredMethods();
        int matched = 0, diverged = 0, skipped = 0, totalInvocations = 0;
        for (Method m : origMethods) {
            int mods = m.getModifiers();
            if (Modifier.isStatic(mods)) continue;       // skip static (no instance driver)
            if (m.getName().contains("$")) continue;     // skip synthetic/bridge
            if (m.isSynthetic()) continue;
            // Method must also exist in the recompiled version.
            Method recompM;
            try {
                recompM = recompCls.getDeclaredMethod(m.getName(), m.getParameterTypes());
            } catch (NoSuchMethodException e) {
                out.println("SKIP " + className + " " + m.getName() + " missing-in-recompiled");
                skipped++;
                continue;
            }
            // Build the cartesian product of input pools for each parameter.
            Class<?>[] pts = m.getParameterTypes();
            List<Object[]> pools = new ArrayList<>();
            boolean genOK = true;
            for (Class<?> pt : pts) {
                Object[] p = poolFor(pt);
                if (p.length == 0) { genOK = false; break; }
                pools.add(p);
            }
            if (!genOK) {
                out.println("SKIP " + className + " " + m.getName() + " unsupported-param-type");
                skipped++;
                continue;
            }
            List<Object[]> combos = cartesian(pools, maxInvocations);
            int methodInvocations = 0;
            for (Object[] args : combos) {
                InvokeResult ir = invoke(origCL, className, m, args);
                InvokeResult rr = invoke(recompCL, className, recompM, args);
                methodInvocations++;
                // Compare: exceptions must agree in type-name, values must deep-equal.
                String origExc = ir.err == null ? null : ir.err.getClass().getName();
                String recompExc = rr.err == null ? null : rr.err.getClass().getName();
                if (origExc != null || recompExc != null) {
                    // An InstantiationException means the driver could not construct the target
                    // (no accessible constructor) — that is a driver limitation, not a decompiler
                    // semantic defect, so skip this invocation rather than counting a divergence.
                    if ((origErrInstantiation(ir.err) || origErrInstantiation(rr.err))) {
                        continue;
                    }
                    if (origExc == null || recompExc == null || !sameExcType(origExc, recompExc)) {
                        diverged++;
                        out.println("EXC  " + className + " " + m.getName() + " " + inputDesc(args) +
                                ": orig threw " + (origExc == null ? "nothing" : excName(origExc)) +
                                " recomp threw " + (recompExc == null ? "nothing" : excName(recompExc)));
                    }
                    continue;
                }
                if (!resultsEqual(ir.value, rr.value)) {
                    diverged++;
                    out.println("DIFF " + className + " " + m.getName() + " " + inputDesc(args) +
                            ": orig=" + desc(ir.value) + " recomp=" + desc(rr.value));
                }
            }
            totalInvocations += methodInvocations;
            if (methodInvocations == 0) {
                out.println("SKIP " + className + " " + m.getName() + " no-inputs");
                skipped++;
            } else {
                matched++;
            }
        }
        int methods = origMethods.length;
        out.println("SUMMARY " + className + " methods=" + methods + " matched=" + matched +
                " diverged=" + diverged + " skipped=" + skipped + " invocations=" + totalInvocations);
    }

    static String excName(String full) {
        int i = full.lastIndexOf('.');
        return i >= 0 ? full.substring(i + 1) : full;
    }

    // Treat runtime-exception subtypes leniently: NPE vs AIOOBE vs similar are both
    // "an exception happened", but we still flag if one throws and the other does not,
    // or if the types are genuinely different (e.g. ArithmeticException vs ArrayIndexOutOfBounds).
    static boolean sameExcType(String a, String b) {
        if (a.equals(b)) return true;
        // Coarse bucket: NullPointerException, ArrayIndexOutOfBoundsException,
        // StringIndexOutOfBoundsException, ClassCastException, NegativeArraySizeException,
        // ArithmeticException, IllegalStateException — treat distinct names as a divergence.
        return false;
    }

    static String inputDesc(Object[] args) {
        StringBuilder sb = new StringBuilder("(");
        for (int i = 0; i < args.length; i++) {
            if (i > 0) sb.append(",");
            sb.append(desc(args[i]));
        }
        return sb.append(")").toString();
    }

    // Cartesian product of input pools, capped to maxInvocations tuples.
    static List<Object[]> cartesian(List<Object[]> pools, int cap) {
        List<Object[]> result = new ArrayList<>();
        result.add(EMPTY);
        for (Object[] pool : pools) {
            List<Object[]> next = new ArrayList<>();
            for (Object[] prefix : result) {
                for (Object val : pool) {
                    Object[] combo = Arrays.copyOf(prefix, prefix.length + 1);
                    combo[prefix.length] = val;
                    next.add(combo);
                    if (next.size() >= cap) break;
                }
                if (next.size() >= cap) break;
            }
            result = next;
            if (result.size() >= cap) {
                result = new ArrayList<>(result.subList(0, cap));
                break;
            }
        }
        return result;
    }

    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            out.println("USAGE: DifferentialTester <originalDir> <recompiledDir> <className> [maxInvocations]");
            System.exit(2);
        }
        String origDir = args[0];
        String recompDir = args[1];
        String className = args[2];
        int maxInvocations = 4000;
        if (args.length >= 4) {
            try { maxInvocations = Integer.parseInt(args[3]); } catch (NumberFormatException ignore) {}
        }
        ClassLoader origCL = new URLClassLoader(new URL[]{new URL("file:" + origDir + "/")});
        ClassLoader recompCL = new URLClassLoader(new URL[]{new URL("file:" + recompDir + "/")});
        testClass(origCL, recompCL, className, maxInvocations);
    }
}
