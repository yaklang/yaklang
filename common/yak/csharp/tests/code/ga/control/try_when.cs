using System;
namespace Ga.Control {
    public class GaControlTryWhen {
        public static int Safe(int n) {
            try { if (n == 0) throw new ArgumentException("z"); return n; }
            catch (ArgumentException ex) when (ex.Message == "z") { return -1; }
            catch { return -2; }
            finally { n = n; }
        }
    }
}
