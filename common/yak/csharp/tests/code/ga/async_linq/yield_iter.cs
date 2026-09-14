using System.Collections.Generic;
namespace Ga.AsyncLinq {
    public class GaAsyncYield {
        public static IEnumerable<int> Range(int n) {
            for (int i = 0; i < n; i++) yield return i;
            yield break;
        }
    }
}
