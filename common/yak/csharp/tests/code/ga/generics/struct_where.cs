namespace Ga.Generics {
    public struct GaGenPair<T, U>
        where T : struct
        where U : class {
        public T Left;
        public U Right;
    }
}
