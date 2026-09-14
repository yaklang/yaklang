namespace Ga.Generics {
    public interface IGaGenMap<in T, out U> { U Map(T x); }
    public class GaGenId : IGaGenMap<string, object> {
        public object Map(string x) { return x; }
    }
}
