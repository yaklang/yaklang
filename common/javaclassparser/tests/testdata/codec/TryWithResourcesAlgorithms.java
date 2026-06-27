package codec;

import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.StringReader;

/**
 * TryWithResourcesAlgorithms - a self-hosted battery for the try-with-resources desugaring, which
 * javac lowers into a nested try/catch/finally with a synthetic `Throwable` primary-exception local,
 * a null guard on the resource, and a suppressed-exception `addSuppressed` inner try/catch:
 *
 *     A a = open();
 *     Throwable primary = null;
 *     try { body }
 *     catch (Throwable t) { primary = t; throw t; }
 *     finally {
 *         if (a != null) {
 *             if (primary != null) { try { a.close(); } catch (Throwable s) { primary.addSuppressed(s); } }
 *             else { a.close(); }
 *         }
 *     }
 *
 * This is one of the densest control-flow shapes javac emits (nested exception tables, a primary-vs-
 * suppressed branch, resource null-check) and a classic decompiler failure point distinct from the
 * plain try/catch/finally cross-arm battery. Every divergence - a dropped close, an inverted
 * primary/suppressed branch, a mis-scoped Throwable temp, a lost checked-exception declaration -
 * changes the fingerprint, so the differential oracle catches it.
 *
 * Scope: this battery exercises single-resource, two-resources-in-one-try (reverse-order close),
 * try-with-resources + explicit catch, and the two shapes that used to be open structuring defects
 * and now lock the fix: (1) a try-with-resources whose body is a loop whose NORMAL exit must flow to
 * a post-try `finally` that then returns AFTER the loop (readLinesGuarded), and (2) a NESTED
 * try-with-resources whose outer body is a loop containing an inner try-with-resources
 * (nestedResources). Both used to mis-structure because the loop structurer treated the synthetic
 * exception-handler edge as a normal loop exit - converting it into a `break` (dropping the catch and
 * leaking the caught-exception placeholder as a bare `Exception`) or fabricating a spurious second
 * loop exit that collapsed `loopEnd` to the method's shared `return`, so the loop lost its `break` and
 * spun forever. Resources are JDK AutoCloseable types so the battery stays a single public top-level
 * class with no inner/extra top-level classes. Methods propagate IOException to also exercise
 * checked-exception declaration reconstruction. Deterministic fingerprint in main().
 *
 * Opcode intent: athrow + multi-region exception tables, aconst_null + ifnonnull/ifnull resource and
 * primary guards, invokevirtual close()/addSuppressed(), invokevirtual read()/readLine(), astore
 * of caught Throwable, byte[]/char[] loops.
 */
public class TryWithResourcesAlgorithms {

    // ---- single resource: sum all bytes read from a ByteArrayInputStream, return inside the try ----
    public static int sumBytes(byte[] data) throws IOException {
        int sum = 0;
        try (ByteArrayInputStream in = new ByteArrayInputStream(data)) {
            int b;
            while ((b = in.read()) != -1) {
                sum += b;
            }
            return sum;
        }
    }

    // ---- single resource with an in-body branch; return inside the try ----
    public static int sumEvenBytes(byte[] data) throws IOException {
        int sum = 0;
        int count = 0;
        try (ByteArrayInputStream in = new ByteArrayInputStream(data)) {
            int b;
            while ((b = in.read()) != -1) {
                if ((b & 1) == 0) {
                    sum += b;
                    count++;
                }
            }
            return sum * 31 + count;
        }
    }

    // ---- write side: ByteArrayOutputStream, return a checksum of the written bytes inside the try ----
    public static int writeChecksum(int n) throws IOException {
        try (ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            for (int i = 0; i < n; i++) {
                out.write((i * 37 + 11) & 0xff);
            }
            byte[] arr = out.toByteArray();
            int crc = 0;
            for (int i = 0; i < arr.length; i++) {
                crc = (crc * 131 + (arr[i] & 0xff)) & 0x7fffffff;
            }
            return crc;
        }
    }

    // ---- two resources in ONE try (a = ...; b = ...): two nested close()s, reverse close order ----
    public static int copyThroughTwo(byte[] data) throws IOException {
        try (ByteArrayInputStream in = new ByteArrayInputStream(data);
             ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            int b;
            while ((b = in.read()) != -1) {
                out.write((b + 1) & 0xff);
            }
            byte[] copy = out.toByteArray();
            int acc = 0;
            for (int i = 0; i < copy.length; i++) {
                acc = acc * 17 + (copy[i] & 0xff);
            }
            return acc;
        }
    }

    // ---- try-with-resources plus an explicit catch; both arms return from inside the try/catch ----
    public static int readLinesCatch(String text) {
        int total = 0;
        int lines = 0;
        try (BufferedReader r = new BufferedReader(new StringReader(text))) {
            String line;
            while ((line = r.readLine()) != null) {
                lines++;
                for (int i = 0; i < line.length(); i++) {
                    total += line.charAt(i);
                }
            }
            return total * 100 + lines;
        } catch (IOException e) {
            return -1;
        }
    }

    // ---- try-with-resources + explicit catch + finally, loop body, normal exit flows to the finally
    // and RETURNS after the loop (Bug U second form regression lock): the loop's normal exit must emit
    // a break, then the implicit resource close + the explicit finally run, then the method returns. ----
    public static int readLinesGuarded(String text) {
        int total = 0;
        int lines = 0;
        try (BufferedReader r = new BufferedReader(new StringReader(text))) {
            String line;
            while ((line = r.readLine()) != null) {
                lines++;
                for (int i = 0; i < line.length(); i++) {
                    total += line.charAt(i);
                }
            }
        } catch (IOException e) {
            total = -1;
        } finally {
            total = total * 100 + lines;
        }
        return total;
    }

    // ---- NESTED try-with-resources whose outer body is a loop containing an inner try-with-resources
    // (Bug U first form regression lock): the outer catch handler must stay bound as `catch (Throwable)`
    // and must not be rewritten into a loop `break`, otherwise the caught-exception placeholder leaks
    // as a bare `Exception` token and the output no longer compiles. ----
    public static int nestedResources(String text) throws IOException {
        int fp = 7;
        try (BufferedReader r = new BufferedReader(new StringReader(text))) {
            String line;
            while ((line = r.readLine()) != null) {
                byte[] bytes = line.getBytes(java.nio.charset.StandardCharsets.UTF_8);
                try (ByteArrayInputStream in = new ByteArrayInputStream(bytes)) {
                    int b;
                    int s = 0;
                    while ((b = in.read()) != -1) {
                        s += b;
                    }
                    fp = fp * 31 + s;
                }
            }
        }
        return fp;
    }

    public static void main(String[] args) throws IOException {
        byte[] data = new byte[64];
        for (int i = 0; i < data.length; i++) {
            data[i] = (byte) ((i * 7 + 3) & 0xff);
        }
        String text = "hello\nworld\ntry-with-resources\n\nlast";

        StringBuilder sb = new StringBuilder();
        sb.append(sumBytes(data)).append(',');
        sb.append(sumEvenBytes(data)).append(',');
        sb.append(writeChecksum(50)).append(',');
        sb.append(copyThroughTwo(data)).append(',');
        sb.append(readLinesCatch(text)).append(',');
        sb.append(readLinesGuarded(text)).append(',');
        sb.append(nestedResources(text));
        System.out.println(sb);
    }
}
