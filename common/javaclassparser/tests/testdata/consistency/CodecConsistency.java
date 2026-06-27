import java.lang.reflect.*;
import java.net.*;
import java.util.*;
import java.nio.charset.Charset;

/**
 * CodecConsistency — computational-consistency oracle for the Yak decompiler.
 *
 * For each algorithm class, it loads the ORIGINAL version (from the jar classloader)
 * and the DECOMPILED-then-recompiled version (from the recompiled classloader), then
 * runs the SAME static methods with the SAME inputs on both and compares results.
 * A divergence means the decompiled body computes differently than the original —
 * the decompiler corrupted an expression, dropped a statement, or mis-reconstructed
 * control flow. Zero divergences across many inputs is strong proof of semantic
 * equivalence for these pure-Java algorithm implementations.
 *
 * Usage: java CodecConsistency <originalJar> <recompiledDir>
 */
public class CodecConsistency {
    static int passed = 0, diverged = 0, totalInvocations = 0;

    public static void main(String[] args) throws Exception {
        URL origURL = new URL("file:" + args[0]);
        URL recompURL = new URL("file:" + args[1] + "/");
        ClassLoader origCL = new URLClassLoader(new URL[]{origURL});
        ClassLoader recompCL = new URLClassLoader(new URL[]{recompURL, origURL});

        ifPresent(recompCL, "org.apache.commons.codec.digest.PureJavaCrc32", () -> {
            Class<?> oc = origCL.loadClass("org.apache.commons.codec.digest.PureJavaCrc32");
            Class<?> rc = recompCL.loadClass("org.apache.commons.codec.digest.PureJavaCrc32");
            Method om = oc.getMethod("compute", byte[].class, int.class, int.class);
            Method rm = rc.getMethod("compute", byte[].class, int.class, int.class);
            byte[][] inputs = { new byte[]{}, new byte[]{0}, new byte[]{1,2,3}, new byte[]{-1,0,1},
                "hello world".getBytes(), new byte[256], new byte[1024] };
            for (int i = 0; i < inputs.length; i++) { if(i==2) fill(inputs[i],(byte)('A'+i)); if(i>=5) randFill(inputs[i], i); }
            for (byte[] in : inputs) for (int len : new int[]{0,1,Math.max(1,in.length/2),in.length}) {
                if (len > in.length) continue;
                long o = (Integer)om.invoke(null, in, 0, len) & 0xFFFFFFFFL;
                long r = (Integer)rm.invoke(null, in, 0, len) & 0xFFFFFFFFL;
                check("PureJavaCrc32[len=" + len + "]", o, r);
            }
        });

        ifPresent(recompCL, "org.apache.commons.codec.digest.MurmurHash2", () -> {
            Class<?> oc = origCL.loadClass("org.apache.commons.codec.digest.MurmurHash2");
            Class<?> rc = recompCL.loadClass("org.apache.commons.codec.digest.MurmurHash2");
            Method om = oc.getMethod("hash32", byte[].class, int.class, int.class);
            Method rm = rc.getMethod("hash32", byte[].class, int.class, int.class);
            byte[][] inputs = { new byte[]{}, "hello".getBytes(), "The quick brown fox".getBytes(),
                new byte[100], new byte[1000], new byte[]{-128,127,0,1} };
            randFill(inputs[3], 7); randFill(inputs[4], 13);
            for (byte[] in : inputs) for (int seed : new int[]{0,1,0x9747b28c,-1,42}) {
                int o = (Integer)om.invoke(null, in, in.length, seed);
                int r = (Integer)rm.invoke(null, in, in.length, seed);
                check("MurmurHash2[seed=" + seed + ",len=" + in.length + "]", o, r);
            }
        });

        ifPresent(recompCL, "org.apache.commons.codec.digest.Md5Crypt", () -> {
            Class<?> oc = origCL.loadClass("org.apache.commons.codec.digest.Md5Crypt");
            Class<?> rc = recompCL.loadClass("org.apache.commons.codec.digest.Md5Crypt");
            Method om = oc.getMethod("md5Crypt", byte[].class, String.class);
            Method rm = rc.getMethod("md5Crypt", byte[].class, String.class);
            byte[][] inputs = { "password".getBytes(), "".getBytes(), "a".getBytes(),
                "The quick brown fox jumps over the lazy dog".getBytes(),
                new byte[100], "中文测试".getBytes(Charset.forName("UTF-8")) };
            randFill(inputs[4], 99);
            String[] salts = { "$1$abc", "$1$saltstr", "$1$xxxx", "$1$abcdefgh" };
            for (byte[] in : inputs) for (String salt : salts) {
                String o = (String)om.invoke(null, in, salt);
                String r = (String)rm.invoke(null, in, salt);
                check("Md5Crypt[salt=" + salt + ",len=" + in.length + "]", o, r);
            }
        });

        ifPresent(recompCL, "org.apache.commons.codec.binary.Hex", () -> {
            Class<?> oc = origCL.loadClass("org.apache.commons.codec.binary.Hex");
            Class<?> rc = recompCL.loadClass("org.apache.commons.codec.binary.Hex");
            byte[][] inputs = { new byte[]{}, new byte[]{0}, new byte[]{1,2,3,(byte)0xff,(byte)0xab},
                "hello".getBytes(), new byte[64] };
            randFill(inputs[4], 5);
            for (byte[] in : inputs) {
                char[] oenc = (char[]) oc.getMethod("encodeHex", byte[].class).invoke(oc.getDeclaredConstructor().newInstance(), in);
                char[] renc = (char[]) rc.getMethod("encodeHex", byte[].class).invoke(rc.getDeclaredConstructor().newInstance(), in);
                check("Hex.encodeHex[len=" + in.length + "]", new String(oenc), new String(renc));
            }
        });

        System.out.println("RESULT " + (diverged == 0 ? "PASS" : "FAIL") + " passed=" + passed + " diverged=" + diverged + " invocations=" + totalInvocations);
    }

    interface ThrowingRun { void run() throws Exception; }
    static void ifPresent(ClassLoader cl, String name, ThrowingRun body) {
        try { cl.loadClass(name); } catch (ClassNotFoundException e) { System.out.println("SKIP " + name + " (not recompiled)"); return; }
        try { body.run(); } catch (Throwable t) { System.out.println("ERROR " + name + ": " + t); diverged++; }
    }
    static void check(String label, Object o, Object r) {
        totalInvocations++;
        boolean eq;
        if (o == null || r == null) eq = (o == null && r == null);
        else if (o.getClass().isArray()) eq = Arrays.equals((byte[])o, (byte[])r);
        else eq = o.equals(r);
        if (eq) { passed++; } else { diverged++; System.out.println("DIFF " + label + ": orig=" + o + " recomp=" + r); }
    }
    static void fill(byte[] a, byte v) { for (int i = 0; i < a.length; i++) a[i] = v; }
    static void randFill(byte[] a, int seed) { Random rnd = new Random(seed); rnd.nextBytes(a); }
}
