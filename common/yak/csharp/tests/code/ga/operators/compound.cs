namespace Ga.Ops {
    public class GaOpsCompound {
        public static int Run(int x) {
            x += 1; x -= 1; x *= 2; x /= 2; x %= 3;
            x &= 7; x |= 1; x ^= 2; x <<= 1; x >>= 1;
            return x;
        }
    }
}
