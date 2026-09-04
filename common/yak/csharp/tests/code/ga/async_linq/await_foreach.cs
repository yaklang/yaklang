using System.Collections.Generic;
using System.Threading.Tasks;
namespace Ga.AsyncLinq {
    public class GaAsyncForEach {
        public static async Task<int> SumAsync(IAsyncEnumerable<int> xs) {
            int s = 0;
            await foreach (var x in xs) s += x;
            return s;
        }
    }
}
