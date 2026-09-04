using System.Threading.Tasks;
namespace Ga.AsyncLinq {
    public class GaAsyncAwait {
        public async Task<int> AddAsync(int a, int b) {
            await Task.Yield();
            return a + b;
        }
    }
}
