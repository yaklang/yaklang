#define GA_ON
namespace Ga.Preproc {
    public class GaPreprocIf {
        public static int V() {
#if GA_ON
            return 1;
#else
            return 0;
#endif
        }
    }
}
