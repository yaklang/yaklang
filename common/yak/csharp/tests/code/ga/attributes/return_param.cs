using System;
namespace Ga.Attr {
    public class GaAttrTargets {
        [return: CLSCompliant(false)]
        public int M([param: Obsolete] int x) { return x; }
    }
}
