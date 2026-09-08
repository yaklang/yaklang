namespace Ga.Generics {
    public class GaGenMethod {
        public static T Id<T>(T x) { return x; }
        public static int Run() { return Id<int>(3); }
    }
}
