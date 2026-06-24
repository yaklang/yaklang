public class Loops {
    public int forLoop(int n) {
        int sum = 0;
        for (int i = 0; i < n; i++) {
            sum += i;
        }
        return sum;
    }

    public int whileLoop(int n) {
        int count = 0;
        while (n > 0) {
            n >>= 1;
            count++;
        }
        return count;
    }

    public int doWhile(int n) {
        int sum = 0;
        do {
            sum += n;
            n--;
        } while (n > 0);
        return sum;
    }

    public int forEach(int[] arr) {
        int sum = 0;
        for (int x : arr) {
            sum += x;
        }
        return sum;
    }

    public int labeled(int[][] grid, int target) {
        outer:
        for (int i = 0; i < grid.length; i++) {
            for (int j = 0; j < grid[i].length; j++) {
                if (grid[i][j] == target) {
                    return i * 100 + j;
                }
                if (grid[i][j] < 0) {
                    continue outer;
                }
            }
        }
        return -1;
    }
}
