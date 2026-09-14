namespace Ga.Patterns {
    public class GaPatternsSwitchExpr {
        public static int Map(int n) => n switch { 0 => 1, 1 => 2, _ => 9 };
    }
}
