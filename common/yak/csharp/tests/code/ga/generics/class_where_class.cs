namespace Ga.Generics {
    public class GaGenBox<T> where T : class, new() {
        public T Value = new T();
    }
    public class GaGenItem { }
}
