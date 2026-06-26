// Numeric boundary corpus: integer overflow wrap, shift counts at/over the type
// width, mixed int/long/byte/short/char promotion, compound assignment with implicit
// narrowing, hex/binary/octal/underscore literals, char arithmetic and float/double
// special values. Exercises operand typing, literal rendering and narrowing-cast
// reconstruction independent of control flow.
public class NumericEdge {
    public int overflowWrap(int a) {
        int r = a * 31 + 17;
        r = r * r;
        r += Integer.MAX_VALUE;
        r -= Integer.MIN_VALUE;
        return r;
    }

    public long mixedWidth(int a, long b, short c, byte d) {
        long r = (long) a + b;
        r += c;
        r -= d;
        r *= 3L;
        r &= 0xFFFFFFFFL;
        return r;
    }

    public int shiftCounts(int a) {
        int r = a << 1;
        r = r >> 2;
        r = r >>> 3;
        r = a << 31;
        r = a >> 31;
        r = a >>> 31;
        return r;
    }

    public long longShift(long a) {
        long r = a << 1;
        r = r >> 2;
        r = r >>> 3;
        r = a << 63;
        return r;
    }

    public byte narrowing(int a) {
        byte b = (byte) a;
        b += 1;
        short s = (short) (a + b);
        char c = (char) (s + 1);
        return (byte) (c - s);
    }

    public int literals() {
        int hex = 0xDEADBEEF;
        int bin = 0b1010_1010;
        int oct = 0777;
        int und = 1_000_000;
        return hex ^ bin ^ oct ^ und;
    }

    public long longLiterals() {
        long hex = 0xCAFEBABEL;
        long big = 9_223_372_036_854_775_807L;
        return hex + big;
    }

    public int charArithmetic(char ch) {
        int sum = 0;
        for (char c = 'a'; c <= ch; c++) {
            sum += c - 'a';
        }
        return sum;
    }

    public double floatSpecials(double x) {
        double r = x / 0.0;
        if (Double.isNaN(r)) {
            r = 0.0;
        }
        r += Double.MAX_VALUE;
        r -= Double.MIN_VALUE;
        return r * 2.5e-3;
    }

    public float floatPromotion(int a, long b, float c) {
        float r = a + c;
        r += b;
        r *= 1.5f;
        return r;
    }

    public int compoundNarrow(int a) {
        byte b = 10;
        b *= a;
        b += 100;
        b <<= 1;
        return b;
    }
}
