namespace Ga.Unsafe {
    public unsafe class GaUnsafePtr {
        public static int Peek(int v) {
            int* p = &v;
            return *p;
        }
    }
}
