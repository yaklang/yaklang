namespace Ga.Unsafe {
    public unsafe class GaUnsafeStack {
        public static int Sum() {
            int* p = stackalloc int[3];
            p[0] = 1; p[1] = 2; p[2] = 3;
            return p[0] + p[1] + p[2];
        }
    }
}
