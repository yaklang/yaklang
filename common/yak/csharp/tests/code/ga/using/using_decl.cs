using System.IO;
namespace Ga.Using {
    public class GaUsingDecl {
        public static string Read() {
            using var r = new StringReader("hi");
            return r.ReadToEnd();
        }
    }
}
