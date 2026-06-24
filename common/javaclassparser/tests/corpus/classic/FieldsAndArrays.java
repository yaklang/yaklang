// Field and array access corpus: instance and static fields, compound assignment to
// fields and array elements, pre/post increment of fields and array slots, multi-
// dimensional and jagged arrays, static counters and a constant folded into an array
// initializer. Exercises getfield/putfield/getstatic/putstatic and the *aload/*astore
// element-access reconstruction together with compound-assignment recovery.
public class FieldsAndArrays {
    private int counter;
    private long total;
    private static int instances;
    private static final int LIMIT = 8;
    private int[] buffer = new int[LIMIT];
    private int[][] grid = new int[3][4];

    public FieldsAndArrays() {
        instances++;
        this.counter = 0;
        this.total = 0L;
    }

    public int bump() {
        this.counter++;
        this.counter += 5;
        this.total += this.counter;
        return this.counter;
    }

    public void fillBuffer(int base) {
        for (int i = 0; i < this.buffer.length; i++) {
            this.buffer[i] = base + i;
            this.buffer[i] *= 2;
        }
    }

    public int sumBuffer() {
        int sum = 0;
        for (int v : this.buffer) {
            sum += v;
        }
        return sum;
    }

    public void fillGrid() {
        for (int i = 0; i < this.grid.length; i++) {
            for (int j = 0; j < this.grid[i].length; j++) {
                this.grid[i][j] = i * 10 + j;
                this.grid[i][j]++;
            }
        }
    }

    public int gridDiagonal() {
        int acc = 0;
        int n = Math.min(this.grid.length, this.grid[0].length);
        for (int i = 0; i < n; i++) {
            acc += this.grid[i][i];
        }
        return acc;
    }

    public int[] jagged(int rows) {
        int[][] j = new int[rows][];
        for (int i = 0; i < rows; i++) {
            j[i] = new int[i + 1];
            for (int k = 0; k <= i; k++) {
                j[i][k] = k;
            }
        }
        int[] lengths = new int[rows];
        for (int i = 0; i < rows; i++) {
            lengths[i] = j[i].length;
        }
        return lengths;
    }

    public int postPreIncrement(int[] a, int idx) {
        int x = a[idx]++;
        int y = ++a[idx];
        return x + y;
    }

    public static int getInstances() {
        return instances;
    }

    public long getTotal() {
        return this.total;
    }

    public int[] makeInitialized() {
        int[] xs = {LIMIT, LIMIT * 2, LIMIT * 3, counter};
        return xs;
    }
}
