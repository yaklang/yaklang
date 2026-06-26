import java.io.IOException;

public class Exceptions {
    public int simpleTryCatch(int a, int b) {
        try {
            return a / b;
        } catch (ArithmeticException e) {
            return -1;
        }
    }

    public int tryCatchFinally(int[] arr, int i) {
        int r = 0;
        try {
            r = arr[i];
        } catch (ArrayIndexOutOfBoundsException e) {
            r = -1;
        } finally {
            r += 100;
        }
        return r;
    }

    public String multiCatch(Object o) {
        try {
            return o.toString();
        } catch (NullPointerException | ClassCastException e) {
            return "bad";
        }
    }

    public void throwIt(int n) throws IOException {
        if (n < 0) {
            throw new IOException("negative");
        }
    }

    public int nestedTry(String s) {
        int r = 0;
        try {
            try {
                r = Integer.parseInt(s);
            } finally {
                r += 1;
            }
        } catch (NumberFormatException e) {
            r = -1;
        }
        return r;
    }
}
