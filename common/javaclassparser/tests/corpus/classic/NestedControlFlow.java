// Deeply nested control-flow corpus: multi-level for/while nesting (three loop levels),
// labeled break/continue across more than two loop levels, a while-with-inner-switch
// (switch dispatch inside a loop with break/return arms), deep if/else-if chains and a
// break/continue mix. Pure structuring stress (no generics/lambdas), exercising CFG
// region detection and reducible-loop reconstruction.
public class NestedControlFlow {
    public int tripleNested(int[][] grid, int rounds, int target) {
        int hits = 0;
        for (int r = 0; r < rounds; r++) {
            for (int i = 0; i < grid.length; i++) {
                for (int j = 0; j < grid[i].length; j++) {
                    if (grid[i][j] == target) {
                        hits++;
                    } else if (grid[i][j] < 0) {
                        continue;
                    }
                }
            }
        }
        return hits;
    }

    public int labeledAcrossThree(int[][] grid, int rounds, int target) {
        scan:
        for (int r = 0; r < rounds; r++) {
            for (int i = 0; i < grid.length; i++) {
                for (int j = 0; j < grid[i].length; j++) {
                    if (grid[i][j] == target) {
                        return r * 10000 + i * 100 + j;
                    }
                    if (grid[i][j] == -1) {
                        continue scan;
                    }
                }
            }
        }
        return -1;
    }

    public int whileWithSwitch(int n) {
        int state = 0;
        int steps = 0;
        while (steps < 1000) {
            switch (state) {
                case 0:
                    state = n > 0 ? 1 : 2;
                    break;
                case 1:
                    n--;
                    if (n <= 0) {
                        state = 3;
                    }
                    break;
                case 2:
                    state = 3;
                    break;
                default:
                    return steps;
            }
            steps++;
        }
        return -1;
    }

    public int nestedIfElseChain(int a, int b, int c) {
        if (a > b) {
            if (b > c) {
                return 1;
            } else {
                if (a > c) {
                    return 2;
                } else {
                    return 3;
                }
            }
        } else {
            if (a > c) {
                return 4;
            } else if (b > c) {
                return 5;
            }
        }
        return 0;
    }

    public int breakContinueMix(int n) {
        int acc = 0;
        for (int i = 0; i < n; i++) {
            if (i % 2 == 0) {
                continue;
            }
            for (int j = 0; j < i; j++) {
                if (j > 5) {
                    break;
                }
                acc += j;
            }
            if (acc > 100) {
                break;
            }
        }
        return acc;
    }
}
