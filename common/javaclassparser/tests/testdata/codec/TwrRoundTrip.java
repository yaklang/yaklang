package codec;

import java.io.ByteArrayInputStream;
import java.io.IOException;

// TwrRoundTrip exercises javac's try-with-resources desugaring: the synthetic `Throwable` primary-exception
// slot, the nested close()/catch scaffold, and the `addSuppressed(Throwable)` call site. Bytecode stores
// the primary-exception local in an int-shaped reference slot whose type must stay Throwable for
// addSuppressed to resolve; this battery guards that the decompiled source keeps that typing and the
// resource-close ordering, with a deterministic fingerprint to also catch behavioral drift.
public class TwrRoundTrip {
    static int sumStream(byte[] data) throws IOException {
        int sum = 0;
        String tag = "x";
        try (ByteArrayInputStream in = new ByteArrayInputStream(data)) {
            int b;
            while ((b = in.read()) != -1) {
                sum += b;
                tag = tag + b;
            }
        }
        return sum + tag.length();
    }

    static int twoResources(byte[] a, byte[] b) throws IOException {
        int sum = 0;
        try (ByteArrayInputStream in1 = new ByteArrayInputStream(a);
             ByteArrayInputStream in2 = new ByteArrayInputStream(b)) {
            int x;
            while ((x = in1.read()) != -1) {
                sum += x;
            }
            while ((x = in2.read()) != -1) {
                sum -= x;
            }
        }
        return sum;
    }

    public static void main(String[] args) throws IOException {
        long fp = 1469598103934665603L;
        fp = (fp ^ (long) sumStream(new byte[]{1, 2, 3, 4, 5})) * 1099511628211L;
        fp = (fp ^ (long) twoResources(new byte[]{10, 20}, new byte[]{1, 2, 3})) * 1099511628211L;
        System.out.println("fp=" + Long.toHexString(fp));
    }
}
