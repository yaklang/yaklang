public class Arrays {
    public int[] oneDim(int n) {
        int[] a = new int[n];
        a[0] = 1;
        a[1] = 2;
        return a;
    }

    public int[][] twoDim() {
        int[][] a = new int[3][4];
        a[0][0] = 9;
        return a;
    }

    public int[][] jagged() {
        int[][] a = new int[3][];
        a[0] = new int[]{1, 2};
        a[1] = new int[]{3, 4, 5};
        return a;
    }

    public int[] literal() {
        return new int[]{10, 20, 30, 40};
    }

    public String[] objectArray() {
        String[] s = new String[]{"a", "b", "c"};
        return s;
    }

    public int varargs(int... nums) {
        int sum = 0;
        for (int n : nums) {
            sum += n;
        }
        return sum;
    }

    public int callVarargs() {
        return varargs(1, 2, 3, 4);
    }
}
