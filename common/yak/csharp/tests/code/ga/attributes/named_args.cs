using System;
namespace Ga.Attr {
    [AttributeUsage(AttributeTargets.Class, AllowMultiple = true)]
    public class GaAttrMarkAttribute : Attribute {
        public int Level;
        public GaAttrMarkAttribute(int n) { Level = n; }
    }
    [GaAttrMark(1, Level = 2)]
    public class GaAttrNamed { }
}
