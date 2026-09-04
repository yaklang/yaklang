namespace Ga.Patterns {
    public readonly struct GaPatternsSize {
        public int W { get; }
        public int H { get; }
        public GaPatternsSize(int w, int h) { W = w; H = h; }
        public void Deconstruct(out int w, out int h) { w = W; h = H; }
    }
    public class GaPatternsPos {
        public static string Class(GaPatternsSize s) => s switch {
            (0, 0) => "z",
            (var w, var h) when w == h => "sq",
            _ => "r",
        };
    }
}
