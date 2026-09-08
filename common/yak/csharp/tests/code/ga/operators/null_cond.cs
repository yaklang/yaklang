namespace Ga.Ops {
    public class GaOpsNullCond {
        public string Name;
        public static int Len(GaOpsNullCond o) { return o?.Name?.Length ?? 0; }
    }
}
