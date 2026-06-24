public class SwitchExpr {
    enum Day {MON, TUE, WED, THU, FRI, SAT, SUN}

    public int arrow(Day d) {
        return switch (d) {
            case SAT, SUN -> 0;
            case MON -> 1;
            default -> 2;
        };
    }

    public String yielding(int n) {
        return switch (n) {
            case 1, 2, 3 -> "low";
            case 4, 5, 6 -> {
                String s = "mid";
                yield s + n;
            }
            default -> "high";
        };
    }
}
