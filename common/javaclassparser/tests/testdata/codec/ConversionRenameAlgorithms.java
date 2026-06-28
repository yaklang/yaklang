package codec;

/**
 * ConversionRenameAlgorithms - guards a primitive-cast variable-rename defect found by decompiling
 * guava's UnsignedLongs.toString. A numeric conversion opcode (l2i/i2c/...) builds its cast
 * expression `(int)(operand)` as a lazily-rendered CustomValue, but the CustomValue did NOT forward
 * ReplaceVar to its operand. When a later depth-collision rename renamed the operand variable (long
 * `rem` -> `var4_1` because it collided with `quotient` at the same scope depth), every other
 * occurrence updated but the `(int) rem` cast kept the STALE name `var5`, referencing an undeclared
 * variable ("cannot find symbol"). The fix mirrors CHECKCAST: the conversion CustomValue forwards
 * ReplaceVar to its operand.
 *
 * The toRadix* methods reproduce the exact bytecode shape: a value computed inside a nested else
 * branch and consumed by a primitive cast, forcing the rename to flow through the cast operand.
 *
 * Pure static methods, single public top-level class, deterministic fingerprint in main().
 */
public class ConversionRenameAlgorithms {

    // Mirrors UnsignedLongs.toString: power-of-two radixes use a shift/mask fast path, others use a
    // division loop where the remainder (a long) is consumed via a `(int)` cast in Character.forDigit.
    static String toRadixUnsigned(long x, int radix) {
        if (x == 0L) {
            return "0";
        }
        char[] buf = new char[64];
        int i = buf.length;
        boolean negative = x < 0L;
        if ((radix & (radix - 1)) == 0) {
            int shift = Integer.numberOfTrailingZeros(radix);
            int mask = radix - 1;
            long v = x;
            do {
                i--;
                buf[i] = Character.forDigit(((int) v) & mask, radix);
                v >>>= shift;
            } while (v != 0L);
        } else {
            long quotient;
            if ((radix & 1) == 0) {
                quotient = (x >>> 1) / (long) (radix >>> 1);
            } else if (negative) {
                quotient = x / radix;
            } else {
                quotient = x / radix;
            }
            long rem = x - quotient * radix;
            i--;
            buf[i] = Character.forDigit((int) rem, radix);
            x = quotient;
            while (x > 0L) {
                i--;
                buf[i] = Character.forDigit((int) (x % radix), radix);
                x /= radix;
            }
        }
        return new String(buf, i, buf.length - i);
    }

    // A second, independent shape: a long product narrowed to int and to char in a nested branch, so
    // the cast operand variable also depth-collides with a sibling-branch local.
    static String mixWidths(long a, int b) {
        StringBuilder sb = new StringBuilder();
        if (b == 0) {
            int single = (int) a;
            sb.append(single);
        } else {
            long scaled;
            if ((b & 1) == 0) {
                scaled = a * b;
            } else {
                scaled = a * (b + 1);
            }
            long delta = scaled - a;
            sb.append((int) delta);
            char c = (char) ('A' + (int) (delta % 26));
            sb.append(c);
            byte lo = (byte) scaled;
            sb.append((int) lo);
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        long[] xs = {0L, 1L, 255L, 1000000007L, -1L, -8L, 9223372036854775807L, -9223372036854775808L};
        int[] radixes = {2, 8, 10, 16, 7, 36};
        for (long x : xs) {
            for (int r : radixes) {
                sb.append("u(").append(x).append(',').append(r).append(")=")
                        .append(toRadixUnsigned(x, r)).append(';');
            }
        }
        long[] as = {0L, 5L, 12345L, -7L};
        int[] bs = {0, 2, 3, 10};
        for (long a : as) {
            for (int b : bs) {
                sb.append("m(").append(a).append(',').append(b).append(")=")
                        .append(mixWidths(a, b)).append(';');
            }
        }
        System.out.println(sb.toString());
    }
}
