// Mixed complex corpus: labeled break/continue out of nested loops, StringBuilder
// fluent chains across types, switch with a default in the middle, do/while, a
// conditional-expression used as a method argument, and an instanceof+cast dispatch
// chain. These combine several structuring concerns (labels, loops, switch, ternary
// as an operand) in one method each.
public class ComplexMisc {
    public int labeledBreak(int[][] grid, int target) {
        int found = -1;
        search:
        for (int i = 0; i < grid.length; i++) {
            for (int j = 0; j < grid[i].length; j++) {
                if (grid[i][j] == target) {
                    found = i * 100 + j;
                    break search;
                }
            }
        }
        return found;
    }

    public int labeledContinue(int n) {
        int count = 0;
        outer:
        for (int i = 1; i <= n; i++) {
            for (int j = 2; j < i; j++) {
                if (i % j == 0) {
                    continue outer;
                }
            }
            count++;
        }
        return count;
    }

    public String builderChain(int id, String name, double score) {
        StringBuilder sb = new StringBuilder(64);
        sb.append('[').append(id).append("] ")
          .append(name)
          .append(" => ")
          .append(score)
          .append(score >= 60.0 ? " (pass)" : " (fail)");
        return sb.toString();
    }

    public int switchDefaultMiddle(int x) {
        int r;
        switch (x) {
            case 1:
                r = 10;
                break;
            default:
                r = -1;
                break;
            case 2:
                r = 20;
                break;
        }
        return r;
    }

    public int doWhileSum(int n) {
        int sum = 0;
        int i = 1;
        do {
            sum += i;
            i++;
        } while (i <= n);
        return sum;
    }

    public int ternaryAsArg(int a, int b) {
        return Math.max(a > b ? a : b, (a + b) / 2);
    }

    public String instanceofDispatch(Object o) {
        if (o instanceof Integer) {
            return "int:" + ((Integer) o).intValue();
        } else if (o instanceof String) {
            return "str:" + ((String) o).length();
        } else if (o instanceof int[]) {
            return "arr:" + ((int[]) o).length;
        } else {
            return "other";
        }
    }

    public long countDownProduct(int n) {
        long product = 1;
        while (n > 0) {
            product *= n;
            n--;
        }
        return product;
    }

    public int nestedConditionalAssign(int a, int b, int c) {
        int max = a;
        if (b > max) {
            max = b;
        }
        if (c > max) {
            max = c;
        }
        int min = a < b ? (a < c ? a : c) : (b < c ? b : c);
        return max - min;
    }
}
