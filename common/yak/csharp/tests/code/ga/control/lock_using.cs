using System.IO;
namespace Ga.Control {
    public class GaControlLockUsing {
        static readonly object Gate = new object();
        public static int Run() {
            lock (Gate) {
                using (var r = new StringReader("ab")) {
                    return r.Read();
                }
            }
        }
    }
}
