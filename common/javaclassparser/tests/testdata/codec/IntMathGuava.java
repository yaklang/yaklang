package codec;

// Self-hosted guava IntMath/LongMath-style integer algorithm battery: gcd, floor/ceil log2, floor
// sqrt (Newton), integer pow, trial-division primality, and checked overflow arithmetic. Exercises
// int and long arithmetic, unsigned shifts (>>>), bit tricks (Integer.numberOfLeadingZeros /
// highestOneBit), nested loops, and exception-throwing overflow branches in one self-verifying class.
public class IntMathGuava {
    static int gcd(int a, int b) {
        a = Math.abs(a);
        b = Math.abs(b);
        while (b != 0) {
            int t = b;
            b = a % b;
            a = t;
        }
        return a;
    }

    static int log2Floor(int x) {
        if (x <= 0) {
            throw new IllegalArgumentException("x must be positive");
        }
        return 31 - Integer.numberOfLeadingZeros(x);
    }

    static int log2Ceil(int x) {
        int floor = log2Floor(x);
        if ((x & (x - 1)) == 0) {
            return floor;
        }
        return floor + 1;
    }

    static int sqrtFloor(int x) {
        if (x < 0) {
            throw new IllegalArgumentException("negative");
        }
        if (x == 0) {
            return 0;
        }
        int guess = (int) Math.sqrt((double) x);
        // correct the double rounding both directions
        while ((long) guess * guess > (long) x) {
            guess--;
        }
        while ((long) (guess + 1) * (guess + 1) <= (long) x) {
            guess++;
        }
        return guess;
    }

    static long pow(int base, int exp) {
        long result = 1L;
        long b = base;
        while (exp > 0) {
            if ((exp & 1) == 1) {
                result *= b;
            }
            b *= b;
            exp >>>= 1;
        }
        return result;
    }

    static boolean isPrime(int n) {
        if (n < 2) {
            return false;
        }
        if (n % 2 == 0) {
            return n == 2;
        }
        for (int i = 3; (long) i * i <= (long) n; i += 2) {
            if (n % i == 0) {
                return false;
            }
        }
        return true;
    }

    static long checkedAdd(long a, long b) {
        long r = a + b;
        if (((a ^ r) & (b ^ r)) < 0) {
            throw new ArithmeticException("overflow");
        }
        return r;
    }

    public static void main(String[] args) {
        long acc = 0;
        for (int i = 1; i <= 40; i++) {
            acc = acc * 1000003L + gcd(i * 12, i * 18 + 6);
            acc = acc * 31 + log2Floor(i);
            acc = acc * 31 + log2Ceil(i);
            acc = acc * 31 + sqrtFloor(i * i + 3);
            acc = acc * 31 + pow(i % 5 + 1, i % 7);
            acc = acc * 31 + (isPrime(i) ? 1 : 0);
        }
        int overflowSeen = 0;
        try {
            checkedAdd(Long.MAX_VALUE, 1L);
        } catch (ArithmeticException e) {
            overflowSeen = 1;
        }
        acc = acc * 31 + checkedAdd(123456789L, 987654321L);
        System.out.println("IntMathGuava:" + acc + ":" + overflowSeen);
    }
}
