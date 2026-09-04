namespace Ga.Patterns {
    public class GaPatternsIs {
        public static int AsInt(object o) {
            if (o is int n) return n;
            return 0;
        }
    }
}
