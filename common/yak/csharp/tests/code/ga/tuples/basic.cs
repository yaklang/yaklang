namespace Ga.Tuples {
    public class GaTuplesBasic {
        public static (int, string) Pair() { return (1, "a"); }
        public static int Left() { var (a, b) = Pair(); return a + b.Length; }
    }
}
