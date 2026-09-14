using System.Linq;
namespace Ga.AsyncLinq {
    public class GaAsyncQuery {
        public static int[] Pos(int[] xs) {
            return (from x in xs where x > 0 orderby x select x).ToArray();
        }
    }
}
