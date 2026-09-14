using System.Diagnostics;
namespace Ga.Attr {
    public class GaAttrCond {
        [Conditional("DEBUG")]
        public static void Trace(string s) { }
    }
}
