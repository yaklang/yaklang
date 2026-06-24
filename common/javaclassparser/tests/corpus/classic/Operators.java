public class Operators {
    public int arithmetic(int a, int b) {
        int r = a + b;
        r = r - b;
        r = r * 2;
        r = r / 3;
        r = r % 7;
        return r;
    }

    public int bitwise(int a, int b) {
        int r = a & b;
        r = r | b;
        r = r ^ b;
        r = ~r;
        r = r << 2;
        r = r >> 1;
        r = r >>> 1;
        return r;
    }

    public int compound(int a) {
        a += 5;
        a -= 2;
        a *= 3;
        a /= 2;
        a %= 4;
        a &= 6;
        a |= 1;
        a ^= 3;
        a <<= 1;
        a >>= 1;
        a >>>= 1;
        return a;
    }

    public int incDec(int a) {
        a++;
        ++a;
        a--;
        --a;
        return a + (a++) - (--a);
    }

    public boolean logic(boolean x, boolean y) {
        return (x && y) || (!x ^ y);
    }

    public long widen(int a) {
        long l = a;
        double d = l * 1.5;
        return (long) d;
    }
}
