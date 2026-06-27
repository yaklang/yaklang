package codec;

/**
 * ForEachRepro - isolates foreach-over-array decompilation. Three shapes:
 *   sumLiteral: foreach directly over an array LITERAL stored in a local
 *   sumMethod:  foreach over an array returned by a method (the common shape)
 *   sumField:   foreach over a static field array
 * All deterministic; the round-trip oracle asserts the decompiler aliases the iterated array
 * instead of re-creating (or re-evaluating) the source expression.
 */
public class ForEachRepro {

    private static final int[] FIELD = {10, 20, 30, 40};

    private static int[] make() {
        return new int[]{1, 2, 3, 4, 5};
    }

    public static long sumLiteral() {
        int[] xs = {7, 8, 9};
        long s = 0;
        for (int x : xs) s += x;
        return s;
    }

    public static long sumMethod() {
        long s = 0;
        for (int x : make()) s += x * 2L;
        return s;
    }

    public static long sumField() {
        long s = 0;
        for (int x : FIELD) s += x;
        return s;
    }

    // 2D array literal then foreach over the rows
    public static long sum2DForEach() {
        int[][] grid = {{1, 2}, {3, 4}, {5, 6}};
        long s = 0;
        for (int[] row : grid) {
            s += row[0] * 10L + row[1];
        }
        return s;
    }

    // 2D array literal then INDEXED access (no foreach)
    public static long sum2DIndexed() {
        int[][] grid = {{1, 2}, {3, 4}, {5, 6}};
        long s = 0;
        for (int i = 0; i < grid.length; i++) {
            s += grid[i][0] * 10L + grid[i][1];
        }
        return s;
    }

    // Multiple 2D foreach loops over freshly-built double/float literals in ONE method, mirroring the
    // shape that exposed a foreach-copy duplication bug when many array slots are reused in a single
    // method body.
    public static String multiForEachOneMethod() {
        StringBuilder sb = new StringBuilder();
        double[][] dpairs = {{3.5, 1.25}, {-2.0, 7.0}, {0.0, -0.0}};
        for (double[] p : dpairs) {
            sb.append(p[0] + p[1]).append(",");
        }
        float[][] fpairs = {{3.5f, 1.25f}, {-2.0f, 7.0f}};
        for (float[] p : fpairs) {
            sb.append(p[0] - p[1]).append(",");
        }
        long[][] lpairs = {{5, 3}, {7, 7}};
        for (long[] p : lpairs) {
            sb.append(p[0] * p[1]).append(",");
        }
        return sb.toString();
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder();
        sb.append(sumLiteral()).append(",");
        sb.append(sumMethod()).append(",");
        sb.append(sumField()).append(",");
        sb.append(sum2DForEach()).append(",");
        sb.append(sum2DIndexed()).append(",");
        sb.append(multiForEachOneMethod());
        System.out.println(sb);
    }
}
