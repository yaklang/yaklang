public class ControlFlow {
    public String classify(int n) {
        if (n < 0) {
            return "negative";
        } else if (n == 0) {
            return "zero";
        } else if (n < 10) {
            return "small";
        } else {
            return "large";
        }
    }

    public int ternary(int n) {
        return n > 0 ? (n > 100 ? 2 : 1) : (n < -100 ? -2 : -1);
    }

    public int nested(int a, int b) {
        int r = 0;
        if (a > 0) {
            if (b > 0) {
                r = a + b;
            } else {
                r = a - b;
            }
        }
        return r;
    }
}
