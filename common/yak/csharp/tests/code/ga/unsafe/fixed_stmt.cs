namespace Ga.Unsafe {
    public unsafe class GaUnsafeFixed {
        public static int First(int[] xs) {
            fixed (int* p = xs) { return p[0]; }
        }
    }
}
