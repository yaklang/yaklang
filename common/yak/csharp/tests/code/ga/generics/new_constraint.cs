namespace Ga.Generics {
    public class GaGenFactory<T> where T : new() {
        public static T Make() { return new T(); }
    }
    public class GaGenWidget { }
}
