// Boundary-condition corpus: numeric extremes, cast chains, nested ternaries, bit
// manipulation, multi-dimensional array access and compound assignment. These are
// edge shapes that stress operand typing, literal rendering and precedence without
// depending on generics or lambdas.
public class Boundary {
    public int extremes() {
        int a = Integer.MAX_VALUE;
        int b = Integer.MIN_VALUE;
        long c = Long.MAX_VALUE;
        long d = Long.MIN_VALUE;
        return (int) (((long) a + (long) b + c + d) & 0xFFFF);
    }

    public int signedDivision(int x, int y) {
        if (y == 0) {
            return -1;
        }
        int q = x / y;
        int r = x % y;
        return q * 31 + r;
    }

    public long bitOps(long v) {
        long r = v & 0xF0F0F0F0L;
        r |= 0x0F0F0F0FL;
        r ^= 0xFFFFFFFFL;
        r <<= 3;
        r >>= 2;
        r >>>= 1;
        return ~r;
    }

    public int charArithmetic(char ch) {
        char upper = (char) (ch - 32);
        int code = upper + 1;
        return code & 0xFF;
    }

    public int nestedTernary(int a, int b) {
        return a > b ? (a > 0 ? 1 : 2) : (b > 0 ? 3 : 4);
    }

    public int castChain(double value) {
        long l = (long) value;
        int i = (int) l;
        short s = (short) i;
        byte by = (byte) s;
        return by;
    }

    public int matrixSum(int[][] grid) {
        int sum = 0;
        for (int i = 0; i < grid.length; i++) {
            for (int j = 0; j < grid[i].length; j++) {
                sum += grid[i][j];
            }
        }
        return sum;
    }

    public int compoundArray(int[] arr, int idx) {
        arr[idx] += 10;
        arr[idx] *= 2;
        arr[idx] -= 3;
        return arr[idx];
    }

    public int unaryMix(int a) {
        int b = -a;
        int c = +b;
        b = ~c;
        return -b + (a++) - (--a);
    }

    public int shiftEdges(int v) {
        int r = v << 31;
        r = r >> 31;
        r = r >>> 16;
        return r;
    }
}
