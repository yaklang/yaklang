using System;
namespace Ga.Types {
    public abstract class GaTypesAbstractBase {
        public abstract int Measure();
        public virtual int Tag() { return 1; }
    }
    public sealed class GaTypesSealedLeaf : GaTypesAbstractBase {
        public override int Measure() { return 7; }
        public sealed override int Tag() { return 2; }
    }
}
