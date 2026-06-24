public class Records {
    record Point(int x, int y) {
        Point {
            if (x < 0) {
                throw new IllegalArgumentException("x");
            }
        }

        int sum() {
            return x + y;
        }
    }

    record Pair<A, B>(A first, B second) {
    }

    public int use() {
        Point p = new Point(3, 4);
        Pair<String, Integer> pair = new Pair<>("a", 1);
        return p.sum() + pair.second();
    }
}
