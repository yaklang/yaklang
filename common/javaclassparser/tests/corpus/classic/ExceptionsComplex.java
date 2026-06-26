// Complex exception corpus: nested try/catch/finally, try-with-resources with a
// single and with multiple resources (javac emits suppressed-exception plumbing and
// nested null-checked close() calls), rethrow from a catch, finally that runs after
// a return, and a catch that recovers a value. These stress the exception-table
// grouping and finally-inlining CFG that a single-level try/catch does not.
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;

public class ExceptionsComplex {
    public int nestedTryFinally(int x) {
        int r = 0;
        try {
            r += 1;
            try {
                r += 2;
                if (x < 0) {
                    throw new IllegalArgumentException("neg");
                }
                r += 4;
            } finally {
                r += 8;
            }
            r += 16;
        } catch (IllegalArgumentException e) {
            r += 32;
        } finally {
            r += 64;
        }
        return r;
    }

    public int catchAndRecover(String s) {
        try {
            return Integer.parseInt(s);
        } catch (NumberFormatException e) {
            return -1;
        }
    }

    public int rethrow(int x) {
        try {
            if (x == 0) {
                throw new IllegalStateException("zero");
            }
            return 100 / x;
        } catch (ArithmeticException e) {
            throw new RuntimeException("wrapped", e);
        }
    }

    public int finallyAfterReturn(int x) {
        int[] holder = new int[1];
        try {
            holder[0] = x * 2;
            return holder[0];
        } finally {
            holder[0] = -1;
        }
    }

    public int singleResource(byte[] data) throws IOException {
        int total = 0;
        try (InputStream in = new ByteArrayInputStream(data)) {
            int b;
            while ((b = in.read()) != -1) {
                total += b;
            }
        }
        return total;
    }

    public int multiResource(byte[] a, byte[] b) throws IOException {
        int total = 0;
        try (InputStream first = new ByteArrayInputStream(a);
             InputStream second = new ByteArrayInputStream(b)) {
            total += first.read();
            total += second.read();
        }
        return total;
    }

    public String multiCatchChain(int kind) {
        try {
            if (kind == 1) {
                throw new IllegalArgumentException();
            } else if (kind == 2) {
                throw new IllegalStateException();
            } else if (kind == 3) {
                throw new NullPointerException();
            }
            return "ok";
        } catch (IllegalArgumentException | IllegalStateException e) {
            return "arg-or-state";
        } catch (NullPointerException e) {
            return "npe";
        } finally {
            kind = 0;
        }
    }
}
