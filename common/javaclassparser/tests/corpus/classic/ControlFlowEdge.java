// Control-flow boundary corpus: switch fall-through, string/char switch, dense vs
// sparse switch (table vs lookup), nested loops with plain break/continue, short-circuit
// boolean used as a CONDITION (not returned), and chained if/else-if dispatch. These
// exercise the CFG structurer and switch-case mapping at their edges.
public class ControlFlowEdge {
    public int fallThrough(int x) {
        int r = 0;
        switch (x) {
            case 0:
            case 1:
                r += 1;
            case 2:
                r += 2;
                break;
            case 3:
                r += 3;
            default:
                r += 100;
        }
        return r;
    }

    public int stringSwitch(String s) {
        switch (s) {
            case "alpha":
                return 1;
            case "beta":
                return 2;
            case "gamma":
                return 3;
            default:
                return -1;
        }
    }

    public int sparseSwitch(int x) {
        switch (x) {
            case 1:
                return 10;
            case 100:
                return 20;
            case 10000:
                return 30;
            default:
                return 0;
        }
    }

    public int nestedBreakContinue(int n) {
        int total = 0;
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if (j == i) {
                    continue;
                }
                if (j > 5) {
                    break;
                }
                total += i * j;
            }
        }
        return total;
    }

    public int shortCircuitCondition(int a, int b, int c) {
        int r = 0;
        if (a > 0 && b > 0) {
            r += 1;
        }
        if (a > 0 || c > 0) {
            r += 2;
        }
        if ((a > 0 && b > 0) || c > 0) {
            r += 4;
        }
        return r;
    }

    public String chainedDispatch(int score) {
        if (score >= 90) {
            return "A";
        } else if (score >= 80) {
            return "B";
        } else if (score >= 70) {
            return "C";
        } else if (score >= 60) {
            return "D";
        } else {
            return "F";
        }
    }

    public int whileWithBreak(int n) {
        int i = 0;
        int sum = 0;
        while (true) {
            if (i >= n) {
                break;
            }
            sum += i;
            i++;
        }
        return sum;
    }
}
