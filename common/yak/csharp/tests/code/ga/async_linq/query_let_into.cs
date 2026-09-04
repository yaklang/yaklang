using System.Linq;
namespace Ga.AsyncLinq {
    public class GaAsyncLet {
        public static object LetInto(int[] xs) {
            return from x in xs
                   let y = x * 2
                   where y > 2
                   select y into z
                   where z < 100
                   select z;
        }
    }
}
