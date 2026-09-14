#define TMP
#undef TMP
namespace Ga.Preproc {
    public class GaPreprocUndef {
        public static int V() {
#region inner
#if TMP
            return 0;
#else
            return 4;
#endif
#endregion
        }
    }
}
