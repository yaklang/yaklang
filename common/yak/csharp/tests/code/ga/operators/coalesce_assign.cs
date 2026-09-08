namespace Ga.Ops {
    public class GaOpsCoalesce {
        public static string Pick(string a, string b) {
            a ??= b;
            return a ?? "z";
        }
    }
}
