namespace Ga.Ops {
    public class GaOpsBit {
        public static int Mix(int a, int b) { return (a & b) | (a ^ b) | (a << 1) | (a >> 1); }
    }
}
