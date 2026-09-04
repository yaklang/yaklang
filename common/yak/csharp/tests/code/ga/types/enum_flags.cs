using System;
namespace Ga.Types {
    [Flags]
    public enum GaTypesColor : byte { None = 0, Red = 1, Green = 2, Blue = 4 }
    public class GaTypesEnumUse {
        public static int Mix() { return (int)(GaTypesColor.Red | GaTypesColor.Blue); }
    }
}
