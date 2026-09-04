namespace Ga.Types {
    public struct GaTypesPoint {
        public int X;
        public int Y;
        public GaTypesPoint(int x, int y) { X = x; Y = y; }
        public int Sum() { return X + Y; }
    }
}
