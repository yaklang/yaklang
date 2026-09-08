using System.Linq;
namespace Ga.AsyncLinq {
    public class GaAsyncJoin {
        public static object Join(int[] a, int[] b) {
            return from x in a
                   join y in b on x equals y
                   group x by x % 2 into g
                   select g.Key;
        }
    }
}
