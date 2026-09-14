namespace Ga.Ops {
    public class GaOpsRange {
        public static int Last(int[] xs) { return xs[^1]; }
        public static int[] Mid(int[] xs) { return xs[1..3]; }
    }
}
