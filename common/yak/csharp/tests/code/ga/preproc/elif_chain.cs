#define GA_B
namespace Ga.Preproc {
    public class GaPreprocElif {
        public static int V() {
#if GA_A
            return 1;
#elif GA_B
            return 2;
#else
            return 3;
#endif
        }
    }
}
