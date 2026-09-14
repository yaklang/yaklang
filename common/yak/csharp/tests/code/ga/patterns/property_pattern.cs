namespace Ga.Patterns {
    public class GaPatternsPropBox { public int X; public int Y; }
    public class GaPatternsProp {
        public static bool Origin(GaPatternsPropBox b) => b is { X: 0, Y: 0 };
    }
}
