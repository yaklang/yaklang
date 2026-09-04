using System;
namespace Ga.Types {
    public interface IGaTypesPrinter { void Print(string s); }
    public class GaTypesExplicit : IGaTypesPrinter {
        void IGaTypesPrinter.Print(string s) { Console.WriteLine(s); }
        public void PrintAll(string s) { ((IGaTypesPrinter)this).Print(s); }
    }
}
