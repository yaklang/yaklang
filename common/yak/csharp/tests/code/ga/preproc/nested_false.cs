namespace Ga.Preproc {
    public class GaPreprocNested {
        public static int V() {
#if false
            int dead = 1;
#if true
            int alsoDead = 2;
#endif
#else
            return 9;
#endif
        }
    }
}
