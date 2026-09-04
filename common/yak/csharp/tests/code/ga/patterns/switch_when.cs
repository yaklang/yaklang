namespace Ga.Patterns {
    public class GaPatternsWhen {
        public static string Kind(object o) {
            switch (o) {
                case int n when n > 0: return "pos";
                case string s when s.Length > 0: return "str";
                default: return "other";
            }
        }
    }
}
